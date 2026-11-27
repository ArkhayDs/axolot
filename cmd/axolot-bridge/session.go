package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"net"
	"net/http"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	// toolServer names the session's MCP server for the CLI, which calls its tools mcp__<server>__<tool>.
	toolServer = "axolot"
	toolPrefix = "mcp__" + toolServer + "__"
	// roundGap is how long a CLI waits for the results of its tool calls: Axolot may hold one for the user's click.
	// shortcut: a turn Axolot gives up (stop, timeout, round cap) keeps its CLI up to roundGap; add a cancel route if orphans pile up.
	roundGap = 15 * time.Minute
)

var (
	errTooLong   = fmt.Errorf("claude took longer than %s", cliTimeout)
	errNoResults = fmt.Errorf("no tool results within %s", roundGap)
)

// session is one CLI run. With tools it outlives the request that started it: each tool round ends that request,
// and the next one, carrying the results, takes the rest of the answer.
type session struct {
	ctx        context.Context
	cancel     context.CancelCauseFunc
	structured bool
	items      chan item
	done       chan outcome
	// sent is the usage already sent with tool rounds, which the result line counts again.
	sent cliUsage
	// toolsRan: the CLI does not stream a model call that follows a tool result, it prints it whole at the end.
	toolsRan bool
	// idle ends a run whose tool results do not come; guarded by bridge.mu.
	idle    *time.Timer
	mu      sync.Mutex
	results map[string]chan string
}

// item is a piece of answer text, or a tool round's calls with the usage of the model call that made them.
type item struct {
	text  string
	calls []toolCall
	usage cliUsage
}

// outcome is how the run ended: its result line, or why it failed.
type outcome struct {
	result cliEvent
	err    error
}

// toolCall is a tool_use block of an assistant line.
type toolCall struct {
	Type  string          `json:"type"`
	ID    string          `json:"id"`
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`
}

// chatTool is an OpenAI function definition.
type chatTool struct {
	Function struct {
		Name        string          `json:"name"`
		Description string          `json:"description"`
		Parameters  json.RawMessage `json:"parameters"`
	} `json:"function"`
}

// mcpTools turns the request's functions into the tools a session serves; the MCP server takes object schemas only.
func mcpTools(tools []chatTool) ([]*mcp.Tool, error) {
	out := make([]*mcp.Tool, 0, len(tools))
	for _, tool := range tools {
		schema := map[string]any{"type": "object"}
		if raw := tool.Function.Parameters; len(raw) > 0 && string(raw) != "null" {
			if err := json.Unmarshal(raw, &schema); err != nil || schema["type"] != "object" {
				return nil, fmt.Errorf("tool %q: parameters must be a JSON object schema", tool.Function.Name)
			}
		}
		out = append(out, &mcp.Tool{Name: tool.Function.Name, Description: tool.Function.Description, InputSchema: schema})
	}
	return out, nil
}

// start launches the CLI for one question; s.read hands its output to the requests serving the session.
// The prompt goes through stdin: documents can exceed what an argument holds.
// @param schema - JSON schema the answer must follow, empty for free text
// @param tools - offered to the model by the session's MCP server, none for a plain answer
func (b *bridge) start(model, system string, schema json.RawMessage, prompt string, tools []*mcp.Tool) (*session, error) {
	ctx, cancel := context.WithCancelCause(b.base)
	s := &session{ctx: ctx, cancel: cancel, structured: len(schema) > 0, items: make(chan item), done: make(chan outcome, 1), results: map[string]chan string{}}
	config := ""
	if len(tools) > 0 {
		var err error
		if config, err = s.serveTools(tools); err != nil {
			cancel(err)
			return nil, err
		}
	}
	cmd := exec.CommandContext(ctx, b.cli, cliArgs(model, system, schema, config)...)
	cmd.Dir = b.dir
	// An API credential would take precedence over the subscription login and bill the API.
	env := slices.DeleteFunc(os.Environ(), func(kv string) bool {
		return strings.HasPrefix(kv, "ANTHROPIC_API_KEY=") || strings.HasPrefix(kv, "ANTHROPIC_AUTH_TOKEN=")
	})
	// The CLI's own limit on a tool call must not end a round before roundGap does.
	cmd.Env = append(env, "CLAUDE_CODE_DISABLE_AUTO_MEMORY=1", "MCP_TOOL_TIMEOUT="+strconv.FormatInt(roundGap.Milliseconds(), 10))
	cmd.Stdin = strings.NewReader(prompt)
	cmd.WaitDelay = cliWaitDelay
	stderr := &bytes.Buffer{}
	cmd.Stderr = stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel(err)
		return nil, fmt.Errorf("open claude output: %w", err)
	}
	started := time.Now()
	if err := cmd.Start(); err != nil {
		cancel(err)
		return nil, fmt.Errorf("start claude: %w", err)
	}
	go s.read(cmd, stdout, stderr, model, started)
	return s, nil
}

// read follows the CLI up to its exit; the session ends with it.
// @returns into s.done: result line; an error when the CLI reports one, dies or prints something unreadable
func (s *session) read(cmd *exec.Cmd, stdout io.Reader, stderr *bytes.Buffer, model string, started time.Time) {
	defer s.cancel(nil)
	result, err := readResult(stdout, started, s.structured, s.text, s.round)
	if err != nil {
		s.cancel(err)
	}
	waitErr := cmd.Wait()
	if s.ctx.Err() != nil && err == nil {
		err = context.Cause(s.ctx)
	}
	if err == nil {
		err = resultError(result, waitErr)
	}
	if err != nil {
		switch {
		case errors.Is(context.Cause(s.ctx), errNoResults):
			slog.Info("claude stopped", "model", model, "err", errNoResults)
		case !errors.Is(err, context.Canceled):
			slog.Error("claude failed", "model", model, "err", err, "stderr", string(stderr.Bytes()[:min(stderr.Len(), maxLogged)]))
		}
		s.done <- outcome{err: err}
		return
	}
	result.Usage.ExitMs = time.Since(result.resultAt).Milliseconds()
	result.Usage = result.Usage.minus(s.sent)
	result.Usage.Buffered = s.toolsRan
	s.done <- outcome{result: *result}
}

// minus is u without the tool rounds' usage, already sent: the result line counts the whole run.
func (u cliUsage) minus(sent cliUsage) cliUsage {
	u.Input -= sent.Input
	u.CacheCreation -= sent.CacheCreation
	u.CacheRead -= sent.CacheRead
	u.Output -= sent.Output
	return u
}

// text hands one piece of answer to the request serving the session.
func (s *session) text(text string) {
	s.emit(item{text: text})
}

// round hands a tool round to the request serving the session, which ends with it.
func (s *session) round(calls []toolCall, usage cliUsage) {
	s.toolsRan = true
	s.sent.Input += usage.Input
	s.sent.CacheCreation += usage.CacheCreation
	s.sent.CacheRead += usage.CacheRead
	s.sent.Output += usage.Output
	s.emit(item{calls: calls, usage: usage})
}

// emit waits for a request to take the item; a stopped session drops it.
func (s *session) emit(it item) {
	select {
	case s.items <- it:
	case <-s.ctx.Done():
	}
}

// serveTools offers tools to the CLI from an MCP server on a free loopback port, closed with the session.
// @returns the --mcp-config value naming it, with a token of its own
func (s *session) serveTools(tools []*mcp.Tool) (string, error) {
	server := mcp.NewServer(&mcp.Implementation{Name: toolServer, Version: "1"}, nil)
	for _, tool := range tools {
		server.AddTool(tool, s.callTool)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", fmt.Errorf("open tool server: %w", err)
	}
	token := rand.Text()
	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true})
	httpServer := &http.Server{Handler: requireToken(token, handler), ReadHeaderTimeout: 5 * time.Second}
	go httpServer.Serve(listener)
	context.AfterFunc(s.ctx, func() { httpServer.Close() })
	config, err := json.Marshal(map[string]any{"mcpServers": map[string]any{toolServer: map[string]any{
		"type": "http", "url": "http://" + listener.Addr().String(), "headers": map[string]string{"Authorization": "Bearer " + token},
	}}})
	return string(config), err
}

// callTool answers the CLI with the result Axolot sent for this call, matched by its tool_use id, waiting for it if needed.
func (s *session) callTool(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	id, _ := req.Params.Meta["claudecode/toolUseId"].(string)
	if id == "" {
		// The key is undocumented: a CLI update renaming it fails every call, and only this log says why.
		slog.Error("tool call without claudecode/toolUseId", "tool", req.Params.Name)
		return nil, errors.New("tool call without its claudecode/toolUseId")
	}
	select {
	case text := <-s.slot(id):
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-s.ctx.Done():
		return nil, context.Cause(s.ctx)
	}
}

// deliver hands a tool's result to its call, which the CLI makes before or after the result comes.
func (s *session) deliver(id, text string) {
	select {
	case s.slot(id) <- text:
	default:
	}
}

// slot is where the result of call id waits for its call.
func (s *session) slot(id string) chan string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.results[id]; !ok {
		s.results[id] = make(chan string, 1)
	}
	return s.results[id]
}

// serve streams the session to one request, up to the end of the run or of a tool round.
// A failure before the first piece of text returns a status; after it, the connection is cut,
// since a stream the client reads to its end would pass for a complete answer.
func (b *bridge) serve(r *http.Request, out *eventStream, s *session) {
	timeout := time.NewTimer(cliTimeout)
	defer timeout.Stop()
	for {
		var end outcome
		select {
		case <-r.Context().Done():
			s.cancel(context.Canceled)
			return
		case <-timeout.C:
			s.cancel(errTooLong)
			end = outcome{err: errTooLong}
		case it := <-s.items:
			if it.calls == nil {
				out.text(it.text)
				continue
			}
			out.toolCalls(it.calls)
			// Parked before the stream ends: the client may send the results right after.
			b.park(s, it.calls)
			out.finish(it.usage)
			return
		case end = <-s.done:
		}
		if end.err == nil && s.structured && !out.started {
			// Prose instead of the StructuredOutput tool is still an answer: grading fails it like any invalid JSON.
			out.text(end.result.Result)
		}
		switch {
		case end.err != nil && out.started:
			panic(http.ErrAbortHandler)
		case end.err != nil:
			writeError(out.w, http.StatusBadGateway, end.err.Error())
		default:
			out.finish(end.result.Usage)
		}
		return
	}
}

// park keeps s for the request that brings the results of calls, and ends it when none comes within roundGap.
func (b *bridge) park(s *session, calls []toolCall) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.waiting == nil {
		b.waiting = map[string]*session{}
	}
	for _, call := range calls {
		b.waiting[call.ID] = s
	}
	s.idle = time.AfterFunc(roundGap, func() {
		b.mu.Lock()
		b.forget(s)
		b.mu.Unlock()
		s.cancel(errNoResults)
	})
}

// unpark takes back the session waiting for the result of call id, nil when none waits: given up, or the bridge restarted.
func (b *bridge) unpark(id string) *session {
	b.mu.Lock()
	defer b.mu.Unlock()
	s := b.waiting[id]
	if s == nil || !s.idle.Stop() {
		return nil
	}
	b.forget(s)
	return s
}

// forget drops every call s waits for; the caller holds b.mu.
func (b *bridge) forget(s *session) {
	maps.DeleteFunc(b.waiting, func(_ string, waiting *session) bool { return waiting == s })
}

// toolResults are the tool messages ending the conversation, which resume the answer waiting for them.
func toolResults(messages []chatMessage) []chatMessage {
	i := len(messages)
	for i > 0 && messages[i-1].Role == "tool" {
		i--
	}
	return messages[i:]
}

// toolUses picks the tool_use blocks out of an assistant line's content.
func toolUses(content json.RawMessage) ([]toolCall, error) {
	if len(content) == 0 {
		return nil, nil
	}
	var blocks []toolCall
	if err := json.Unmarshal(content, &blocks); err != nil {
		return nil, err
	}
	return slices.DeleteFunc(blocks, func(block toolCall) bool { return block.Type != "tool_use" }), nil
}

package main

import (
	"bufio"
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"
)

const (
	// maxBody and maxLine match what Axolot may send and what a large answer may weigh.
	maxBody = 16 << 20
	maxLine = 16 << 20
	// cliTimeout outlasts Axolot's own 5 minute limit, whose message is the clearer one.
	cliTimeout = 6 * time.Minute
	// cliWaitDelay keeps a killed CLI's surviving children from holding the handler through its pipes.
	cliWaitDelay = 2 * time.Second
	maxLogged    = 2048
)

// models are the identifiers served: the name is the cumulative base's key, so aliases like "opus" are refused.
var models = []string{
	"claude-fable-5-1", "claude-fable-5",
	"claude-opus-5-5", "claude-opus-5", "claude-opus-4-8", "claude-opus-4-7", "claude-opus-4-6", "claude-opus-4-5",
	"claude-sonnet-5-5", "claude-sonnet-5", "claude-sonnet-4-6",
	"claude-haiku-5-5", "claude-haiku-4-5",
}

// bridge serves the Claude CLI over HTTP.
type bridge struct {
	token string
	cli   string
	// dir is the empty working directory, so that no project file reaches the model.
	dir string
	// base ends every CLI at shutdown, since a run outlives its request between tool rounds.
	base context.Context
	mu   sync.Mutex
	// waiting are the sessions waiting for tool results, by tool call id.
	waiting map[string]*session
}

// handler returns the routes behind the token check.
func (b *bridge) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/models", b.listModels)
	mux.HandleFunc("POST /v1/chat/completions", b.chat)
	return requireToken(b.token, mux)
}

// requireToken refuses any request without the bearer token.
func requireToken(token string, next http.Handler) http.Handler {
	expected := []byte("Bearer " + token)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), expected) != 1 {
			slog.Warn("bridge request refused", "path", r.URL.Path, "remote", r.RemoteAddr)
			writeError(w, http.StatusUnauthorized, "invalid token")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// listModels answers /v1/models.
func (b *bridge) listModels(w http.ResponseWriter, _ *http.Request) {
	data := make([]map[string]string, 0, len(models))
	for _, name := range models {
		data = append(data, map[string]string{"id": name, "object": "model"})
	}
	writeJSON(w, http.StatusOK, map[string]any{"object": "list", "data": data})
}

// chatRequest is the part of /v1/chat/completions the CLI can honour: temperature and max_tokens have no CLI option.
type chatRequest struct {
	Model          string        `json:"model"`
	Messages       []chatMessage `json:"messages"`
	Tools          []chatTool    `json:"tools"`
	ResponseFormat struct {
		JSONSchema struct {
			Schema json.RawMessage `json:"schema"`
		} `json:"json_schema"`
	} `json:"response_format"`
}

// chatMessage only takes text content: a message with images comes as parts and fails to decode.
type chatMessage struct {
	Role       string `json:"role"`
	Content    string `json:"content"`
	ToolCallID string `json:"tool_call_id"`
}

// chat answers /v1/chat/completions, always as a stream, the only form Axolot calls.
// A conversation ending with tool results resumes the run that asked for them instead of starting the CLI.
func (b *bridge) chat(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming not supported")
		return
	}
	var body chatRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBody)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request, images are not supported: "+err.Error())
		return
	}
	if !slices.Contains(models, body.Model) {
		writeError(w, http.StatusNotFound, "unknown model "+body.Model)
		return
	}
	out := &eventStream{w: w, flusher: flusher}
	if results := toolResults(body.Messages); len(results) > 0 {
		s := b.unpark(results[0].ToolCallID)
		if s == nil {
			writeError(w, http.StatusNotFound, "no answer waits for these tool results: it was given up or the bridge restarted")
			return
		}
		for _, result := range results {
			s.deliver(result.ToolCallID, result.Content)
		}
		b.serve(r, out, s)
		return
	}
	schema := body.ResponseFormat.JSONSchema.Schema
	if len(body.Tools) > 0 && len(schema) > 0 {
		writeError(w, http.StatusBadRequest, "tools and a JSON schema together are not supported")
		return
	}
	tools, err := mcpTools(body.Tools)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	system, prompt, err := splitMessages(body.Messages)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	s, err := b.start(body.Model, system, schema, prompt, tools)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	b.serve(r, out, s)
}

// splitMessages turns the conversation into the CLI's two inputs, since `claude -p` takes a single prompt.
// @returns system prompt, prompt with past turns serialized when there are any
func splitMessages(messages []chatMessage) (string, string, error) {
	var system []string
	for len(messages) > 0 && messages[0].Role == "system" {
		system = append(system, messages[0].Content)
		messages = messages[1:]
	}
	if len(messages) == 0 || messages[len(messages)-1].Role != "user" {
		return "", "", errors.New("the last message must be a question")
	}
	joined := strings.Join(system, "\n\n")
	if len(messages) == 1 {
		return joined, messages[0].Content, nil
	}
	turns := make([]string, 0, len(messages))
	for _, message := range messages {
		switch message.Role {
		case "user":
			turns = append(turns, "User: "+message.Content)
		case "assistant":
			turns = append(turns, "Assistant: "+message.Content)
		default:
			return "", "", fmt.Errorf("unsupported role %q", message.Role)
		}
	}
	return joined, strings.Join(turns, "\n\n"), nil
}

// cliArgs cuts everything local from the CLI: tools, MCP, settings, skills, sessions.
// --system-prompt is always passed, or Claude Code's own prompt would apply.
// --json-schema adds the StructuredOutput tool, the only one --tools "" leaves.
// @param mcpConfig - the session's MCP server, whose tools run without asking: Axolot applies the user's permissions; empty for none
func cliArgs(model, system string, schema json.RawMessage, mcpConfig string) []string {
	args := []string{
		"-p", "--model", model, "--system-prompt", system,
		"--output-format", "stream-json", "--verbose", "--include-partial-messages",
		"--no-session-persistence", "--setting-sources", "", "--strict-mcp-config",
		"--disable-slash-commands", "--tools", "",
	}
	if len(schema) > 0 {
		args = append(args, "--json-schema", string(schema))
	}
	if mcpConfig != "" {
		args = append(args, "--mcp-config", mcpConfig, "--allowedTools", "mcp__"+toolServer)
	}
	return args
}

// cliUsage is the token count of the result line, the input split over the prompt cache, plus the CLI's
// own time measured by the bridge: startup before the API call (nil without an init line) and exit after the result.
// Buffered marks text the CLI printed in one block, too late to time its generation.
type cliUsage struct {
	Input         int    `json:"input_tokens"`
	CacheCreation int    `json:"cache_creation_input_tokens"`
	CacheRead     int    `json:"cache_read_input_tokens"`
	Output        int    `json:"output_tokens"`
	StartupMs     *int64 `json:"-"`
	ExitMs        int64  `json:"-"`
	Buffered      bool   `json:"-"`
}

// cliEvent holds the fields read from a stream-json line; other line types only leave them empty.
// resultAt is when the bridge read the result line.
type cliEvent struct {
	Type    string `json:"type"`
	Subtype string `json:"subtype"`
	// Message is an assistant line's content, read for its tool_use blocks.
	Message struct {
		Content json.RawMessage `json:"content"`
	} `json:"message"`
	Event struct {
		ContentBlock struct {
			Type string `json:"type"`
		} `json:"content_block"`
		Delta struct {
			Type        string `json:"type"`
			Text        string `json:"text"`
			PartialJSON string `json:"partial_json"`
			StopReason  string `json:"stop_reason"`
		} `json:"delta"`
		Usage cliUsage `json:"usage"`
	} `json:"event"`
	IsError  bool     `json:"is_error"`
	Result   string   `json:"result"`
	Usage    cliUsage `json:"usage"`
	resultAt time.Time
}

// readResult follows the stream up to its end and returns the result line, nil when none came.
// With a schema the answer is the StructuredOutput tool's input, streamed like text so that TTFT and
// tok/s stay measured; free text around it is dropped, and only the first attempt passes, as a local model gets one.
// Without one, a model call ending on tool use hands its calls and usage to onRound.
// The CLI's startup time, up to its init line, goes with the first usage handed on.
// @param started - when the CLI started
// @param structured - a schema was requested
func readResult(stdout io.Reader, started time.Time, structured bool, onText func(string), onRound func([]toolCall, cliUsage)) (*cliEvent, error) {
	var result *cliEvent
	var startup *int64
	var calls []toolCall
	attempts := 0
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 64*1024), maxLine)
	for scanner.Scan() {
		var event cliEvent
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			return nil, fmt.Errorf("read claude output: %w", err)
		}
		switch {
		case event.Type == "system" && event.Subtype == "init":
			ms := time.Since(started).Milliseconds()
			startup = &ms
		case event.Type == "result":
			event.resultAt = time.Now()
			event.Usage.StartupMs, startup = startup, nil
			result = &event
		case !structured && event.Type == "assistant":
			uses, err := toolUses(event.Message.Content)
			if err != nil {
				return nil, fmt.Errorf("read claude output: %w", err)
			}
			calls = append(calls, uses...)
		case !structured && event.Event.Delta.StopReason == "tool_use" && len(calls) > 0:
			usage := event.Event.Usage
			usage.StartupMs, startup = startup, nil
			onRound(calls, usage)
			calls = nil
		case !structured && event.Event.Delta.Type == "text_delta":
			onText(event.Event.Delta.Text)
		case structured && event.Event.Delta.Type == "input_json_delta" && attempts == 1:
			onText(event.Event.Delta.PartialJSON)
		case structured && event.Event.ContentBlock.Type == "tool_use":
			attempts++
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read claude output: %w", err)
	}
	return result, nil
}

// resultError turns how the CLI ended into an error; its own message comes first as the most useful (quota, login).
func resultError(result *cliEvent, waitErr error) error {
	switch {
	case result == nil:
		return fmt.Errorf("claude ended without a result: %v", waitErr)
	case result.IsError:
		return errors.New(result.Result)
	case waitErr != nil:
		return fmt.Errorf("claude failed after its result: %w", waitErr)
	}
	return nil
}

// eventStream writes the SSE answer; headers leave with the first chunk so that an early failure can still set a status.
type eventStream struct {
	w       http.ResponseWriter
	flusher http.Flusher
	started bool
}

// send writes one chunk.
func (s *eventStream) send(chunk any) {
	if !s.started {
		s.started = true
		s.w.Header().Set("Content-Type", "text/event-stream")
		s.w.Header().Set("Cache-Control", "no-cache")
	}
	payload, err := json.Marshal(chunk)
	if err != nil {
		panic(err)
	}
	// A failed write means the client left; the context stops the CLI.
	_, _ = fmt.Fprintf(s.w, "data: %s\n\n", payload)
	s.flusher.Flush()
}

// text sends one piece of answer; an empty one would start the stream for nothing, losing the error status.
func (s *eventStream) text(content string) {
	if content == "" {
		return
	}
	s.send(map[string]any{"choices": []any{map[string]any{"index": 0, "delta": map[string]string{"content": content}}}})
}

// toolCalls sends a round's calls in one chunk, named as Axolot offered them, arguments as JSON text.
// shortcut: assumes the CLI keeps "mcp__axolot__" + the offered name whole; check if a name over 51 characters comes back unknown.
func (s *eventStream) toolCalls(calls []toolCall) {
	wire := make([]map[string]any, len(calls))
	for i, call := range calls {
		function := map[string]string{"name": strings.TrimPrefix(call.Name, toolPrefix), "arguments": string(call.Input)}
		wire[i] = map[string]any{"index": i, "id": call.ID, "type": "function", "function": function}
	}
	s.send(map[string]any{"choices": []any{map[string]any{"index": 0, "delta": map[string]any{"tool_calls": wire}}}})
}

// finish sends the usage and closes the stream. The cache parts count as input: they are read by the model
// and weigh on the quota, like a local model's whole prompt. The CLI times ride along, ignored by other clients.
func (s *eventStream) finish(usage cliUsage) {
	sent := map[string]any{
		"prompt_tokens":     usage.Input + usage.CacheCreation + usage.CacheRead,
		"completion_tokens": usage.Output,
		"cli_exit_ms":       usage.ExitMs,
	}
	if usage.StartupMs != nil {
		sent["cli_startup_ms"] = *usage.StartupMs
	}
	if usage.Buffered {
		sent["cli_buffered"] = true
	}
	s.send(map[string]any{"choices": []any{}, "usage": sent})
	_, _ = fmt.Fprint(s.w, "data: [DONE]\n\n")
	s.flusher.Flush()
}

// writeError answers in the OpenAI error shape, which Axolot shows as is.
func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]any{"error": map[string]string{"message": message}})
}

// writeJSON writes one JSON response.
func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		slog.Warn("write response", "err", err)
	}
}

package api

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/arkhayds/axolot/internal/mcp"
	"github.com/arkhayds/axolot/internal/provider"
	"github.com/arkhayds/axolot/internal/store"
)

const (
	chatMaxRounds = 10
	toolTimeout   = 30 * time.Second
	// toolResultMax counts characters: a large result would crowd the model's context out.
	toolResultMax = 20000
	toolCutNotice = "\n\n[Résultat tronqué : la suite n'a pas été transmise.]"
)

var (
	// errTooManyRounds is shown as is in the answer card.
	errTooManyRounds = fmt.Errorf("Arrêté après %d tours d'appels d'outils, sans réponse", chatMaxRounds)
	errRefused       = errors.New("the user refused this tool call")
)

// answer runs one chat turn: the model, then each tool it asks for, until it answers in text.
// TTFT runs from the turn start to the final answer's first token, tools included; TotalMs is the whole turn;
// both leave out the time spent waiting for the user's approval (tools.waited); tokens and speed are the last model call's.
func answer(ctx context.Context, client provider.Provider, req provider.ChatRequest, tools *toolbox, send func(map[string]any)) (provider.ChatResult, error) {
	start := time.Now()
	elapsed := func() int64 { return (time.Since(start) - tools.waited).Milliseconds() }
	req.Tools = tools.tools
	onDelta := func(text string) { send(map[string]any{"type": "delta", "text": text}) }
	for round := range chatMaxRounds {
		offset := elapsed()
		result, err := client.Chat(ctx, req, onDelta)
		if err != nil && len(req.Tools) > 0 && refusesTools(err) {
			send(map[string]any{"type": "notice", "text": "Ce modèle n'appelle pas d'outils"})
			req.Tools = nil
			offset = elapsed()
			result, err = client.Chat(ctx, req, onDelta)
		}
		if err != nil {
			return result, err
		}
		if len(result.ToolCalls) == 0 {
			if result.TTFTMs != nil {
				ttft := offset + *result.TTFTMs
				result.TTFTMs = &ttft
			}
			result.TotalMs = elapsed()
			return result, nil
		}
		if round == chatMaxRounds-1 {
			break
		}
		req.Messages = append(req.Messages, provider.Message{Role: "assistant", Content: result.Output, ToolCalls: result.ToolCalls})
		for _, call := range result.ToolCalls {
			req.Messages = append(req.Messages, tools.run(ctx, call, send))
		}
	}
	return provider.ChatResult{}, errTooManyRounds
}

// refusesTools tells whether the model server turned the request down for its tools:
// Ollama for a model without tool support, vLLM and llama.cpp when started without tool parsing.
func refusesTools(err error) bool {
	for _, refusal := range []string{"does not support tools", "--enable-auto-tool-choice", "--jinja"} {
		if strings.Contains(err.Error(), refusal) {
			return true
		}
	}
	return false
}

// run calls one tool for the model and streams the step; a failure or a refusal becomes the tool's reply, for the model to go on.
// Steps run one after the other, so the client pairs each approval and tool_result with the tool_call just before it.
func (t *toolbox) run(ctx context.Context, call provider.ToolCall, send func(map[string]any)) provider.Message {
	send(map[string]any{"type": "tool_call", "name": call.Name, "arguments": call.Arguments})
	var err error
	if t.routes[call.Name].ask {
		err = t.approve(ctx, send)
	}
	start := time.Now()
	var text string
	if err == nil {
		callCtx, cancel := context.WithTimeout(ctx, toolTimeout)
		text, err = t.call(callCtx, call.Name, call.Arguments)
		cancel()
	}
	if err != nil {
		slog.Warn("tool call failed", "tool", call.Name, "err", err)
		text = "Error: " + err.Error()
	}
	text = cutToolResult(text)
	send(map[string]any{"type": "tool_result", "result": text, "error": err != nil, "ms": time.Since(start).Milliseconds()})
	return provider.Message{Role: "tool", Content: text, ToolCallID: call.ID, ToolName: call.Name}
}

// approve waits for the user's click on an asked tool, adding the wait to t.waited.
func (t *toolbox) approve(ctx context.Context, send func(map[string]any)) error {
	start := time.Now()
	allow, err := t.approvals.wait(ctx, func(id string) { send(map[string]any{"type": "approval", "id": id}) })
	t.waited += time.Since(start)
	if err == nil && !allow {
		err = errRefused
	}
	return err
}

// approvals are the tool calls waiting for the user's click in the Chat, by id.
type approvals struct {
	mu      sync.Mutex
	pending map[string]chan bool
}

// wait registers a call, hands its id to announce, and blocks until the user answers or ctx ends.
// @returns {bool} - true when the user allowed the call
func (a *approvals) wait(ctx context.Context, announce func(id string)) (bool, error) {
	id := rand.Text()
	click := make(chan bool, 1)
	a.mu.Lock()
	if a.pending == nil {
		a.pending = map[string]chan bool{}
	}
	a.pending[id] = click
	a.mu.Unlock()
	defer func() {
		a.mu.Lock()
		delete(a.pending, id)
		a.mu.Unlock()
	}()
	announce(id)
	select {
	case allow := <-click:
		return allow, nil
	case <-ctx.Done():
		return false, ctx.Err()
	}
}

// answer hands the user's click to the call waiting under id; false when none waits, answered or turn over.
func (a *approvals) answer(id string, allow bool) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	click, ok := a.pending[id]
	if ok {
		delete(a.pending, id)
		click <- allow
	}
	return ok
}

// answerApproval passes the user's "Autoriser" or "Refuser" click to the tool call waiting for it.
func (s *Server) answerApproval(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Allow bool `json:"allow"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	if !s.approvals.answer(r.PathValue("id"), body.Allow) {
		writeError(w, http.StatusNotFound, "Cet appel n'attend plus de réponse")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// cutToolResult keeps the first toolResultMax characters and says the rest was dropped.
func cutToolResult(text string) string {
	kept := 0
	for i := range text {
		if kept == toolResultMax {
			return text[:i] + toolCutNotice
		}
		kept++
	}
	return text
}

// toolbox is what one chat turn may call: the enabled servers' tools not blocked, named by offeredName,
// each routed to its server's open session.
type toolbox struct {
	tools    []provider.Tool
	routes   map[string]toolRoute
	sessions []*mcp.Session
	// warnings name the servers whose tools are missing, for the reader of the answer.
	warnings  []string
	approvals *approvals
	// waited is the turn's time spent waiting for the user's clicks.
	waited time.Duration
}

// toolRoute is the session running an offered tool, the name its server knows it by, and whether the user must allow each call.
type toolRoute struct {
	session *mcp.Session
	name    string
	ask     bool
}

// openedMcp is one server's session and tools, or why it could not be opened.
type openedMcp struct {
	session *mcp.Session
	tools   []mcp.Tool
	err     error
}

// openToolbox connects to every enabled server at once; a server that fails leaves a warning, the others still serve.
// shortcut: a sign-in token expiring during the turn makes the next calls fail, returned to the model; refresh per call if turns outgrow the hour.
func (s *Server) openToolbox(ctx context.Context, servers []store.McpServer) *toolbox {
	opened := make([]openedMcp, len(servers))
	var wg sync.WaitGroup
	for i, m := range servers {
		if m.Enabled {
			wg.Go(func() { opened[i] = s.openMcp(ctx, m) })
		}
	}
	wg.Wait()
	box := &toolbox{tools: []provider.Tool{}, routes: map[string]toolRoute{}, approvals: &s.approvals}
	for i, m := range servers {
		o := opened[i]
		switch {
		case !m.Enabled:
			continue
		case signInNeeded(o.err, m):
			box.warnings = append(box.warnings, m.Name+" : connexion requise, outils absents")
		case o.err != nil:
			box.warnings = append(box.warnings, m.Name+" : "+mcpFailure(o.err)+", outils absents")
		}
		if o.err != nil {
			slog.Warn("mcp server left out of the chat", "server", m.Name, "err", o.err)
			continue
		}
		box.sessions = append(box.sessions, o.session)
		for _, tool := range o.tools {
			mode := toolMode(m, tool)
			if mode == "block" {
				continue
			}
			offered := offeredName(m.Name, tool.Name)
			if _, taken := box.routes[offered]; taken {
				slog.Warn("mcp tool left out, its offered name is taken", "server", m.Name, "tool", tool.Name, "offered", offered)
				continue
			}
			box.routes[offered] = toolRoute{session: o.session, name: tool.Name, ask: mode == "ask"}
			tool.Name = offered
			box.tools = append(box.tools, tool.Tool)
		}
	}
	return box
}

// notToolNameChar is what OpenAI refuses in a tool name; it turns the whole request down for one.
var notToolNameChar = regexp.MustCompile(`[^a-zA-Z0-9_-]`)

// offeredName is "<server>__<tool>" as OpenAI accepts it: other characters become _, cut at 64.
func offeredName(server, tool string) string {
	name := server + "__" + notToolNameChar.ReplaceAllString(tool, "_")
	return name[:min(len(name), 64)]
}

// openMcp connects to one server and lists its tools within the probe timeout; the session outlives that timeout.
func (s *Server) openMcp(ctx context.Context, m store.McpServer) openedMcp {
	token, err := s.mcpToken(ctx, m)
	if err != nil {
		return openedMcp{err: err}
	}
	ctx, cancel := context.WithTimeout(ctx, mcpProbeTimeout)
	defer cancel()
	session, err := mcp.Open(ctx, m.URL, token)
	if err != nil {
		return openedMcp{err: err}
	}
	tools, err := session.Tools(ctx)
	if err != nil {
		session.Close()
		return openedMcp{err: err}
	}
	return openedMcp{session: session, tools: tools}
}

// call runs an offered tool on its server; an unknown name is the model's mistake, returned to it as an error.
// @param arguments - JSON object written by the model
func (t *toolbox) call(ctx context.Context, name, arguments string) (string, error) {
	route, ok := t.routes[name]
	if !ok {
		return "", fmt.Errorf("unknown tool %q", name)
	}
	return route.session.Call(ctx, route.name, arguments)
}

// close ends every session of the turn.
func (t *toolbox) close() {
	for _, session := range t.sessions {
		session.Close()
	}
}

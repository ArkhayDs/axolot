package api

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"slices"
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
	// approvalTimeout stays under the bridge's 15 min roundGap, after which Claude Code no longer takes the result.
	approvalTimeout = 14 * time.Minute
)

var (
	// errTooManyRounds is shown as is in the answer card.
	errTooManyRounds = fmt.Errorf("Arrêté après %d tours d'appels d'outils, sans réponse", chatMaxRounds)
	errRefused       = errors.New("the user refused this tool call")
	errNoApproval    = fmt.Errorf("the user did not answer within %d minutes, the tool was not called", int(approvalTimeout.Minutes()))
)

// answer runs one chat turn: the model, then each tool it asks for, until it answers in text.
// TTFT runs from the turn start to the final answer's first token, tools included; TotalMs is the whole turn;
// both leave out the time spent waiting for the user's approval (tools.waited); tokens and speed are the last model call's;
// model load and CLI startup add up over every call, as they mostly happen in the first one.
func answer(ctx context.Context, client provider.Provider, req provider.ChatRequest, tools *toolbox, send func(map[string]any)) (provider.ChatResult, error) {
	start := time.Now()
	elapsed := func() int64 { return (time.Since(start) - tools.waited).Milliseconds() }
	var load, startup *int64
	req.Tools = tools.tools
	req.Messages = tools.tellBlocked(req.Messages)
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
		load, startup = addMs(load, result.LoadMs), addMs(startup, result.CLIStartupMs)
		if len(result.ToolCalls) == 0 {
			if result.TTFTMs != nil {
				ttft := offset + *result.TTFTMs
				result.TTFTMs = &ttft
			}
			result.TotalMs = elapsed()
			result.LoadMs, result.CLIStartupMs = load, startup
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

// addMs adds a call's optional duration to a running total, nil while no call reported one.
func addMs(total, more *int64) *int64 {
	if more == nil {
		return total
	}
	sum := *more
	if total != nil {
		sum += *total
	}
	return &sum
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
	if route := t.routes[call.Name]; route.ask && !t.approvals.allowed(route.server, route.name) {
		err = t.approve(ctx, route, send)
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
	send(map[string]any{"type": "tool_result", "result": text, "error": err != nil, "refused": errors.Is(err, errRefused), "ms": time.Since(start).Milliseconds()})
	return provider.Message{Role: "tool", Content: text, ToolCallID: call.ID, ToolName: call.Name}
}

// approve waits for the user's click on an asked tool, adding the wait to t.waited; the answer's clock stands still meanwhile.
// No click within t.approvalMax leaves the tool uncalled, for the model to go on.
func (t *toolbox) approve(ctx context.Context, route toolRoute, send func(map[string]any)) error {
	start := time.Now()
	resume := t.clock.pause()
	waitCtx, cancel := context.WithTimeout(ctx, t.approvalMax)
	allow, err := t.approvals.wait(waitCtx, route.server, route.name, func(id string) { send(map[string]any{"type": "approval", "id": id}) })
	cancel()
	resume()
	t.waited += time.Since(start)
	switch {
	case err != nil && ctx.Err() == nil:
		err = errNoApproval
	case err == nil && !allow:
		err = errRefused
	}
	return err
}

// approvals are the tool calls waiting for the user's click in the Chat, by id,
// and the tools "Toujours autoriser" switched to allow, which answers opened before the click would still ask for.
type approvals struct {
	mu      sync.Mutex
	pending map[string]pendingCall
	always  map[serverTool]bool
}

// serverTool is a tool by its server's id and the name its server knows.
type serverTool struct {
	server int64
	tool   string
}

// pendingCall is where the click goes, and the tool "Toujours autoriser" switches to allow.
type pendingCall struct {
	click chan bool
	serverTool
}

// wait registers a call to the server's tool, hands its id to announce, and blocks until the user answers or ctx ends.
// @returns {bool} - true when the user allowed the call
func (a *approvals) wait(ctx context.Context, server int64, tool string, announce func(id string)) (bool, error) {
	id := rand.Text()
	click := make(chan bool, 1)
	a.mu.Lock()
	if a.pending == nil {
		a.pending = map[string]pendingCall{}
	}
	a.pending[id] = pendingCall{click: click, serverTool: serverTool{server, tool}}
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

// find returns the call waiting under id; ok is false when none waits, answered or turn over.
func (a *approvals) find(id string) (pendingCall, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	call, ok := a.pending[id]
	return call, ok
}

// answer hands the user's click to the call waiting under id, false when none waits;
// always lets the answers already running call this tool without asking.
func (a *approvals) answer(id string, allow, always bool) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	call, ok := a.pending[id]
	if !ok {
		return false
	}
	delete(a.pending, id)
	call.click <- allow
	if always {
		if a.always == nil {
			a.always = map[serverTool]bool{}
		}
		a.always[call.serverTool] = true
	}
	return true
}

// allowed tells whether "Toujours autoriser" was clicked for the tool since its permission was last set in the panel.
func (a *approvals) allowed(server int64, tool string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.always[serverTool{server, tool}]
}

// forget drops a tool's "Toujours autoriser": the permission set in the panel rules again.
func (a *approvals) forget(server int64, tool string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.always, serverTool{server, tool})
}

// answerApproval passes the user's "Autoriser", "Toujours autoriser" or "Refuser" click to the tool call waiting for it;
// "Toujours autoriser" (always) also sets the tool to allow for every later call.
func (s *Server) answerApproval(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Allow  bool `json:"allow"`
		Always bool `json:"always"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	if body.Always && !body.Allow {
		writeError(w, http.StatusBadRequest, "Toujours autoriser un appel refusé n'a pas de sens")
		return
	}
	id := r.PathValue("id")
	call, ok := s.approvals.find(id)
	if !ok {
		writeError(w, http.StatusNotFound, "Cet appel n'attend plus de réponse")
		return
	}
	// Saved before the call goes on: a failure leaves it waiting for another click.
	if body.Always {
		if err := s.cfg.Store.SetMcpToolMode(r.Context(), call.server, call.tool, "allow"); err != nil {
			slog.Error("always allow not saved", "server", call.server, "tool", call.tool, "err", err)
			writeError(w, http.StatusInternalServerError, "Permission non enregistrée, l'appel attend toujours une réponse")
			return
		}
	}
	if !s.approvals.answer(id, body.Allow, body.Always) {
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
	// blocked are the offered names of the tools the user blocked, told to the model.
	blocked []string
	// warnings name the servers whose tools are missing, for the reader of the answer.
	warnings  []string
	approvals *approvals
	// clock is the answer's chatTimeout, nil when the caller sets none.
	clock *chatClock
	// waited is the turn's time spent waiting for the user's clicks.
	waited      time.Duration
	approvalMax time.Duration
}

// toolRoute is the session running an offered tool, its server's id, the name its server knows it by, and whether the user must allow each call.
type toolRoute struct {
	session *mcp.Session
	server  int64
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
	box := &toolbox{tools: []provider.Tool{}, routes: map[string]toolRoute{}, approvals: &s.approvals, approvalMax: approvalTimeout}
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
			offered := offeredName(m.Name, tool.Name)
			if mode == "block" {
				box.blocked = append(box.blocked, offered)
				continue
			}
			if _, taken := box.routes[offered]; taken {
				slog.Warn("mcp tool left out, its offered name is taken", "server", m.Name, "tool", tool.Name, "offered", offered)
				continue
			}
			box.routes[offered] = toolRoute{session: o.session, server: m.ID, name: tool.Name, ask: mode == "ask"}
			tool.Name = offered
			box.tools = append(box.tools, tool.Tool)
		}
	}
	// Two tools can share an offered name: the one offered must not be called blocked.
	// Two blocked tools can share one too: the model reads it once.
	box.blocked = slices.DeleteFunc(box.blocked, func(name string) bool { _, offered := box.routes[name]; return offered })
	slices.Sort(box.blocked)
	box.blocked = slices.Compact(box.blocked)
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

// tellBlocked adds the blocked tools to the system instruction, so the model says the user blocked one instead of claiming it does not exist.
// Appended to an existing instruction: some chat templates read only the first system message.
func (t *toolbox) tellBlocked(messages []provider.Message) []provider.Message {
	if len(t.blocked) == 0 {
		return messages
	}
	notice := "The user blocked these tools, you cannot call them: " + strings.Join(t.blocked, ", ") +
		". When a request needs one of them, tell the user they blocked it themselves, in the MCP server's Permissions tab."
	if len(messages) > 0 && messages[0].Role == "system" {
		messages = slices.Clone(messages)
		messages[0].Content += "\n\n" + notice
		return messages
	}
	return append([]provider.Message{{Role: "system", Content: notice}}, messages...)
}

// close ends every session of the turn.
func (t *toolbox) close() {
	for _, session := range t.sessions {
		session.Close()
	}
}

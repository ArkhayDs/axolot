package api

import (
	"context"
	"fmt"
	"log/slog"
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

// errTooManyRounds is shown as is in the answer card.
var errTooManyRounds = fmt.Errorf("Arrêté après %d tours d'appels d'outils, sans réponse", chatMaxRounds)

// answer runs one chat turn: the model, then each tool it asks for, until it answers in text.
// TTFT runs from the turn start to the final answer's first token, tools included; TotalMs is the whole turn;
// tokens and speed are the last model call's.
func answer(ctx context.Context, client provider.Provider, req provider.ChatRequest, tools *toolbox, send func(map[string]any)) (provider.ChatResult, error) {
	start := time.Now()
	req.Tools = tools.tools
	onDelta := func(text string) { send(map[string]any{"type": "delta", "text": text}) }
	for round := range chatMaxRounds {
		offset := time.Since(start).Milliseconds()
		result, err := client.Chat(ctx, req, onDelta)
		if err != nil && len(req.Tools) > 0 && refusesTools(err) {
			send(map[string]any{"type": "notice", "text": "Ce modèle n'appelle pas d'outils"})
			req.Tools = nil
			offset = time.Since(start).Milliseconds()
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
			result.TotalMs = time.Since(start).Milliseconds()
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

// run calls one tool for the model and streams the step; a failure becomes the tool's reply, for the model to correct.
// Steps run one after the other, so the client pairs each tool_result with the tool_call just before it.
func (t *toolbox) run(ctx context.Context, call provider.ToolCall, send func(map[string]any)) provider.Message {
	send(map[string]any{"type": "tool_call", "name": call.Name, "arguments": call.Arguments})
	ctx, cancel := context.WithTimeout(ctx, toolTimeout)
	defer cancel()
	start := time.Now()
	text, err := t.call(ctx, call.Name, call.Arguments)
	if err != nil {
		slog.Warn("tool call failed", "tool", call.Name, "err", err)
		text = "Error: " + err.Error()
	}
	text = cutToolResult(text)
	send(map[string]any{"type": "tool_result", "result": text, "error": err != nil, "ms": time.Since(start).Milliseconds()})
	return provider.Message{Role: "tool", Content: text, ToolCallID: call.ID, ToolName: call.Name}
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

// toolbox is what one chat turn may call: the enabled servers' tools, named "<server>__<tool>",
// each routed to its server's open session.
// shortcut: a tool name outside [a-zA-Z0-9_-] or over 64 characters makes OpenAI refuse the whole request; filter them if a server has some.
type toolbox struct {
	tools    []provider.Tool
	routes   map[string]toolRoute
	sessions []*mcp.Session
	// warnings name the servers whose tools are missing, for the reader of the answer.
	warnings []string
}

// toolRoute is the session running an offered tool and the name its server knows it by.
type toolRoute struct {
	session *mcp.Session
	name    string
}

// openedMcp is one server's session and tools, or why it could not be opened.
type openedMcp struct {
	session *mcp.Session
	tools   []provider.Tool
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
	box := &toolbox{tools: []provider.Tool{}, routes: map[string]toolRoute{}}
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
			offered := m.Name + "__" + tool.Name
			box.routes[offered] = toolRoute{session: o.session, name: tool.Name}
			tool.Name = offered
			box.tools = append(box.tools, tool)
		}
	}
	return box
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

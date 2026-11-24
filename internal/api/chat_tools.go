package api

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/arkhayds/axolot/internal/mcp"
	"github.com/arkhayds/axolot/internal/provider"
	"github.com/arkhayds/axolot/internal/store"
)

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
			box.warnings = append(box.warnings, m.Name+" injoignable, outils absents")
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

package api

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/arkhayds/axolot/internal/mcp"
	"github.com/arkhayds/axolot/internal/store"
)

// mcpProbeTimeout covers connect plus tools/list, slower than a model listing over the internet.
const mcpProbeTimeout = 5 * time.Second

// mcpName becomes the prefix of the server's tools, and tool names accept only letters, digits, _ and -.
var mcpName = regexp.MustCompile(`^[a-z0-9_-]{1,32}$`)

// mcpServerView is a saved server with the tools it offers right now, or why it could not be reached.
type mcpServerView struct {
	store.McpServer
	Tools []string `json:"tools"`
	Error string   `json:"error,omitempty"`
}

// listMcpServers returns each server with its live tool names.
func (s *Server) listMcpServers(w http.ResponseWriter, r *http.Request) {
	servers, err := s.cfg.Store.McpServers(r.Context())
	if err != nil {
		internalError(w, r, err)
		return
	}
	views := make([]mcpServerView, 0, len(servers))
	for _, m := range servers {
		views = append(views, s.probeMcp(r.Context(), m))
	}
	writeJSON(w, http.StatusOK, views)
}

// probeMcp lists a server's tools; a failure is data for the panel, not an error.
func (s *Server) probeMcp(ctx context.Context, m store.McpServer) mcpServerView {
	view := mcpServerView{McpServer: m, Tools: []string{}}
	token := ""
	if m.HasToken {
		var err error
		if token, err = s.cfg.Box.Open(m.TokenEnc); err != nil {
			view.Error = "Jeton illisible, supprimer et recréer le serveur"
			return view
		}
	}
	probeCtx, cancel := context.WithTimeout(ctx, mcpProbeTimeout)
	defer cancel()
	tools, err := mcp.Tools(probeCtx, m.URL, token)
	if err != nil {
		view.Error = err.Error()
		return view
	}
	for _, tool := range tools {
		view.Tools = append(view.Tools, tool.Name)
	}
	return view
}

// createMcpServer saves an enabled server, encrypting its token.
func (s *Server) createMcpServer(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name  string `json:"name"`
		URL   string `json:"url"`
		Token string `json:"token"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	if msg := validateMcpServer(body.Name, body.URL); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	var sealed []byte
	if token := strings.TrimSpace(body.Token); token != "" {
		var err error
		if sealed, err = s.cfg.Box.Seal(token); err != nil {
			internalError(w, r, err)
			return
		}
	}
	id, err := s.cfg.Store.CreateMcpServer(r.Context(), body.Name, body.URL, sealed)
	if errors.Is(err, store.ErrNameTaken) {
		writeError(w, http.StatusConflict, "Ce nom est déjà pris")
		return
	}
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]int64{"id": id})
}

// validateMcpServer returns a user-facing message, empty when valid.
func validateMcpServer(name, rawURL string) string {
	switch {
	case !mcpName.MatchString(name):
		return "Nom invalide : minuscules, chiffres, - et _ (32 au plus)"
	case !httpURL(rawURL):
		return "URL invalide"
	}
	return ""
}

// setMcpServerEnabled turns a server's tools on or off for the Chat.
func (s *Server) setMcpServerEnabled(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var body struct {
		Enabled bool `json:"enabled"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	if err := s.cfg.Store.SetMcpServerEnabled(r.Context(), id, body.Enabled); err != nil {
		internalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// deleteMcpServer removes a server.
func (s *Server) deleteMcpServer(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	if err := s.cfg.Store.DeleteMcpServer(r.Context(), id); err != nil {
		internalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

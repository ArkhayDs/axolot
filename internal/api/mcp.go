package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"syscall"
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

// listMcpServers returns each server with its live tool names, probed in parallel so unreachable servers wait out one timeout together.
func (s *Server) listMcpServers(w http.ResponseWriter, r *http.Request) {
	servers, err := s.cfg.Store.McpServers(r.Context())
	if err != nil {
		internalError(w, r, err)
		return
	}
	views := make([]mcpServerView, len(servers))
	var wg sync.WaitGroup
	for i, m := range servers {
		wg.Go(func() { views[i] = s.probeMcp(r.Context(), m) })
	}
	wg.Wait()
	writeJSON(w, http.StatusOK, views)
}

// probeMcp lists a server's tools; a failure is data for the panel, not an error.
func (s *Server) probeMcp(ctx context.Context, m store.McpServer) mcpServerView {
	view := mcpServerView{McpServer: m, Tools: []string{}}
	token := ""
	if len(m.TokenEnc) > 0 {
		var err error
		if token, err = s.cfg.Box.Open(m.TokenEnc); err != nil {
			view.Error = "Jeton illisible, supprimer et recréer le serveur"
			return view
		}
	}
	probeCtx, cancel := context.WithTimeout(ctx, mcpProbeTimeout)
	defer cancel()
	names, err := mcp.ToolNames(probeCtx, m.URL, token)
	if err != nil {
		slog.Warn("mcp probe failed", "server", m.Name, "err", err)
		view.Error = mcpFailure(err)
		return view
	}
	view.Tools = names
	return view
}

// mcpFailure words a probe error for the panel; the full error goes to the log.
func mcpFailure(err error) string {
	var dnsErr *net.DNSError
	switch {
	case errors.Is(err, mcp.ErrUnauthorized):
		return "Jeton refusé ou manquant"
	case errors.Is(err, context.DeadlineExceeded):
		return fmt.Sprintf("Pas de réponse en %v", mcpProbeTimeout)
	case errors.Is(err, syscall.ECONNREFUSED):
		return "Connexion refusée, rien n'écoute à cette adresse"
	case errors.As(err, &dnsErr):
		return "Adresse introuvable"
	}
	return "Réponse inattendue, détail dans les journaux de l'application"
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

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
	"syscall"
	"time"

	"github.com/arkhayds/axolot/internal/mcp"
	"github.com/arkhayds/axolot/internal/store"
)

// mcpProbeTimeout covers connect plus tools/list, slower than a model listing over the internet.
const mcpProbeTimeout = 5 * time.Second

// mcpName becomes the prefix of the server's tools, and tool names accept only letters, digits, _ and -.
var mcpName = regexp.MustCompile(`^[a-z0-9_-]{1,32}$`)

// mcpProbe is what a server offers right now, or why it could not be reached.
type mcpProbe struct {
	Tools []string `json:"tools"`
	Error string   `json:"error,omitempty"`
	// Login: signing in from the browser would let the server answer.
	Login bool `json:"login"`
}

// listMcpServers returns the saved servers without contacting them, so the panel shows at once.
func (s *Server) listMcpServers(w http.ResponseWriter, r *http.Request) {
	servers, err := s.cfg.Store.McpServers(r.Context())
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, servers)
}

// probeMcpServer asks one server for its tools; the panel calls it per row, so a slow server delays only its own row.
func (s *Server) probeMcpServer(w http.ResponseWriter, r *http.Request) {
	m, ok := s.mcpServerByID(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, s.probeMcp(r.Context(), m))
}

// mcpServerByID reads the {id} path value and loads its server, answering the error itself.
func (s *Server) mcpServerByID(w http.ResponseWriter, r *http.Request) (store.McpServer, bool) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return store.McpServer{}, false
	}
	m, err := s.cfg.Store.McpServer(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "Serveur MCP introuvable, rouvrir le panneau")
		return store.McpServer{}, false
	}
	if err != nil {
		internalError(w, r, err)
		return store.McpServer{}, false
	}
	return m, true
}

// probeMcp lists a server's tools; a failure is data for the panel, not an error.
func (s *Server) probeMcp(ctx context.Context, m store.McpServer) mcpProbe {
	view := mcpProbe{Tools: []string{}}
	token, err := s.mcpToken(ctx, m)
	var names []string
	if err == nil {
		probeCtx, cancel := context.WithTimeout(ctx, mcpProbeTimeout)
		defer cancel()
		names, err = mcp.ToolNames(probeCtx, m.URL, token)
	}
	if err != nil {
		slog.Warn("mcp probe failed", "server", m.Name, "err", err)
		view.Error = mcpFailure(err)
		if signInNeeded(err, m) {
			view.Error, view.Login = "Connexion requise", true
		}
		return view
	}
	view.Tools = names
	return view
}

// mcpToken gives the bearer for a server: its fixed token first, else its browser sign-in, else none.
func (s *Server) mcpToken(ctx context.Context, m store.McpServer) (string, error) {
	switch {
	case len(m.TokenEnc) > 0:
		token, err := s.cfg.Box.Open(m.TokenEnc)
		if err != nil {
			return "", fmt.Errorf("%w: %v", errTokenUnreadable, err)
		}
		return token, nil
	case len(m.OAuthEnc) > 0:
		return s.oauthToken(ctx, m)
	}
	return "", nil
}

// errTokenUnreadable means the fixed token was sealed with another AXOLOT_SECRET.
var errTokenUnreadable = errors.New("token unreadable")

// signInNeeded tells whether only a browser sign-in can make the server answer; a fixed token is fixed by hand.
func signInNeeded(err error, m store.McpServer) bool {
	return errors.Is(err, errSignInLost) || (errors.Is(err, mcp.ErrSignInOffered) && len(m.TokenEnc) == 0)
}

// mcpFailure words a probe error for the panel; the full error goes to the log.
func mcpFailure(err error) string {
	var dnsErr *net.DNSError
	switch {
	case errors.Is(err, errTokenUnreadable):
		return "Jeton illisible, supprimer et recréer le serveur"
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
	err := s.cfg.Store.SetMcpServerEnabled(r.Context(), id, body.Enabled)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "Serveur MCP introuvable, rouvrir le panneau")
		return
	}
	if err != nil {
		internalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// deleteMcpServer removes a server and forgets its sign-in.
func (s *Server) deleteMcpServer(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	err := s.cfg.Store.DeleteMcpServer(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "Serveur MCP introuvable, rouvrir le panneau")
		return
	}
	if err != nil {
		internalError(w, r, err)
		return
	}
	s.oauthMu.Lock()
	delete(s.oauthSources, id)
	s.oauthMu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

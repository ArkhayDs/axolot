// Package api exposes the JSON and SSE endpoints used by the web UI, and serves the UI itself.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"sync"

	"github.com/arkhayds/axolot/cases"
	"github.com/arkhayds/axolot/internal/hardware"
	"github.com/arkhayds/axolot/internal/provider"
	"github.com/arkhayds/axolot/internal/runner"
	"github.com/arkhayds/axolot/internal/secret"
	"github.com/arkhayds/axolot/internal/store"
)

const maxJSONBody = 1 << 20

// Config holds what the server needs at startup.
type Config struct {
	Store      *store.Store
	Box        *secret.Box
	Cases      []cases.Case
	Hardware   hardware.Info
	CorpusRoot string
	Static     fs.FS
	// BaseContext outlives requests: campaigns and downloads continue after the HTTP call returns.
	BaseContext context.Context
}

// Server holds handlers and the single in-flight campaign, ingest and downloads.
type Server struct {
	cfg    Config
	hub    *hub
	mu     sync.Mutex
	cancel context.CancelFunc
	active string
	// What the active campaign was asked to play, for its remaining time.
	activePlans []runner.Plan
	ingest      bool
	chats       int
	pulls       map[string]pullState
}

// New wires the routes.
func New(cfg Config) http.Handler {
	s := &Server{cfg: cfg, hub: newHub(), pulls: map[string]pullState{}}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/status", s.status)
	mux.HandleFunc("GET /api/events", s.events)
	mux.HandleFunc("GET /api/settings", s.getSettings)
	mux.HandleFunc("PUT /api/settings", s.putSettings)
	mux.HandleFunc("GET /api/providers", s.listProviders)
	mux.HandleFunc("POST /api/providers", s.createProvider)
	mux.HandleFunc("DELETE /api/providers/{id}", s.deleteProvider)
	mux.HandleFunc("GET /api/models", s.listModels)
	mux.HandleFunc("POST /api/models/pull", s.pullModel)
	mux.HandleFunc("GET /api/cases", s.listCases)
	mux.HandleFunc("PUT /api/cases/{id}/models", s.setCaseModels)
	mux.HandleFunc("GET /api/corpus", s.listCorpus)
	mux.HandleFunc("POST /api/corpus", s.createCorpus)
	mux.HandleFunc("GET /api/corpus/{name}/files/{file}", s.readCorpusFile)
	mux.HandleFunc("POST /api/corpus/{name}/files", s.uploadCorpusFile)
	mux.HandleFunc("POST /api/corpus/ingest", s.ingestCorpus)
	mux.HandleFunc("GET /api/campaigns", s.listCampaigns)
	mux.HandleFunc("POST /api/campaigns", s.startCampaign)
	mux.HandleFunc("POST /api/campaigns/{id}/cancel", s.cancelCampaign)
	mux.HandleFunc("GET /api/campaigns/{id}", s.getCampaign)
	mux.HandleFunc("GET /api/map", s.getMap)
	mux.HandleFunc("GET /api/base", s.getBase)
	mux.HandleFunc("POST /api/estimate", s.postEstimate)
	mux.HandleFunc("GET /api/estimate/remaining", s.getRemaining)
	mux.HandleFunc("POST /api/chat", s.chat)
	mux.Handle("/", spa(cfg.Static))
	return sameOrigin(mux)
}

// sameOrigin blocks cross-site writes: the app has no login, so any web page could otherwise POST to it.
// shortcut: no authentication and no DNS-rebinding defense, compose binds to loopback only; add a login before any network exposure.
func sameOrigin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			if origin := r.Header.Get("Origin"); origin != "" {
				parsed, err := url.Parse(origin)
				if err != nil || parsed.Host != r.Host {
					writeError(w, http.StatusForbidden, "origine refusée")
					return
				}
			}
		}
		next.ServeHTTP(w, r)
	})
}

// spa serves built assets, falling back to index.html for client-side routes.
func spa(static fs.FS) http.Handler {
	files := http.FileServerFS(static)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			if _, err := fs.Stat(static, r.URL.Path[1:]); errors.Is(err, fs.ErrNotExist) {
				r.URL.Path = "/"
			}
		}
		files.ServeHTTP(w, r)
	})
}

// status gives the UI its startup view.
func (s *Server) status(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	active := s.active
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{"hardware": s.cfg.Hardware, "active_campaign": active})
}

// getSettings returns the current settings.
func (s *Server) getSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := s.cfg.Store.Settings(r.Context())
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, settings)
}

// putSettings validates and saves settings.
func (s *Server) putSettings(w http.ResponseWriter, r *http.Request) {
	var settings store.Settings
	if !readJSON(w, r, &settings) {
		return
	}
	if msg := validateSettings(settings); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	if err := s.cfg.Store.SaveSettings(r.Context(), settings); err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, settings)
}

// validateSettings returns a user-facing message, empty when valid.
func validateSettings(settings store.Settings) string {
	switch {
	case !corpusName.MatchString(settings.CorpusDir):
		return "Dossier de corpus invalide"
	case settings.ChunkSize < 100 || settings.ChunkSize > 8000:
		return "Taille de découpe : entre 100 et 8000"
	case settings.ChunkOverlap < 0 || settings.ChunkOverlap >= settings.ChunkSize:
		return "Chevauchement : doit rester sous la taille de découpe"
	case settings.TopK < 1 || settings.TopK > 20:
		return "Extraits retenus : entre 1 et 20"
	case settings.EmbedModel == "":
		return "Modèle d'embeddings requis"
	case settings.ReservedGB < 0 || settings.ReservedGB > 256:
		return "RAM réservée invalide"
	}
	return ""
}

// listCases returns cases with the models selected for each.
func (s *Server) listCases(w http.ResponseWriter, r *http.Request) {
	selections, err := s.cfg.Store.Selections(r.Context())
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"cases": s.cfg.Cases, "selections": selections})
}

// setCaseModels replaces the selection of a case.
func (s *Server) setCaseModels(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, ok := s.findCase(id); !ok {
		writeError(w, http.StatusNotFound, "Scénario inconnu")
		return
	}
	var selections []store.Selection
	if !readJSON(w, r, &selections) {
		return
	}
	if err := s.cfg.Store.SetCaseModels(r.Context(), id, selections); err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, selections)
}

// findCase looks a case up by id.
func (s *Server) findCase(id string) (cases.Case, bool) {
	for _, c := range s.cfg.Cases {
		if c.ID == id {
			return c, true
		}
	}
	return cases.Case{}, false
}

// connection builds a live client for a saved connection.
func (s *Server) connection(ctx context.Context, id int64) (provider.Provider, store.Provider, error) {
	saved, err := s.cfg.Store.Providers(ctx)
	if err != nil {
		return nil, store.Provider{}, err
	}
	for _, p := range saved {
		if p.ID != id {
			continue
		}
		client, err := s.client(p)
		return client, p, err
	}
	return nil, store.Provider{}, fmt.Errorf("connection %d not found", id)
}

// client decrypts the key of a connection and builds its client.
func (s *Server) client(p store.Provider) (provider.Provider, error) {
	apiKey := ""
	if p.HasAPIKey {
		var err error
		if apiKey, err = s.cfg.Box.Open(p.APIKeyEnc); err != nil {
			return nil, fmt.Errorf("read key of %s: %w", p.Name, err)
		}
	}
	return provider.New(p.Kind, p.BaseURL, apiKey)
}

// pathID parses a numeric path parameter, answering 400 when malformed.
func pathID(w http.ResponseWriter, r *http.Request, name string) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue(name), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Identifiant invalide")
		return 0, false
	}
	return id, true
}

// readJSON decodes a bounded body, answering 400 on failure.
func readJSON(w http.ResponseWriter, r *http.Request, out any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxJSONBody)
	if err := json.NewDecoder(r.Body).Decode(out); err != nil {
		writeError(w, http.StatusBadRequest, "Requête invalide")
		return false
	}
	return true
}

// writeJSON sends a JSON response.
func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		slog.Warn("write response", "err", err)
	}
}

// writeError sends a user-facing message.
func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

// internalError logs the cause and hides it from the client.
func internalError(w http.ResponseWriter, r *http.Request, err error) {
	slog.Error("request failed", "path", r.URL.Path, "err", err)
	writeError(w, http.StatusInternalServerError, "Erreur interne, voir les logs du serveur")
}

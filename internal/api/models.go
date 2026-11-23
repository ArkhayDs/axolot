package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/arkhayds/axolot/internal/catalog"
	"github.com/arkhayds/axolot/internal/hardware"
	"github.com/arkhayds/axolot/internal/provider"
	"github.com/arkhayds/axolot/internal/store"
)

// providerProbeTimeout keeps the page responsive when a server is down.
const providerProbeTimeout = 3 * time.Second

// modelTag restricts what reaches the model server's download endpoint.
var modelTag = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._/-]*(:[a-zA-Z0-9._-]+)?$`)

// pullState is the last known state of a download.
type pullState struct {
	Type       string `json:"type"`
	ProviderID int64  `json:"provider_id"`
	Name       string `json:"name"`
	provider.PullProgress
	Done  bool   `json:"done"`
	Error string `json:"error,omitempty"`
}

// listProviders returns connections; encrypted keys never leave the server.
func (s *Server) listProviders(w http.ResponseWriter, r *http.Request) {
	providers, err := s.cfg.Store.Providers(r.Context())
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, providers)
}

// createProvider saves a connection, encrypting its key.
func (s *Server) createProvider(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name    string `json:"name"`
		Kind    string `json:"kind"`
		BaseURL string `json:"base_url"`
		APIKey  string `json:"api_key"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	if !httpURL(body.BaseURL) {
		writeError(w, http.StatusBadRequest, "URL invalide")
		return
	}
	if strings.TrimSpace(body.Name) == "" || (body.Kind != "ollama" && body.Kind != "openai") {
		writeError(w, http.StatusBadRequest, "Nom ou type de connexion invalide")
		return
	}
	var sealed []byte
	if body.APIKey != "" {
		var err error
		if sealed, err = s.cfg.Box.Seal(body.APIKey); err != nil {
			internalError(w, r, err)
			return
		}
	}
	id, err := s.cfg.Store.CreateProvider(r.Context(), body.Name, body.Kind, strings.TrimRight(body.BaseURL, "/"), sealed)
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

// createBridge saves the Claude bridge connection without a key: client reads the token from the environment.
// shortcut: only the fixed name is unique, not the kind; the page locks the button until the reloaded list hides it; check kind in SQL if another client calls this.
func (s *Server) createBridge(w http.ResponseWriter, r *http.Request) {
	id, err := s.cfg.Store.CreateProvider(r.Context(), "Claude (abonnement)", "bridge", s.cfg.BridgeURL, nil)
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

// deleteProvider removes a connection.
func (s *Server) deleteProvider(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	if err := s.cfg.Store.DeleteProvider(r.Context(), id); err != nil {
		internalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// connectionModels is the state of one connection, shown as such when the server is down.
type connectionModels struct {
	Provider store.Provider `json:"provider"`
	Online   bool           `json:"online"`
	// ConnectionType spares the page a copy of connectionType.
	ConnectionType string `json:"connection_type"`
	// Bridge marks the Claude bridge, whose command the page gives when it is down.
	Bridge bool        `json:"bridge"`
	Error  string      `json:"error,omitempty"`
	Models []modelView `json:"models"`
}

// modelView adds the fit estimate for this machine and the label results name the model by.
type modelView struct {
	provider.Model
	Label string `json:"label"`
	Fit   string `json:"fit"`
}

// catalogView is a library entry with its fit and download state.
type catalogView struct {
	catalog.Entry
	Fit string `json:"fit"`
}

// listModels merges live connections, the library, and in-progress downloads.
func (s *Server) listModels(w http.ResponseWriter, r *http.Request) {
	settings, err := s.cfg.Store.Settings(r.Context())
	if err != nil {
		internalError(w, r, err)
		return
	}
	providers, err := s.cfg.Store.Providers(r.Context())
	if err != nil {
		internalError(w, r, err)
		return
	}
	connections := make([]connectionModels, 0, len(providers))
	for _, p := range providers {
		connections = append(connections, s.probe(r.Context(), p, settings.ReservedGB))
	}
	library := make([]catalogView, 0)
	for _, entry := range catalog.All() {
		library = append(library, catalogView{Entry: entry, Fit: hardware.FitFor(s.cfg.Hardware, entry.SizeBytes, settings.ReservedGB)})
	}
	s.mu.Lock()
	pulls := make([]pullState, 0, len(s.pulls))
	for _, state := range s.pulls {
		pulls = append(pulls, state)
	}
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{"connections": connections, "library": library, "pulls": pulls})
}

// probe asks one server for its models; a failure is data for the UI, not an error.
func (s *Server) probe(ctx context.Context, p store.Provider, reservedGB int) connectionModels {
	view := connectionModels{Provider: p, ConnectionType: connectionType(p), Bridge: p.Kind == "bridge", Models: []modelView{}}
	client, err := s.client(p)
	if err != nil {
		view.Error = err.Error()
		return view
	}
	probeCtx, cancel := context.WithTimeout(ctx, providerProbeTimeout)
	defer cancel()
	models, err := client.Models(probeCtx)
	if err != nil {
		view.Error = err.Error()
		return view
	}
	view.Online = true
	for _, m := range models {
		view.Models = append(view.Models, modelView{
			Model: m,
			Label: store.ModelLabel(m.Name, view.ConnectionType, p.Name),
			Fit:   hardware.FitFor(s.cfg.Hardware, m.SizeBytes, reservedGB),
		})
	}
	return view
}

// pullModel starts a background download; progress goes through the event stream.
func (s *Server) pullModel(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ProviderID int64  `json:"provider_id"`
		Name       string `json:"name"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	if !modelTag.MatchString(body.Name) {
		writeError(w, http.StatusBadRequest, "Nom de modèle invalide")
		return
	}
	client, _, err := s.connection(r.Context(), body.ProviderID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Connexion inconnue")
		return
	}
	puller, ok := client.(provider.Puller)
	if !ok {
		writeError(w, http.StatusBadRequest, "Cette connexion ne gère pas les téléchargements")
		return
	}
	s.mu.Lock()
	if state, running := s.pulls[body.Name]; running && !state.Done {
		s.mu.Unlock()
		writeError(w, http.StatusConflict, "Téléchargement déjà en cours")
		return
	}
	s.pulls[body.Name] = pullState{Type: "pull", ProviderID: body.ProviderID, Name: body.Name}
	s.mu.Unlock()
	go s.runPull(puller, body.ProviderID, body.Name)
	w.WriteHeader(http.StatusAccepted)
}

// runPull downloads a model and publishes each progress step.
func (s *Server) runPull(puller provider.Puller, providerID int64, name string) {
	publish := func(state pullState) {
		s.mu.Lock()
		s.pulls[name] = state
		s.mu.Unlock()
		s.hub.publish(state)
	}
	base := pullState{Type: "pull", ProviderID: providerID, Name: name}
	err := puller.Pull(s.cfg.BaseContext, name, func(progress provider.PullProgress) {
		state := base
		state.PullProgress = progress
		publish(state)
	})
	final := base
	final.Done = true
	if err != nil {
		slog.Warn("model download failed", "model", name, "err", err)
		final.Error = err.Error()
	}
	publish(final)
}

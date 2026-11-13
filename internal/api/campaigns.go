package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"regexp"
	"strings"

	"github.com/arkhayds/axolot/internal/analytics"
	"github.com/arkhayds/axolot/internal/provider"
	"github.com/arkhayds/axolot/internal/rag"
	"github.com/arkhayds/axolot/internal/runner"
	"github.com/arkhayds/axolot/internal/store"
)

var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F-]{36}$`)

// listCampaigns returns the history.
func (s *Server) listCampaigns(w http.ResponseWriter, r *http.Request) {
	campaigns, err := s.cfg.Store.Campaigns(r.Context())
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, campaigns)
}

// getCampaign returns a campaign with its runs, stats and verdicts.
func (s *Server) getCampaign(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !uuidPattern.MatchString(id) {
		writeError(w, http.StatusBadRequest, "Identifiant invalide")
		return
	}
	campaign, err := s.cfg.Store.CampaignByID(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "Benchmark introuvable")
		return
	}
	if err != nil {
		internalError(w, r, err)
		return
	}
	runs, err := s.cfg.Store.Runs(r.Context(), id)
	if err != nil {
		internalError(w, r, err)
		return
	}
	stats, err := s.cfg.Store.Stats(r.Context(), id)
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"campaign": campaign, "runs": runs, "stats": stats, "verdicts": analytics.Verdicts(stats),
	})
}

// startCampaign launches the saved selections, restricted to the given cases when provided.
func (s *Server) startCampaign(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name  string   `json:"name"`
		Cases []string `json:"cases"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	settings, err := s.cfg.Store.Settings(r.Context())
	if err != nil {
		internalError(w, r, err)
		return
	}
	plans, err := s.buildPlans(r.Context(), body.Cases)
	if err != nil {
		internalError(w, r, err)
		return
	}
	total := runner.CountItems(plans)
	if total == 0 {
		writeError(w, http.StatusBadRequest, "Choisissez au moins un modèle pour un scénario")
		return
	}
	if needsIndex(plans) {
		indexed, err := s.isIndexed(r.Context(), settings)
		if err != nil {
			internalError(w, r, err)
			return
		}
		if !indexed {
			writeError(w, http.StatusBadRequest, "Indexez d'abord le corpus dans Réglages")
			return
		}
	}
	embedder, err := s.embedConnection(r, settings)
	if err != nil && needsIndex(plans) {
		writeError(w, http.StatusBadRequest, "Aucune connexion d'embeddings disponible")
		return
	}

	hardwareRaw, ragRaw, err := s.campaignSnapshots(settings)
	if err != nil {
		internalError(w, r, err)
		return
	}
	name := strings.TrimSpace(body.Name)
	if name == "" {
		name = "Benchmark"
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.active != "" {
		writeError(w, http.StatusConflict, "Un Benchmark est déjà en cours")
		return
	}
	if s.chats > 0 {
		writeError(w, http.StatusConflict, "Une réponse de chat est en cours : elle fausserait les temps")
		return
	}
	id, err := s.cfg.Store.CreateCampaign(r.Context(), name, hardwareRaw, ragRaw, total)
	if err != nil {
		internalError(w, r, err)
		return
	}
	runCtx, cancel := context.WithCancel(s.cfg.BaseContext)
	s.active, s.cancel, s.activePlans = id, cancel, plans
	go s.runCampaign(runCtx, id, plans, settings, embedder)
	writeJSON(w, http.StatusAccepted, map[string]string{"id": id})
}

// campaignSnapshots freezes the hardware and RAG settings, so results stay interpretable after later changes.
func (s *Server) campaignSnapshots(settings store.Settings) (json.RawMessage, json.RawMessage, error) {
	hardwareRaw, err := json.Marshal(s.cfg.Hardware)
	if err != nil {
		return nil, nil, err
	}
	ragRaw, err := json.Marshal(settings)
	if err != nil {
		return nil, nil, err
	}
	return hardwareRaw, ragRaw, nil
}

// buildPlans joins saved selections with case definitions.
func (s *Server) buildPlans(ctx context.Context, only []string) ([]runner.Plan, error) {
	selections, err := s.cfg.Store.Selections(ctx)
	if err != nil {
		return nil, err
	}
	providers, err := s.cfg.Store.Providers(ctx)
	if err != nil {
		return nil, err
	}
	byID := make(map[int64]store.Provider, len(providers))
	for _, p := range providers {
		byID[p.ID] = p
	}
	wanted := make(map[string]bool, len(only))
	for _, id := range only {
		wanted[id] = true
	}
	var plans []runner.Plan
	for _, c := range s.cfg.Cases {
		if len(only) > 0 && !wanted[c.ID] {
			continue
		}
		plan := runner.Plan{Case: c}
		for _, sel := range selections {
			if sel.CaseID == c.ID {
				p := byID[sel.ProviderID]
				plan.Targets = append(plan.Targets, runner.Target{ProviderID: sel.ProviderID, ProviderName: p.Name, ConnectionType: connectionType(p), Model: sel.Model})
			}
		}
		if len(plan.Targets) > 0 {
			plans = append(plans, plan)
		}
	}
	return plans, nil
}

// needsIndex reports whether a plan includes a RAG case.
func needsIndex(plans []runner.Plan) bool {
	for _, plan := range plans {
		if plan.Case.Kind == "rag" {
			return true
		}
	}
	return false
}

// isIndexed checks the configured corpus was indexed with the configured embedding model.
func (s *Server) isIndexed(ctx context.Context, settings store.Settings) (bool, error) {
	indexes, err := s.cfg.Store.CorpusIndexes(ctx)
	if err != nil {
		return false, err
	}
	for _, index := range indexes {
		if index.Corpus == settings.CorpusDir && index.EmbedModel == settings.EmbedModel {
			return true, nil
		}
	}
	return false, nil
}

// runCampaign drives the runner and releases the campaign slot.
func (s *Server) runCampaign(ctx context.Context, id string, plans []runner.Plan, settings store.Settings, embedder provider.Provider) {
	defer func() {
		s.mu.Lock()
		s.active, s.cancel, s.activePlans = "", nil, nil
		s.mu.Unlock()
	}()
	deps := runner.Deps{
		Results: s.cfg.Store,
		Provider: func(ctx context.Context, providerID int64) (provider.Provider, error) {
			client, _, err := s.connection(ctx, providerID)
			return client, err
		},
		Search: func(ctx context.Context, question string) ([]rag.Hit, error) {
			return rag.Search(ctx, s.cfg.Store, embedder, settings.EmbedModel, settings.CorpusDir, question, settings.TopK)
		},
		Emit: func(event runner.Event) { s.hub.publish(event) },
	}
	if err := runner.Run(ctx, id, plans, deps); err != nil {
		slog.Error("campaign failed", "campaign", id, "err", err)
	}
}

// cancelCampaign stops the running campaign; the current item is abandoned.
func (s *Server) cancelCampaign(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.active != r.PathValue("id") || s.cancel == nil {
		writeError(w, http.StatusConflict, "Ce Benchmark n'est pas en cours")
		return
	}
	s.cancel()
	w.WriteHeader(http.StatusAccepted)
}

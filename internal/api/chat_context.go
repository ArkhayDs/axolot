package api

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/arkhayds/axolot/internal/rag"
)

// chatSearch retrieves the configured corpus' passages once for a whole turn, timed apart from the models.
func (s *Server) chatSearch(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Question string `json:"question"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	if strings.TrimSpace(body.Question) == "" {
		writeError(w, http.StatusBadRequest, "Question vide")
		return
	}
	settings, err := s.cfg.Store.Settings(r.Context())
	if err != nil {
		internalError(w, r, err)
		return
	}
	embedder, err := s.embedConnection(r, settings)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Aucune connexion de modèles disponible pour la recherche")
		return
	}
	if !s.claimChat() {
		writeError(w, http.StatusConflict, "Un Benchmark est en cours : le chat fausserait ses temps")
		return
	}
	defer s.releaseChat()
	ctx, cancel := context.WithTimeout(r.Context(), chatTimeout)
	defer cancel()
	start := time.Now()
	hits, err := rag.Search(ctx, s.cfg.Store, embedder, settings.EmbedModel, settings.CorpusDir, body.Question, settings.TopK)
	if err != nil {
		slog.Warn("chat search failed", "corpus", settings.CorpusDir, "err", err)
		writeError(w, http.StatusBadGateway, fmt.Sprintf("Recherche impossible avec le modèle %s : vérifiez qu'il est installé", settings.EmbedModel))
		return
	}
	if len(hits) == 0 {
		writeError(w, http.StatusConflict, fmt.Sprintf("Le corpus « %s » n'est pas indexé avec %s : lancez l'indexation dans Réglages", settings.CorpusDir, settings.EmbedModel))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"passages": hits, "search_ms": time.Since(start).Milliseconds()})
}

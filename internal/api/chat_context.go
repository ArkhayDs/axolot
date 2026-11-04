package api

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"path"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/arkhayds/axolot/internal/provider"
	"github.com/arkhayds/axolot/internal/rag"
)

const (
	// chatReserveTokens leaves room for the question, the passages and a short history.
	chatReserveTokens = 2048
	// chatDocumentRunes bounds an attached document; 3 runes per token is pessimistic for French prose.
	chatDocumentRunes = (provider.ContextWindow - chatMaxTokens - chatReserveTokens) * 3
	chatCutNotice     = "\n\n[Document tronqué : la suite n'a pas été transmise.]"
)

// readChatFile reads the uploaded file of a multipart body into memory.
// The limit stays under FormFile's 32 Mo memory threshold, so nothing reaches the disk.
// @param limit - largest accepted body
// @param tooLarge - message when the body is missing or over the limit
func readChatFile(w http.ResponseWriter, r *http.Request, limit int64, tooLarge string) (string, []byte, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	source, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, tooLarge)
		return "", nil, false
	}
	defer source.Close()
	name := path.Base(strings.ReplaceAll(header.Filename, `\`, "/"))
	raw, err := io.ReadAll(source)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Fichier illisible : "+name)
		return "", nil, false
	}
	return name, raw, true
}

// fitDocument cuts a text to what the context window can hold.
// @returns kept text, runes kept from the document, whether it was cut
func fitDocument(text string) (string, int, bool) {
	total := utf8.RuneCountInString(text)
	if total <= chatDocumentRunes {
		return text, total, false
	}
	keep := chatDocumentRunes - utf8.RuneCountInString(chatCutNotice)
	return string([]rune(text)[:keep]) + chatCutNotice, keep, true
}

// chatDocument extracts the text of a document for one turn; it is never stored.
func (s *Server) chatDocument(w http.ResponseWriter, r *http.Request) {
	name, raw, ok := readChatFile(w, r, maxUpload, "Document manquant ou trop volumineux (20 Mo max)")
	if !ok {
		return
	}
	if !rag.Supported(name) {
		writeError(w, http.StatusBadRequest, "Formats acceptés : "+strings.Join(rag.Extensions(), ", "))
		return
	}
	text, err := rag.Extract(name, raw)
	if err == nil && strings.TrimSpace(text) == "" {
		err = errors.New("no text")
	}
	if err != nil {
		rejectDocument(w, r, name, err)
		return
	}
	kept, chars, truncated := fitDocument(text)
	writeJSON(w, http.StatusOK, map[string]any{
		"name": name, "text": kept, "chars": chars, "total_chars": utf8.RuneCountInString(text), "truncated": truncated,
	})
}

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

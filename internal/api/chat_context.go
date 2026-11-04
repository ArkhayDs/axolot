package api

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"path"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/arkhayds/axolot/internal/provider"
	"github.com/arkhayds/axolot/internal/rag"
)

const (
	// chatReserveTokens leaves room for the question, the passages and a short history.
	chatReserveTokens = 2048
	// chatDocumentCost bounds an attached document, in the units of runeCost.
	chatDocumentCost = (provider.ContextWindow - chatMaxTokens - chatReserveTokens) * tokenCost
	tokenCost        = 3
	chatCutNotice    = "\n\n[Document tronqué : la suite n'a pas été transmise.]"
	chatMaxImage     = 10 << 20
	// chatImageOverhead lets a file of exactly the limit through, with its multipart envelope.
	chatImageOverhead = 1 << 16
)

// chatImageTypes are the formats model servers decode.
var chatImageTypes = []string{"image/png", "image/jpeg", "image/webp"}

// validateChatImage returns a user-facing message, empty when the base64 holds an accepted picture.
// The content is sniffed: a name or a client type proves nothing.
func validateChatImage(encoded string) string {
	if base64.StdEncoding.DecodedLen(len(encoded)) > chatMaxImage+2 {
		return "Image trop volumineuse (10 Mo max)"
	}
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || !slices.Contains(chatImageTypes, http.DetectContentType(raw)) {
		return "Ce fichier n'est pas une image PNG, JPEG ou WebP"
	}
	if len(raw) > chatMaxImage {
		return "Image trop volumineuse (10 Mo max)"
	}
	return ""
}

// chatImage checks a picture for one turn and returns it in base64; it is never stored.
func (s *Server) chatImage(w http.ResponseWriter, r *http.Request) {
	name, raw, ok := readChatFile(w, r, chatMaxImage+chatImageOverhead, "Image manquante ou trop volumineuse (10 Mo max)")
	if !ok {
		return
	}
	encoded := base64.StdEncoding.EncodeToString(raw)
	if msg := validateChatImage(encoded); msg != "" {
		writeError(w, http.StatusUnprocessableEntity, msg+" : "+name)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"name": name, "media_type": http.DetectContentType(raw), "size": len(raw), "data": encoded})
}

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

// runeCost estimates a rune's share of a token, in thirds, without a tokenizer.
// Measured on gemma4 and qwen3.6: French prose costs under 1/5 token per rune, but a digit or a separator a whole one.
// An overflowing prompt is cut silently by Ollama, from its start, so the estimate stays pessimistic.
func runeCost(r rune) int {
	if unicode.IsLetter(r) || unicode.IsSpace(r) {
		return 1
	}
	return tokenCost
}

// documentCost sums runeCost over a text.
func documentCost(text string) int {
	cost := 0
	for _, r := range text {
		cost += runeCost(r)
	}
	return cost
}

// fitDocument cuts a text to what the context window can hold.
// @returns kept text, runes kept from the document, whether it was cut
func fitDocument(text string) (string, int, bool) {
	if documentCost(text) <= chatDocumentCost {
		return text, utf8.RuneCountInString(text), false
	}
	budget := chatDocumentCost - documentCost(chatCutNotice)
	chars := 0
	for i, r := range text {
		budget -= runeCost(r)
		if budget < 0 {
			return text[:i] + chatCutNotice, chars, true
		}
		chars++
	}
	return text, chars, false
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

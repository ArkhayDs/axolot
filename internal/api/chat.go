package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/arkhayds/axolot/internal/provider"
)

const (
	chatTimeout   = 5 * time.Minute
	chatMaxTokens = 2048
	chatMaxTurns  = 40
)

// chatRequest is one free question sent to one model.
type chatRequest struct {
	ProviderID int64              `json:"provider_id"`
	Model      string             `json:"model"`
	Messages   []provider.Message `json:"messages"`
}

// validateChat returns a user-facing message, empty when valid.
// The system role is refused: the page tests the model as it is, without a hidden instruction.
func validateChat(body chatRequest) string {
	if body.Model == "" {
		return "Choisissez un modèle"
	}
	if len(body.Messages) == 0 || len(body.Messages) > chatMaxTurns {
		return "Conversation vide ou trop longue"
	}
	for _, message := range body.Messages {
		if message.Role != "user" && message.Role != "assistant" {
			return "Rôle de message invalide"
		}
		if strings.TrimSpace(message.Content) == "" {
			return "Message vide"
		}
	}
	if body.Messages[len(body.Messages)-1].Role != "user" {
		return "Le dernier message doit être une question"
	}
	return ""
}

// claimChat reserves the GPU for a chat answer; it refuses while a campaign runs, whose timings would be falsified.
func (s *Server) claimChat() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.active != "" {
		return false
	}
	s.chats++
	return true
}

// releaseChat ends a reservation made by claimChat.
func (s *Server) releaseChat() {
	s.mu.Lock()
	s.chats--
	s.mu.Unlock()
}

// chat streams one answer as newline-delimited JSON, and stops when the browser cancels.
func (s *Server) chat(w http.ResponseWriter, r *http.Request) {
	var body chatRequest
	if !readJSON(w, r, &body) {
		return
	}
	if msg := validateChat(body); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "Flux non supporté")
		return
	}
	client, _, err := s.connection(r.Context(), body.ProviderID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Connexion inconnue")
		return
	}
	if !s.claimChat() {
		writeError(w, http.StatusConflict, "Un Benchmark est en cours : le chat fausserait ses temps")
		return
	}
	defer s.releaseChat()

	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("Cache-Control", "no-cache")
	send := func(event map[string]any) {
		line, err := json.Marshal(event)
		if err != nil {
			return
		}
		// A failed write means the browser left; the context stops the generation.
		_, _ = fmt.Fprintf(w, "%s\n", line)
		flusher.Flush()
	}
	ctx, cancel := context.WithTimeout(r.Context(), chatTimeout)
	defer cancel()
	result, err := client.Chat(ctx, provider.ChatRequest{Model: body.Model, Messages: body.Messages, MaxTokens: chatMaxTokens}, func(text string) {
		send(map[string]any{"type": "delta", "text": text})
	})
	if err != nil {
		send(map[string]any{"type": "error", "error": err.Error()})
		return
	}
	send(map[string]any{
		"type": "done", "ttft_ms": result.TTFTMs, "total_ms": result.TotalMs, "load_ms": result.LoadMs,
		"output_tokens": result.OutputTokens, "tok_per_sec": result.TokPerSec,
	})
}

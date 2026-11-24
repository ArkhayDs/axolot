package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/arkhayds/axolot/internal/provider"
	"github.com/arkhayds/axolot/internal/rag"
	"github.com/arkhayds/axolot/internal/store"
)

const (
	chatTimeout   = 5 * time.Minute
	chatMaxTokens = 2048
	chatMaxTurns  = 40
	// chatMaxPassages matches the highest top_k the settings accept.
	chatMaxPassages = 20
	// chatMaxBody fits one image in base64 on top of the regular JSON limit.
	chatMaxBody = maxJSONBody + (chatMaxImage/3+1)*4
)

// chatRequest is one free question sent to one model.
// Passages come from chatSearch, once per turn, so every model of a turn reads the same excerpts.
type chatRequest struct {
	ProviderID int64              `json:"provider_id"`
	Model      string             `json:"model"`
	Messages   []provider.Message `json:"messages"`
	Passages   []rag.Hit          `json:"passages"`
	Document   *chatAttachment    `json:"document"`
	Image      string             `json:"image"`
}

// chatAttachment is the text of a document read for one turn, sent back by the client since nothing is kept on the server.
type chatAttachment struct {
	Name string `json:"name"`
	Text string `json:"text"`
}

// validateChat returns a user-facing message, empty when valid.
// The system role is refused: only the server adds an instruction, the RAG prompt when asked for.
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
		if len(message.Images) > 0 {
			return "Image refusée hors de la question courante"
		}
		if strings.TrimSpace(message.Content) == "" {
			return "Message vide"
		}
	}
	if body.Messages[len(body.Messages)-1].Role != "user" {
		return "Le dernier message doit être une question"
	}
	if len(body.Passages) > chatMaxPassages {
		return "Trop de passages"
	}
	if doc := body.Document; doc != nil && (strings.TrimSpace(doc.Text) == "" || documentCost(doc.Text) > chatDocumentCost) {
		return "Document vide ou trop long"
	}
	if body.Image != "" {
		return validateChatImage(body.Image)
	}
	return ""
}

// chatMessages assembles what the model reads: excerpts go with the current question only, past turns stay as they were asked.
// The RAG prompt comes with passages only: a document alone leaves the model free of any instruction.
func chatMessages(body chatRequest) []provider.Message {
	last := len(body.Messages) - 1
	excerpts := body.Passages
	if body.Document != nil {
		excerpts = append([]rag.Hit{{Chunk: rag.Chunk{File: body.Document.Name, Content: body.Document.Text}}}, excerpts...)
	}
	current := body.Messages[last]
	messages := make([]provider.Message, 0, len(body.Messages)+1)
	if len(excerpts) > 0 {
		built := rag.BuildMessages("", current.Content, excerpts)
		if len(body.Passages) > 0 {
			messages = append(messages, built[0])
		}
		current = built[1]
	}
	if body.Image != "" {
		current.Images = []string{body.Image}
	}
	messages = append(messages, body.Messages[:last]...)
	return append(messages, current)
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
	if !readJSONLimit(w, r, &body, chatMaxBody) {
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
	client, conn, err := s.connection(r.Context(), body.ProviderID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Connexion inconnue")
		return
	}
	// shortcut: the bridge gets no tools until it can hand them to the Claude CLI.
	var servers []store.McpServer
	if conn.Kind != "bridge" {
		if servers, err = s.cfg.Store.McpServers(r.Context()); err != nil {
			internalError(w, r, err)
			return
		}
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
	tools := s.openToolbox(ctx, servers)
	defer tools.close()
	for _, warning := range tools.warnings {
		send(map[string]any{"type": "notice", "text": warning})
	}
	result, err := answer(ctx, client, provider.ChatRequest{Model: body.Model, Messages: chatMessages(body), MaxTokens: chatMaxTokens}, tools, send)
	if err != nil {
		send(map[string]any{"type": "error", "error": err.Error()})
		return
	}
	send(map[string]any{
		"type": "done", "ttft_ms": result.TTFTMs, "total_ms": result.TotalMs, "load_ms": result.LoadMs,
		"prompt_tokens": result.PromptTokens, "output_tokens": result.OutputTokens, "tok_per_sec": result.TokPerSec,
	})
}

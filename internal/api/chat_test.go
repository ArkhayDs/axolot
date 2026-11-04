package api

import (
	"strings"
	"testing"

	"github.com/arkhayds/axolot/internal/provider"
	"github.com/arkhayds/axolot/internal/rag"
)

func TestValidateChat(t *testing.T) {
	user := provider.Message{Role: "user", Content: "Bonjour"}
	assistant := provider.Message{Role: "assistant", Content: "Bonjour"}
	tooLong := make([]provider.Message, chatMaxTurns+1)
	for i := range tooLong {
		tooLong[i] = user
	}
	tests := []struct {
		name  string
		body  chatRequest
		valid bool
	}{
		{"question", chatRequest{Model: "m", Messages: []provider.Message{user}}, true},
		{"follow-up", chatRequest{Model: "m", Messages: []provider.Message{user, assistant, user}}, true},
		{"no model", chatRequest{Messages: []provider.Message{user}}, false},
		{"no message", chatRequest{Model: "m"}, false},
		{"too long", chatRequest{Model: "m", Messages: tooLong}, false},
		{"system role", chatRequest{Model: "m", Messages: []provider.Message{{Role: "system", Content: "x"}, user}}, false},
		{"blank message", chatRequest{Model: "m", Messages: []provider.Message{{Role: "user", Content: strings.Repeat(" ", 3)}}}, false},
		{"ends with assistant", chatRequest{Model: "m", Messages: []provider.Message{user, assistant}}, false},
		{"too many passages", chatRequest{Model: "m", Messages: []provider.Message{user}, Passages: make([]rag.Hit, chatMaxPassages+1)}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := validateChat(tt.body) == ""; got != tt.valid {
				t.Errorf("valid = %v, want %v", got, tt.valid)
			}
		})
	}
}

func TestChatMessagesWithoutOptionsKeepsTheConversation(t *testing.T) {
	messages := []provider.Message{{Role: "user", Content: "Bonjour"}}
	got := chatMessages(chatRequest{Messages: messages})
	if len(got) != 1 || got[0].Content != "Bonjour" {
		t.Fatalf("messages = %+v", got)
	}
}

func TestChatMessagesSendsExcerptsWithTheCurrentQuestionOnly(t *testing.T) {
	got := chatMessages(chatRequest{
		Messages: []provider.Message{
			{Role: "user", Content: "Question passée"},
			{Role: "assistant", Content: "Réponse passée"},
			{Role: "user", Content: "Question courante"},
		},
		Passages: []rag.Hit{{Chunk: rag.Chunk{File: "conges.md", Content: "Vingt-cinq jours"}}},
	})
	if len(got) != 4 || got[0].Role != "system" {
		t.Fatalf("messages = %+v", got)
	}
	if got[1].Content != "Question passée" || got[2].Content != "Réponse passée" {
		t.Errorf("past turns changed: %+v", got[1:3])
	}
	if !strings.Contains(got[3].Content, "Vingt-cinq jours") || !strings.Contains(got[3].Content, "Question courante") {
		t.Errorf("current question = %q", got[3].Content)
	}
}

func TestClaimChatRefusedDuringCampaign(t *testing.T) {
	s := &Server{active: "campaign"}
	if s.claimChat() {
		t.Fatal("chat accepted while a campaign runs")
	}
	s.active = ""
	if !s.claimChat() {
		t.Fatal("chat refused with no campaign")
	}
	if s.chats != 1 {
		t.Fatalf("chats = %d, want 1", s.chats)
	}
	s.releaseChat()
	if s.chats != 0 {
		t.Fatalf("chats = %d after release, want 0", s.chats)
	}
}

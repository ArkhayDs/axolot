package api

import (
	"strings"
	"testing"

	"github.com/arkhayds/axolot/internal/provider"
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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := validateChat(tt.body) == ""; got != tt.valid {
				t.Errorf("valid = %v, want %v", got, tt.valid)
			}
		})
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

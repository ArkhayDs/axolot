package api

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"unicode/utf8"

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
		{"document", chatRequest{Model: "m", Messages: []provider.Message{user}, Document: &chatAttachment{Name: "a.md", Text: "texte"}}, true},
		{"blank document", chatRequest{Model: "m", Messages: []provider.Message{user}, Document: &chatAttachment{Name: "a.md", Text: " "}}, false},
		{"oversized document", chatRequest{Model: "m", Messages: []provider.Message{user}, Document: &chatAttachment{Name: "a.md", Text: strings.Repeat("é", chatDocumentCost+1)}}, false},
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

func TestChatMessagesWithDocumentAloneAddsNoInstruction(t *testing.T) {
	got := chatMessages(chatRequest{
		Messages: []provider.Message{{Role: "user", Content: "Résume"}},
		Document: &chatAttachment{Name: "note.md", Text: "Contenu joint"},
	})
	if len(got) != 1 || got[0].Role != "user" {
		t.Fatalf("messages = %+v", got)
	}
	if !strings.Contains(got[0].Content, "[note.md]") || !strings.Contains(got[0].Content, "Contenu joint") {
		t.Errorf("content = %q", got[0].Content)
	}
}

func TestChatMessagesWithDocumentAndPassagesKeepsBoth(t *testing.T) {
	got := chatMessages(chatRequest{
		Messages: []provider.Message{{Role: "user", Content: "Compare"}},
		Document: &chatAttachment{Name: "note.md", Text: "Contenu joint"},
		Passages: []rag.Hit{{Chunk: rag.Chunk{File: "conges.md", Content: "Vingt-cinq jours"}}},
	})
	if len(got) != 2 || got[0].Role != "system" {
		t.Fatalf("messages = %+v", got)
	}
	if !strings.Contains(got[1].Content, "Contenu joint") || !strings.Contains(got[1].Content, "Vingt-cinq jours") {
		t.Errorf("content = %q", got[1].Content)
	}
}

func TestFitDocument(t *testing.T) {
	atLimit := strings.Repeat("é", chatDocumentCost)
	if kept, chars, truncated := fitDocument(atLimit); kept != atLimit || chars != chatDocumentCost || truncated {
		t.Errorf("text at the limit changed: chars %d, truncated %v", chars, truncated)
	}
	kept, chars, truncated := fitDocument(atLimit + "é")
	if !truncated || !strings.HasSuffix(kept, chatCutNotice) || !utf8.ValidString(kept) {
		t.Errorf("cut text: truncated %v, valid %v", truncated, utf8.ValidString(kept))
	}
	if documentCost(kept) > chatDocumentCost || chars >= chatDocumentCost {
		t.Errorf("cut text costs %d, chars %d", documentCost(kept), chars)
	}
}

func TestFitDocumentKeepsLessOfANumericTable(t *testing.T) {
	_, prose, _ := fitDocument(strings.Repeat("a", chatDocumentCost*2))
	_, table, truncated := fitDocument(strings.Repeat("1;", chatDocumentCost))
	if !truncated || table*tokenCost > prose {
		t.Errorf("table kept %d runes, prose %d", table, prose)
	}
}

// postFile calls an upload handler with one file in a multipart body.
func postFile(t *testing.T, handler http.HandlerFunc, name string, content []byte) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	part, err := form.CreateFormFile("file", name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatal(err)
	}
	form.Close()
	req := httptest.NewRequest(http.MethodPost, "/", &body)
	req.Header.Set("Content-Type", form.FormDataContentType())
	rec := httptest.NewRecorder()
	handler(rec, req)
	return rec
}

func TestChatDocumentExtractsAndTruncates(t *testing.T) {
	rec := postFile(t, (&Server{}).chatDocument, "long note.txt", []byte(strings.Repeat("a", chatDocumentCost+10)))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	var got struct {
		Name       string `json:"name"`
		Truncated  bool   `json:"truncated"`
		TotalChars int    `json:"total_chars"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Name != "long note.txt" || !got.Truncated || got.TotalChars != chatDocumentCost+10 {
		t.Errorf("response = %+v", got)
	}
}

func TestChatDocumentRefusals(t *testing.T) {
	tests := []struct {
		name    string
		file    string
		content []byte
		status  int
		message string
	}{
		{"broken pdf", "cassé.pdf", []byte("pas un pdf"), http.StatusUnprocessableEntity, "Document illisible"},
		{"empty text", "vide.md", []byte("  \n"), http.StatusUnprocessableEntity, "Document illisible"},
		{"not UTF-8", "latin.txt", []byte{0xe9, 0xe8}, http.StatusUnprocessableEntity, "Document illisible"},
		{"unknown format", "photo.exe", []byte("x"), http.StatusBadRequest, "Formats acceptés"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := postFile(t, (&Server{}).chatDocument, tt.file, tt.content)
			if rec.Code != tt.status || !strings.Contains(rec.Body.String(), tt.message) {
				t.Errorf("status = %d, body = %s", rec.Code, rec.Body)
			}
		})
	}
}

// pngHeader is enough for the content sniffing to recognize a PNG.
var pngHeader = []byte("\x89PNG\r\n\x1a\nrest")

func TestValidateChatImage(t *testing.T) {
	encode := base64.StdEncoding.EncodeToString
	tests := []struct {
		name    string
		encoded string
		valid   bool
	}{
		{"png", encode(pngHeader), true},
		{"jpeg", encode([]byte("\xff\xd8\xff\xe0rest")), true},
		{"text renamed as image", encode([]byte("bonjour")), false},
		{"svg", encode([]byte("<svg xmlns='http://www.w3.org/2000/svg'></svg>")), false},
		{"not base64", "%%%", false},
		{"over the limit", encode(append(pngHeader, make([]byte, chatMaxImage)...)), false},
		{"at the limit", encode(append(pngHeader, make([]byte, chatMaxImage-len(pngHeader))...)), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := validateChatImage(tt.encoded) == ""; got != tt.valid {
				t.Errorf("valid = %v, want %v", got, tt.valid)
			}
		})
	}
}

func TestValidateChatRefusesAnImageSmuggledInHistory(t *testing.T) {
	body := chatRequest{Model: "m", Messages: []provider.Message{{Role: "user", Content: "x", Images: []string{"abc"}}}}
	if validateChat(body) == "" {
		t.Fatal("image accepted inside messages")
	}
}

func TestChatMessagesPutsTheImageOnTheCurrentQuestionOnly(t *testing.T) {
	got := chatMessages(chatRequest{
		Messages: []provider.Message{
			{Role: "user", Content: "Avant"},
			{Role: "assistant", Content: "Réponse"},
			{Role: "user", Content: "Que vois-tu ?"},
		},
		Image: "abc",
	})
	if len(got) != 3 || len(got[0].Images) != 0 || len(got[2].Images) != 1 || got[2].Content != "Que vois-tu ?" {
		t.Fatalf("messages = %+v", got)
	}
}

func TestChatImageUpload(t *testing.T) {
	rec := postFile(t, (&Server{}).chatImage, "capture.txt", pngHeader)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "image/png") {
		t.Errorf("png by content: status %d, body %s", rec.Code, rec.Body)
	}
	rec = postFile(t, (&Server{}).chatImage, "faux.png", []byte("bonjour"))
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("text named png: status %d", rec.Code)
	}
	rec = postFile(t, (&Server{}).chatImage, "lourd.png", append(pngHeader, make([]byte, chatMaxImage+chatImageOverhead)...))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("oversized body: status %d", rec.Code)
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

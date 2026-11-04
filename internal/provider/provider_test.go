package provider

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const noThinkingStream = `{"message":{"content":"Bon"},"done":false}
{"message":{"content":"jour"},"done":false}
{"message":{"content":""},"done":true,"load_duration":2000000000,"prompt_eval_count":12,"eval_count":40,"eval_duration":2000000000}
`

func newChat() ChatRequest {
	return ChatRequest{Model: "m", Messages: []Message{{Role: "user", Content: "x"}}, MaxTokens: 10}
}

func TestOllamaChatReadsStreamAndMetrics(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, noThinkingStream)
	}))
	defer srv.Close()

	var deltas []string
	result, err := (&Ollama{BaseURL: srv.URL}).Chat(context.Background(), newChat(), func(d string) { deltas = append(deltas, d) })
	if err != nil {
		t.Fatal(err)
	}
	if result.Output != "Bonjour" || len(deltas) != 2 {
		t.Fatalf("output %q deltas %v", result.Output, deltas)
	}
	if *result.LoadMs != 2000 || *result.OutputTokens != 40 || *result.TokPerSec != 20 || result.TTFTMs == nil {
		t.Fatalf("metrics: %+v", result)
	}
}

func TestOllamaChatRetriesWithoutThinkOption(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		body, _ := io.ReadAll(r.Body)
		if strings.Contains(string(body), `"think"`) {
			http.Error(w, `{"error":"m does not support thinking"}`, http.StatusBadRequest)
			return
		}
		fmt.Fprint(w, noThinkingStream)
	}))
	defer srv.Close()

	result, err := (&Ollama{BaseURL: srv.URL}).Chat(context.Background(), newChat(), func(string) {})
	if err != nil || result.Output != "Bonjour" || calls != 2 {
		t.Fatalf("calls=%d output=%q err=%v", calls, result.Output, err)
	}
}

func TestOllamaChatSurfacesStreamError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, `{"message":{"content":"Bon"},"done":false}`)
		fmt.Fprintln(w, `{"error":"out of memory"}`)
	}))
	defer srv.Close()

	result, err := (&Ollama{BaseURL: srv.URL}).Chat(context.Background(), newChat(), func(string) {})
	if err == nil || !strings.Contains(err.Error(), "out of memory") {
		t.Fatalf("err=%v", err)
	}
	if result.Output != "Bon" {
		t.Fatalf("partial output lost: %q", result.Output)
	}
}

func TestOpenAIChatSendsKeySchemaAndReadsUsage(t *testing.T) {
	var gotAuth string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"Oui\"}}]}\n\n")
		fmt.Fprint(w, "data: {\"choices\":[],\"usage\":{\"prompt_tokens\":3,\"completion_tokens\":7}}\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	req := newChat()
	req.Schema = json.RawMessage(`{"type":"object"}`)
	result, err := (&OpenAI{BaseURL: srv.URL, APIKey: "secret"}).Chat(context.Background(), req, func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	if gotAuth != "Bearer secret" || gotBody["response_format"] == nil {
		t.Fatalf("auth=%q body=%v", gotAuth, gotBody)
	}
	if result.Output != "Oui" || *result.PromptTokens != 3 || *result.OutputTokens != 7 {
		t.Fatalf("result: %+v", result)
	}
}

func TestEmbedRejectsCountMismatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"embeddings":[[0.1,0.2]]}`)
	}))
	defer srv.Close()

	if _, err := (&Ollama{BaseURL: srv.URL}).Embed(context.Background(), "m", []string{"a", "b"}); err == nil {
		t.Fatal("expected count mismatch error")
	}
}

func TestPullReportsProgressAndError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, `{"status":"downloading","completed":5,"total":10}`)
		fmt.Fprintln(w, `{"error":"manifest not found"}`)
	}))
	defer srv.Close()

	var steps []PullProgress
	err := (&Ollama{BaseURL: srv.URL}).Pull(context.Background(), "m", func(p PullProgress) { steps = append(steps, p) })
	if err == nil || len(steps) != 1 || steps[0].Total != 10 {
		t.Fatalf("err=%v steps=%v", err, steps)
	}
}

// captureChat runs one chat against a fake server and returns the decoded request body.
func captureChat(t *testing.T, build func(url string) Provider, messages []Message) map[string]any {
	t.Helper()
	var got map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Error(err)
		}
		fmt.Fprintln(w, `{"done":true}`)
	}))
	defer server.Close()
	if _, err := build(server.URL).Chat(context.Background(), ChatRequest{Model: "m", Messages: messages}, func(string) {}); err != nil {
		t.Fatal(err)
	}
	return got
}

func TestOllamaSendsImagesOnTheMessage(t *testing.T) {
	got := captureChat(t, func(url string) Provider { return &Ollama{BaseURL: url} }, []Message{{Role: "user", Content: "Quoi ?", Images: []string{"abcd"}}})
	message := got["messages"].([]any)[0].(map[string]any)
	if images := message["images"].([]any); len(images) != 1 || images[0] != "abcd" {
		t.Errorf("message = %v", message)
	}
}

func TestOpenAISendsImagesAsContentParts(t *testing.T) {
	png := base64.StdEncoding.EncodeToString([]byte("\x89PNG\r\n\x1a\nrest"))
	got := captureChat(t, func(url string) Provider { return &OpenAI{BaseURL: url} }, []Message{
		{Role: "user", Content: "Avant"},
		{Role: "user", Content: "Quoi ?", Images: []string{png}},
	})
	messages := got["messages"].([]any)
	if plain := messages[0].(map[string]any); plain["content"] != "Avant" {
		t.Errorf("plain message = %v", plain)
	}
	parts := messages[1].(map[string]any)["content"].([]any)
	if len(parts) != 2 || parts[0].(map[string]any)["text"] != "Quoi ?" {
		t.Fatalf("parts = %v", parts)
	}
	url := parts[1].(map[string]any)["image_url"].(map[string]any)["url"]
	if url != "data:image/png;base64,"+png {
		t.Errorf("url = %v", url)
	}
}

func TestOllamaModelsReportVision(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/tags" {
			fmt.Fprint(w, `{"models":[{"name":"voit"},{"name":"aveugle"},{"name":"ancien"}]}`)
			return
		}
		var body struct {
			Model string `json:"model"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		switch body.Model {
		case "voit":
			fmt.Fprint(w, `{"capabilities":["completion","vision"]}`)
		case "aveugle":
			fmt.Fprint(w, `{"capabilities":["completion"]}`)
		default:
			fmt.Fprint(w, `{}`)
		}
	}))
	defer server.Close()
	models, err := (&Ollama{BaseURL: server.URL}).Models(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if models[0].Vision == nil || !*models[0].Vision {
		t.Errorf("voit = %v", models[0].Vision)
	}
	if models[1].Vision == nil || *models[1].Vision {
		t.Errorf("aveugle = %v", models[1].Vision)
	}
	if models[2].Vision != nil {
		t.Errorf("ancien = %v, want unknown", *models[2].Vision)
	}
}

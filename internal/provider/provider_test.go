package provider

import (
	"context"
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

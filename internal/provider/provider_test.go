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
	if result.DefaultTemperature {
		t.Fatal("thinking retry keeps the temperature, marked as default")
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
	if gotAuth != "Bearer secret" || gotBody["response_format"] == nil || gotBody["temperature"] != Temperature {
		t.Fatalf("auth=%q body=%v", gotAuth, gotBody)
	}
	if result.Output != "Oui" || *result.PromptTokens != 3 || *result.OutputTokens != 7 {
		t.Fatalf("result: %+v", result)
	}
	if result.CLIStartupMs != nil || result.CLIExitMs != nil || result.DefaultTemperature {
		t.Fatalf("bridge-only fields outside the bridge: %+v", result)
	}
}

func TestOpenAIChatReadsBridgeFieldsFromUsage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"Oui\"}}]}\n\n")
		fmt.Fprint(w, "data: {\"choices\":[],\"usage\":{\"prompt_tokens\":3,\"completion_tokens\":7,\"cli_startup_ms\":430,\"cli_exit_ms\":650}}\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	result, err := (&OpenAI{BaseURL: srv.URL}).Chat(context.Background(), newChat(), func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	if result.CLIStartupMs == nil || *result.CLIStartupMs != 430 || result.CLIExitMs == nil || *result.CLIExitMs != 650 {
		t.Fatalf("result: %+v", result)
	}
}

func TestOpenAIRateLeavesOutTheCLIExit(t *testing.T) {
	ttft, exit, tokens := int64(1000), int64(1000), 100
	result := ChatResult{TTFTMs: &ttft, TotalMs: 3000, OutputTokens: &tokens, CLIExitMs: &exit}
	setOpenAIRate(&result)
	if result.TokPerSec == nil || *result.TokPerSec != 100 {
		t.Fatalf("rate: %v", result.TokPerSec)
	}
}

func TestOpenAIChatRetriesRejectedSamplingParams(t *testing.T) {
	for _, rejected := range []string{"temperature", "max_tokens"} {
		var bodies []map[string]any
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			bodies = append(bodies, body)
			if _, ok := body[rejected]; ok {
				http.Error(w, `{"error":{"message":"Unsupported parameter: '`+rejected+`' is not supported with this model."}}`, http.StatusBadRequest)
				return
			}
			fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"Oui\"}}]}\n\ndata: [DONE]\n\n")
		}))

		result, err := (&OpenAI{BaseURL: srv.URL}).Chat(context.Background(), newChat(), func(string) {})
		srv.Close()
		if err != nil || result.Output != "Oui" || len(bodies) != 2 || !result.DefaultTemperature {
			t.Fatalf("%s: calls=%d output=%q default=%v err=%v", rejected, len(bodies), result.Output, result.DefaultTemperature, err)
		}
		if bodies[0]["max_tokens"] != 10.0 {
			t.Errorf("%s: first attempt budget %v", rejected, bodies[0]["max_tokens"])
		}
		retry := bodies[1]
		if retry["temperature"] != nil || retry["max_tokens"] != nil || retry["max_completion_tokens"] != float64(reasoningMaxTokens) {
			t.Errorf("%s: retry body %v", rejected, retry)
		}
	}
}

func TestOpenAIChatDoesNotRetryOtherErrors(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		http.Error(w, `{"error":{"message":"invalid api key"}}`, http.StatusUnauthorized)
	}))
	defer srv.Close()

	if _, err := (&OpenAI{BaseURL: srv.URL}).Chat(context.Background(), newChat(), func(string) {}); err == nil || calls != 1 {
		t.Fatalf("calls=%d err=%v", calls, err)
	}
}

func TestOpenAIChatSendsStrictSchemaLeavingTheCaseOne(t *testing.T) {
	var got struct {
		ResponseFormat struct {
			JSONSchema struct {
				Strict bool `json:"strict"`
				Schema struct {
					AdditionalProperties *bool `json:"additionalProperties"`
					Properties           struct {
						Lines struct {
							Items struct {
								AdditionalProperties *bool `json:"additionalProperties"`
							} `json:"items"`
						} `json:"lines"`
					} `json:"properties"`
				} `json:"schema"`
			} `json:"json_schema"`
		} `json:"response_format"`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	schema := `{"type":"object","properties":{"lines":{"type":"array","items":{"type":"object","properties":{"n":{"type":"number"}}}}}}`
	req := newChat()
	req.Schema = json.RawMessage(schema)
	if _, err := (&OpenAI{BaseURL: srv.URL}).Chat(context.Background(), req, func(string) {}); err != nil {
		t.Fatal(err)
	}
	sent := got.ResponseFormat.JSONSchema
	if !sent.Strict || sent.Schema.AdditionalProperties == nil || *sent.Schema.AdditionalProperties ||
		sent.Schema.Properties.Lines.Items.AdditionalProperties == nil || *sent.Schema.Properties.Lines.Items.AdditionalProperties {
		t.Fatalf("schema not strict: %+v", sent)
	}
	if string(req.Schema) != schema {
		t.Fatalf("case schema changed: %s", req.Schema)
	}
}

func TestOpenAIChatRejectsInvalidSchema(t *testing.T) {
	req := newChat()
	req.Schema = json.RawMessage(`[`)
	if _, err := (&OpenAI{BaseURL: "http://unused"}).Chat(context.Background(), req, func(string) {}); err == nil {
		t.Fatal("invalid schema sent")
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

var weatherTool = Tool{Name: "weather", Description: "Météo d'une ville", Parameters: json.RawMessage(`{"type":"object","properties":{"city":{"type":"string"}}}`)}

// toolHistory is one tool round: the model's call, then the tool's answer.
var toolHistory = []Message{
	{Role: "user", Content: "Quel temps ?"},
	{Role: "assistant", ToolCalls: []ToolCall{{ID: "c1", Name: "weather", Arguments: `{"city":"Pau"}`}}},
	{Role: "tool", Content: "12 °C", ToolCallID: "c1", ToolName: "weather"},
}

// sentTool returns the first tool definition of a request body.
func sentTool(t *testing.T, body map[string]any) map[string]any {
	t.Helper()
	tools, _ := body["tools"].([]any)
	if len(tools) != 1 {
		t.Fatalf("tools = %v", body["tools"])
	}
	tool := tools[0].(map[string]any)
	if tool["type"] != "function" {
		t.Errorf("tool = %v", tool)
	}
	return tool["function"].(map[string]any)
}

func TestOllamaChatSendsToolsAndReadsToolCalls(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		fmt.Fprintln(w, `{"message":{"content":"","tool_calls":[{"id":"c1","function":{"name":"weather","arguments":{"city":"Pau"}}}]},"done":false}`)
		fmt.Fprintln(w, `{"message":{"content":""},"done":true,"eval_count":9,"eval_duration":1000000000}`)
	}))
	defer srv.Close()

	req := newChat()
	req.Tools = []Tool{weatherTool}
	result, err := (&Ollama{BaseURL: srv.URL}).Chat(context.Background(), req, func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	if function := sentTool(t, got); function["name"] != "weather" || function["parameters"] == nil {
		t.Errorf("function = %v", function)
	}
	want := ToolCall{ID: "c1", Name: "weather", Arguments: `{"city":"Pau"}`}
	if len(result.ToolCalls) != 1 || result.ToolCalls[0] != want {
		t.Fatalf("tool calls = %+v", result.ToolCalls)
	}
	if result.TTFTMs != nil || *result.OutputTokens != 9 {
		t.Errorf("metrics: %+v", result)
	}
}

func TestOllamaSendsToolHistory(t *testing.T) {
	got := captureChat(t, func(url string) Provider { return &Ollama{BaseURL: url} }, toolHistory)
	messages := got["messages"].([]any)
	call := messages[1].(map[string]any)["tool_calls"].([]any)[0].(map[string]any)["function"].(map[string]any)
	if call["name"] != "weather" || call["arguments"].(map[string]any)["city"] != "Pau" {
		t.Errorf("call = %v", call)
	}
	if answer := messages[2].(map[string]any); answer["role"] != "tool" || answer["tool_name"] != "weather" || answer["content"] != "12 °C" {
		t.Errorf("answer = %v", answer)
	}
	if _, ok := got["tools"]; ok {
		t.Error("empty tools sent")
	}
}

func TestOpenAIChatJoinsStreamedToolCalls(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		for _, delta := range []string{
			`{"index":0,"id":"c1","type":"function","function":{"name":"weather","arguments":""}}`,
			`{"index":0,"function":{"arguments":"{\"city\":"}}`,
			`{"index":1,"id":"c2","type":"function","function":{"name":"weather","arguments":"{\"city\":\"Dax\"}"}}`,
			`{"index":0,"function":{"arguments":"\"Pau\"}"}}`,
		} {
			fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"tool_calls\":[%s]}}]}\n\n", delta)
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	req := newChat()
	req.Tools = []Tool{weatherTool}
	result, err := (&OpenAI{BaseURL: srv.URL}).Chat(context.Background(), req, func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	if function := sentTool(t, got); function["name"] != "weather" || function["description"] != "Météo d'une ville" {
		t.Errorf("function = %v", function)
	}
	want := []ToolCall{{ID: "c1", Name: "weather", Arguments: `{"city":"Pau"}`}, {ID: "c2", Name: "weather", Arguments: `{"city":"Dax"}`}}
	if len(result.ToolCalls) != 2 || result.ToolCalls[0] != want[0] || result.ToolCalls[1] != want[1] {
		t.Fatalf("tool calls = %+v", result.ToolCalls)
	}
}

func TestOpenAIChatSplitsWholeToolCallsSentWithoutIndex(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for _, call := range []string{
			`{"id":"c1","type":"function","function":{"name":"weather","arguments":"{\"city\":\"Pau\"}"}}`,
			`{"id":"c2","type":"function","function":{"name":"weather","arguments":"{\"city\":\"Dax\"}"}}`,
		} {
			fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"tool_calls\":[%s]}}]}\n\n", call)
		}
	}))
	defer srv.Close()

	result, err := (&OpenAI{BaseURL: srv.URL}).Chat(context.Background(), newChat(), func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	want := []ToolCall{{ID: "c1", Name: "weather", Arguments: `{"city":"Pau"}`}, {ID: "c2", Name: "weather", Arguments: `{"city":"Dax"}`}}
	if len(result.ToolCalls) != 2 || result.ToolCalls[0] != want[0] || result.ToolCalls[1] != want[1] {
		t.Fatalf("tool calls = %+v", result.ToolCalls)
	}
}

func TestOpenAIChatRejectsToolCallIndexOutOfOrder(t *testing.T) {
	for index, want := range map[int]string{5: "streamed before call 0", -1: "invalid tool call index -1"} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":%d,\"id\":\"c1\",\"function\":{\"name\":\"weather\"}}]}}]}\n\n", index)
		}))

		if _, err := (&OpenAI{BaseURL: srv.URL}).Chat(context.Background(), newChat(), func(string) {}); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("index %d: err = %v", index, err)
		}
		srv.Close()
	}
}

func TestOpenAISendsToolHistory(t *testing.T) {
	got := captureChat(t, func(url string) Provider { return &OpenAI{BaseURL: url} }, toolHistory)
	messages := got["messages"].([]any)
	call := messages[1].(map[string]any)["tool_calls"].([]any)[0].(map[string]any)
	if call["id"] != "c1" || call["type"] != "function" || call["function"].(map[string]any)["arguments"] != `{"city":"Pau"}` {
		t.Errorf("call = %v", call)
	}
	answer := messages[2].(map[string]any)
	if answer["role"] != "tool" || answer["tool_call_id"] != "c1" || answer["content"] != "12 °C" {
		t.Errorf("answer = %v", answer)
	}
	if _, ok := answer["tool_name"]; ok {
		t.Errorf("Ollama-only field sent: %v", answer)
	}
	if _, ok := got["tools"]; ok {
		t.Error("empty tools sent")
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

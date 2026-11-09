package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/arkhayds/axolot/internal/provider"
)

const (
	testToken = "jeton"
	testModel = "claude-haiku-5-5"
	okResult  = `{"type":"result","is_error":false,"result":"Bonjour","usage":{"input_tokens":2,"cache_creation_input_tokens":619,"cache_read_input_tokens":10,"output_tokens":4}}`
)

// textDelta is the stream-json line of one piece of answer.
func textDelta(text string) string {
	return fmt.Sprintf(`{"type":"stream_event","event":{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":%q}}}`, text)
}

// startBridge serves a bridge whose CLI is a shell script recording its arguments, stdin and environment next to itself.
// @param body - script run after the recording
// @returns server, recording directory
func startBridge(t *testing.T, body string) (*httptest.Server, string) {
	t.Helper()
	dir := t.TempDir()
	script := fmt.Sprintf(`#!/bin/sh
for arg in "$@"; do printf '%%s\n' "$arg"; done > %[1]s/args
cat > %[1]s/stdin
pwd > %[1]s/cwd
printf '%%s' "$CLAUDE_CODE_DISABLE_AUTO_MEMORY" > %[1]s/memory
%[2]s
`, dir, body)
	cli := filepath.Join(dir, "claude")
	if err := os.WriteFile(cli, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer((&bridge{token: testToken, cli: cli, dir: t.TempDir()}).handler())
	t.Cleanup(server.Close)
	return server, dir
}

// printLines is a script body writing stream-json lines.
func printLines(lines ...string) string {
	return "cat <<'EOF'\n" + strings.Join(lines, "\n") + "\nEOF"
}

// client is Axolot's own connection pointed at the bridge.
func client(server *httptest.Server) *provider.OpenAI {
	return &provider.OpenAI{BaseURL: server.URL + "/v1", APIKey: testToken}
}

// ask sends one question and returns the streamed pieces with the result.
func ask(t *testing.T, server *httptest.Server, messages ...provider.Message) ([]string, provider.ChatResult, error) {
	t.Helper()
	var deltas []string
	result, err := client(server).Chat(context.Background(), provider.ChatRequest{Model: testModel, Messages: messages}, func(d string) { deltas = append(deltas, d) })
	return deltas, result, err
}

func user(content string) provider.Message {
	return provider.Message{Role: "user", Content: content}
}

func readRecorded(t *testing.T, dir, name string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func cliStarted(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, "args"))
	return err == nil
}

func TestChatStreamsTextAndReportsUsage(t *testing.T) {
	thinking := `{"type":"stream_event","event":{"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"hmm"}}}`
	noise := `{"type":"system","subtype":"init","tools":[]}`
	server, _ := startBridge(t, printLines(noise, thinking, textDelta("Bon"), textDelta("jour"), okResult))

	deltas, result, err := ask(t, server, user("Salut"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(deltas, "|") != "Bon|jour" || result.Output != "Bonjour" {
		t.Errorf("deltas = %v, output = %q", deltas, result.Output)
	}
	if result.PromptTokens == nil || *result.PromptTokens != 631 || result.OutputTokens == nil || *result.OutputTokens != 4 {
		t.Errorf("usage = %+v", result)
	}
}

func TestChatCutsCLIFromLocalContext(t *testing.T) {
	server, dir := startBridge(t, printLines(okResult))

	if _, _, err := ask(t, server, provider.Message{Role: "system", Content: "Consigne"}, user("Salut")); err != nil {
		t.Fatal(err)
	}
	args := readRecorded(t, dir, "args")
	for _, want := range []string{"--model\n" + testModel + "\n", "--system-prompt\nConsigne\n", "--tools\n\n", "--setting-sources\n\n", "--strict-mcp-config\n", "--no-session-persistence\n"} {
		if !strings.Contains(args, want) {
			t.Errorf("args miss %q:\n%s", want, args)
		}
	}
	if got := readRecorded(t, dir, "stdin"); got != "Salut" {
		t.Errorf("stdin = %q", got)
	}
	if got := readRecorded(t, dir, "memory"); got != "1" {
		t.Errorf("auto memory switch = %q", got)
	}
}

func TestChatAlwaysReplacesTheSystemPrompt(t *testing.T) {
	server, dir := startBridge(t, printLines(okResult))

	if _, _, err := ask(t, server, user("Salut")); err != nil {
		t.Fatal(err)
	}
	if args := readRecorded(t, dir, "args"); !strings.Contains(args, "--system-prompt\n\n") {
		t.Errorf("args = %s", args)
	}
}

func TestChatSerializesHistoryIntoOnePrompt(t *testing.T) {
	server, dir := startBridge(t, printLines(okResult))

	_, _, err := ask(t, server, user("Un"), provider.Message{Role: "assistant", Content: "Deux"}, user("Trois"))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := readRecorded(t, dir, "stdin"), "User: Un\n\nAssistant: Deux\n\nUser: Trois"; got != want {
		t.Errorf("stdin = %q, want %q", got, want)
	}
}

const (
	toolStart        = `{"type":"stream_event","event":{"type":"content_block_start","index":1,"content_block":{"type":"tool_use","name":"StructuredOutput","input":{}}}}`
	structuredResult = `{"type":"result","is_error":false,"result":"{\"ville\":\"Paris\"}","structured_output":{"ville":"Paris"},"usage":{"input_tokens":2,"output_tokens":9}}`
)

// jsonDelta is the stream-json line of one piece of StructuredOutput input.
func jsonDelta(partial string) string {
	return fmt.Sprintf(`{"type":"stream_event","event":{"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":%q}}}`, partial)
}

// askWithSchema sends one question that requires a JSON answer.
func askWithSchema(t *testing.T, server *httptest.Server, schema string) ([]string, provider.ChatResult, error) {
	t.Helper()
	var deltas []string
	req := provider.ChatRequest{Model: testModel, Messages: []provider.Message{user("Capitale ?")}, Schema: json.RawMessage(schema)}
	result, err := client(server).Chat(context.Background(), req, func(d string) { deltas = append(deltas, d) })
	return deltas, result, err
}

func TestChatStreamsStructuredOutputForASchema(t *testing.T) {
	server, dir := startBridge(t, printLines(textDelta("Voici :"), toolStart, jsonDelta(""), jsonDelta(`{"ville": "Pa`), jsonDelta(`ris"}`), structuredResult))

	schema := `{"type":"object","properties":{"ville":{"type":"string"}}}`
	deltas, result, err := askWithSchema(t, server, schema)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(deltas, "|") != `{"ville": "Pa|ris"}` {
		t.Errorf("deltas = %v", deltas)
	}
	if result.OutputTokens == nil || *result.OutputTokens != 9 {
		t.Errorf("usage = %+v", result)
	}
	if args := readRecorded(t, dir, "args"); !strings.Contains(args, "--json-schema\n"+schema+"\n") {
		t.Errorf("args = %s", args)
	}
}

func TestChatFailsWhenASchemaGetsNoStructuredOutput(t *testing.T) {
	server, _ := startBridge(t, printLines(textDelta("Bonjour"), okResult))

	if _, _, err := askWithSchema(t, server, `{"type":"object"}`); err == nil || !strings.Contains(err.Error(), "502") {
		t.Fatalf("err = %v", err)
	}
}

func TestChatCutsConnectionWhenStructuredOutputIsRetried(t *testing.T) {
	server, _ := startBridge(t, printLines(toolStart, jsonDelta(`{"ville": 1}`), toolStart, jsonDelta(`{"ville": "Paris"}`), structuredResult))

	if _, _, err := askWithSchema(t, server, `{"type":"object"}`); err == nil {
		t.Fatal("two concatenated attempts must not read as an answer")
	}
}

func TestChatReturnsCLIErrorBeforeAnyText(t *testing.T) {
	server, _ := startBridge(t, printLines(`{"type":"result","is_error":true,"api_error_status":429,"result":"Limite atteinte"}`))

	_, _, err := ask(t, server, user("Salut"))
	if err == nil || !strings.Contains(err.Error(), "502") || !strings.Contains(err.Error(), "Limite atteinte") {
		t.Fatalf("err = %v", err)
	}
}

func TestChatFailsWhenCLIEndsWithoutResult(t *testing.T) {
	server, _ := startBridge(t, "echo 'pas connecté' >&2; exit 1")

	if _, _, err := ask(t, server, user("Salut")); err == nil || !strings.Contains(err.Error(), "502") {
		t.Fatalf("err = %v", err)
	}
}

func TestChatFailsOnNonZeroExitDespiteResult(t *testing.T) {
	server, _ := startBridge(t, printLines(okResult)+"\nexit 3")

	if _, _, err := ask(t, server, user("Salut")); err == nil || !strings.Contains(err.Error(), "502") {
		t.Fatalf("err = %v", err)
	}
}

func TestChatFailsOnUnreadableCLIOutput(t *testing.T) {
	server, _ := startBridge(t, printLines("pas du json", okResult))

	if _, _, err := ask(t, server, user("Salut")); err == nil || !strings.Contains(err.Error(), "502") {
		t.Fatalf("err = %v", err)
	}
}

func TestChatCutsConnectionOnFailureAfterText(t *testing.T) {
	server, _ := startBridge(t, printLines(textDelta("Bon"), `{"type":"result","is_error":true,"result":"Coupure"}`))

	deltas, result, err := ask(t, server, user("Salut"))
	if err == nil {
		t.Fatal("a cut stream must not read as a complete answer")
	}
	if len(deltas) != 1 || result.Output != "Bon" {
		t.Errorf("deltas = %v, output = %q", deltas, result.Output)
	}
}

func TestChatKillsCLIWhenClientLeaves(t *testing.T) {
	server, dir := startBridge(t, printLines(textDelta("Bon"))+"\nexec sleep 60")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, err := client(server).Chat(ctx, provider.ChatRequest{Model: testModel, Messages: []provider.Message{user("Salut")}}, func(string) { cancel() })
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v", err)
	}
	if !cliStarted(dir) {
		t.Fatal("CLI never started")
	}
	// Close waits for handlers, which only return once the CLI is gone.
	start := time.Now()
	server.Close()
	if elapsed := time.Since(start); elapsed > 20*time.Second {
		t.Errorf("handler held for %s", elapsed)
	}
}

func TestChatRefusesInvalidRequestsWithoutStartingCLI(t *testing.T) {
	image := provider.Message{Role: "user", Content: "Quoi ?", Images: []string{"aGVsbG8="}}
	cases := map[string]provider.ChatRequest{
		"unknown model":    {Model: "opus", Messages: []provider.Message{user("Salut")}},
		"option as model":  {Model: "--dangerously-skip-permissions", Messages: []provider.Message{user("Salut")}},
		"image":            {Model: testModel, Messages: []provider.Message{image}},
		"no question":      {Model: testModel, Messages: []provider.Message{{Role: "system", Content: "Consigne"}}},
		"assistant last":   {Model: testModel, Messages: []provider.Message{user("Salut"), {Role: "assistant", Content: "Oui"}}},
		"tool role":        {Model: testModel, Messages: []provider.Message{{Role: "tool", Content: "x"}, user("Salut")}},
		"no message":       {Model: testModel},
		"system mid-story": {Model: testModel, Messages: []provider.Message{user("Un"), {Role: "system", Content: "x"}, user("Deux")}},
	}
	for name, req := range cases {
		t.Run(name, func(t *testing.T) {
			server, dir := startBridge(t, printLines(okResult))
			_, err := client(server).Chat(context.Background(), req, func(string) {})
			if err == nil || !strings.Contains(err.Error(), "returned 4") {
				t.Errorf("err = %v", err)
			}
			if cliStarted(dir) {
				t.Error("CLI started on an invalid request")
			}
		})
	}
}

func TestModelsListsFullIdentifiers(t *testing.T) {
	server, _ := startBridge(t, "")

	models, err := client(server).Models(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	got := make([]string, 0, len(models))
	for _, m := range models {
		got = append(got, m.Name)
	}
	if want := "claude-fable-5-1 claude-opus-5-5 claude-sonnet-5-5 claude-haiku-5-5"; strings.Join(got, " ") != want {
		t.Errorf("models = %v", got)
	}
}

func TestEveryRouteRequiresTheToken(t *testing.T) {
	server, dir := startBridge(t, printLines(okResult))
	body := `{"model":"` + testModel + `","messages":[{"role":"user","content":"x"}]}`
	routes := []struct{ method, path, body string }{
		{http.MethodGet, "/v1/models", ""},
		{http.MethodPost, "/v1/chat/completions", body},
	}
	for _, route := range routes {
		for _, header := range []string{"", "Bearer faux", "Basic " + testToken, testToken} {
			req, err := http.NewRequest(route.method, server.URL+route.path, strings.NewReader(route.body))
			if err != nil {
				t.Fatal(err)
			}
			if header != "" {
				req.Header.Set("Authorization", header)
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			resp.Body.Close()
			if resp.StatusCode != http.StatusUnauthorized {
				t.Errorf("%s with %q = %d", route.path, header, resp.StatusCode)
			}
		}
	}
	if cliStarted(dir) {
		t.Error("CLI started without the token")
	}
}

func TestErrorsUseTheOpenAIShape(t *testing.T) {
	server, _ := startBridge(t, "")
	req, err := http.NewRequest(http.MethodPost, server.URL+"/v1/chat/completions", strings.NewReader("pas du json"))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+testToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var got struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusBadRequest || got.Error.Message == "" {
		t.Errorf("status = %d, error = %+v", resp.StatusCode, got)
	}
}

package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"slices"
	"strings"
	"time"
)

const (
	// maxBody and maxLine match what Axolot may send and what a large answer may weigh.
	maxBody = 16 << 20
	maxLine = 16 << 20
	// cliTimeout outlasts Axolot's own 5 minute limit, whose message is the clearer one.
	cliTimeout = 6 * time.Minute
	// cliWaitDelay keeps a killed CLI's surviving children from holding the handler through its pipes.
	cliWaitDelay = 2 * time.Second
	maxLogged    = 2048
)

// models are the identifiers served: the name is the cumulative base's key, so aliases like "opus" are refused.
var models = []string{
	"claude-fable-5-1", "claude-fable-5",
	"claude-opus-5-5", "claude-opus-5", "claude-opus-4-8", "claude-opus-4-7", "claude-opus-4-6", "claude-opus-4-5",
	"claude-sonnet-5-5", "claude-sonnet-5", "claude-sonnet-4-6",
	"claude-haiku-5-5", "claude-haiku-4-5",
}

// bridge serves the Claude CLI over HTTP.
type bridge struct {
	token string
	cli   string
	// dir is the empty working directory, so that no project file reaches the model.
	dir string
}

// handler returns the routes behind the token check.
func (b *bridge) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/models", b.listModels)
	mux.HandleFunc("POST /v1/chat/completions", b.chat)
	return b.requireToken(mux)
}

// requireToken refuses any request without the bearer token.
func (b *bridge) requireToken(next http.Handler) http.Handler {
	expected := []byte("Bearer " + b.token)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), expected) != 1 {
			slog.Warn("bridge request refused", "path", r.URL.Path, "remote", r.RemoteAddr)
			writeError(w, http.StatusUnauthorized, "invalid token")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// listModels answers /v1/models.
func (b *bridge) listModels(w http.ResponseWriter, _ *http.Request) {
	data := make([]map[string]string, 0, len(models))
	for _, name := range models {
		data = append(data, map[string]string{"id": name, "object": "model"})
	}
	writeJSON(w, http.StatusOK, map[string]any{"object": "list", "data": data})
}

// chatRequest is the part of /v1/chat/completions the CLI can honour: temperature and max_tokens have no CLI option.
type chatRequest struct {
	Model          string        `json:"model"`
	Messages       []chatMessage `json:"messages"`
	ResponseFormat struct {
		JSONSchema struct {
			Schema json.RawMessage `json:"schema"`
		} `json:"json_schema"`
	} `json:"response_format"`
}

// chatMessage only takes text content: a message with images comes as parts and fails to decode.
type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// chat answers /v1/chat/completions, always as a stream, the only form Axolot calls.
// A failure before the first piece of text returns a status; after it, the connection is cut,
// since a stream the client reads to its end would pass for a complete answer.
func (b *bridge) chat(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming not supported")
		return
	}
	var body chatRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBody)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request, images are not supported: "+err.Error())
		return
	}
	if !slices.Contains(models, body.Model) {
		writeError(w, http.StatusNotFound, "unknown model "+body.Model)
		return
	}
	system, prompt, err := splitMessages(body.Messages)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	out := &eventStream{w: w, flusher: flusher}
	ctx, cancel := context.WithTimeout(r.Context(), cliTimeout)
	defer cancel()
	schema := body.ResponseFormat.JSONSchema.Schema
	result, err := b.run(ctx, body.Model, system, schema, prompt, out.text)
	if structured := string(result.StructuredOutput); err == nil && len(schema) > 0 && (structured == "" || structured == "null") {
		err = errors.New("claude returned no structured output")
	}
	switch {
	case r.Context().Err() != nil:
	case err != nil && out.started:
		panic(http.ErrAbortHandler)
	case err != nil:
		writeError(w, http.StatusBadGateway, err.Error())
	default:
		out.finish(result.Usage)
	}
}

// splitMessages turns the conversation into the CLI's two inputs, since `claude -p` takes a single prompt.
// @returns system prompt, prompt with past turns serialized when there are any
func splitMessages(messages []chatMessage) (string, string, error) {
	var system []string
	for len(messages) > 0 && messages[0].Role == "system" {
		system = append(system, messages[0].Content)
		messages = messages[1:]
	}
	if len(messages) == 0 || messages[len(messages)-1].Role != "user" {
		return "", "", errors.New("the last message must be a question")
	}
	joined := strings.Join(system, "\n\n")
	if len(messages) == 1 {
		return joined, messages[0].Content, nil
	}
	turns := make([]string, 0, len(messages))
	for _, message := range messages {
		switch message.Role {
		case "user":
			turns = append(turns, "User: "+message.Content)
		case "assistant":
			turns = append(turns, "Assistant: "+message.Content)
		default:
			return "", "", fmt.Errorf("unsupported role %q", message.Role)
		}
	}
	return joined, strings.Join(turns, "\n\n"), nil
}

// cliArgs cuts everything local from the CLI: tools, MCP, settings, skills, sessions.
// --system-prompt is always passed, or Claude Code's own prompt would apply.
// --json-schema adds the StructuredOutput tool, the only one --tools "" leaves.
func cliArgs(model, system string, schema json.RawMessage) []string {
	args := []string{
		"-p", "--model", model, "--system-prompt", system,
		"--output-format", "stream-json", "--verbose", "--include-partial-messages",
		"--no-session-persistence", "--setting-sources", "", "--strict-mcp-config",
		"--disable-slash-commands", "--tools", "",
	}
	if len(schema) > 0 {
		args = append(args, "--json-schema", string(schema))
	}
	return args
}

// cliUsage is the token count of the result line; the input splits over the prompt cache.
type cliUsage struct {
	Input         int `json:"input_tokens"`
	CacheCreation int `json:"cache_creation_input_tokens"`
	CacheRead     int `json:"cache_read_input_tokens"`
	Output        int `json:"output_tokens"`
}

// cliEvent holds the fields read from a stream-json line; other line types only leave them empty.
type cliEvent struct {
	Type  string `json:"type"`
	Event struct {
		ContentBlock struct {
			Type string `json:"type"`
		} `json:"content_block"`
		Delta struct {
			Type        string `json:"type"`
			Text        string `json:"text"`
			PartialJSON string `json:"partial_json"`
		} `json:"delta"`
	} `json:"event"`
	IsError          bool            `json:"is_error"`
	Result           string          `json:"result"`
	StructuredOutput json.RawMessage `json:"structured_output"`
	Usage            cliUsage        `json:"usage"`
}

// run executes one generation and passes each piece of text to onText.
// The prompt goes through stdin: documents can exceed what an argument holds.
// @param schema - JSON schema the answer must follow, empty for free text
// @returns result line; an error when the CLI reports one, dies or prints something unreadable
func (b *bridge) run(ctx context.Context, model, system string, schema json.RawMessage, prompt string, onText func(string)) (cliEvent, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	cmd := exec.CommandContext(ctx, b.cli, cliArgs(model, system, schema)...)
	cmd.Dir = b.dir
	// An API credential would take precedence over the subscription login and bill the API.
	env := slices.DeleteFunc(os.Environ(), func(kv string) bool {
		return strings.HasPrefix(kv, "ANTHROPIC_API_KEY=") || strings.HasPrefix(kv, "ANTHROPIC_AUTH_TOKEN=")
	})
	cmd.Env = append(env, "CLAUDE_CODE_DISABLE_AUTO_MEMORY=1")
	cmd.Stdin = strings.NewReader(prompt)
	cmd.WaitDelay = cliWaitDelay
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return cliEvent{}, fmt.Errorf("open claude output: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return cliEvent{}, fmt.Errorf("start claude: %w", err)
	}

	result, err := readResult(stdout, len(schema) > 0, onText)
	if err != nil {
		cancel()
	}
	waitErr := cmd.Wait()
	if ctx.Err() != nil && err == nil {
		err = ctx.Err()
	}
	if err == nil {
		err = resultError(result, waitErr)
	}
	if err != nil {
		if !errors.Is(err, context.Canceled) {
			slog.Error("claude failed", "model", model, "err", err, "stderr", string(stderr.Bytes()[:min(stderr.Len(), maxLogged)]))
		}
		return cliEvent{}, err
	}
	return *result, nil
}

// readResult follows the stream up to its end and returns the result line, nil when none came.
// With a schema the answer is the StructuredOutput tool's input, streamed like text so that TTFT and
// tok/s stay measured; free text around it is dropped, and a second attempt fails since both would concatenate.
// @param structured - a schema was requested
func readResult(stdout io.Reader, structured bool, onText func(string)) (*cliEvent, error) {
	var result *cliEvent
	attempts := 0
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 64*1024), maxLine)
	for scanner.Scan() {
		var event cliEvent
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			return nil, fmt.Errorf("read claude output: %w", err)
		}
		switch {
		case event.Type == "result":
			result = &event
		case !structured && event.Event.Delta.Type == "text_delta":
			onText(event.Event.Delta.Text)
		case structured && event.Event.Delta.Type == "input_json_delta":
			onText(event.Event.Delta.PartialJSON)
		case structured && event.Event.ContentBlock.Type == "tool_use":
			if attempts++; attempts > 1 {
				return nil, errors.New("claude retried its structured output")
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read claude output: %w", err)
	}
	return result, nil
}

// resultError turns how the CLI ended into an error; its own message comes first as the most useful (quota, login).
func resultError(result *cliEvent, waitErr error) error {
	switch {
	case result == nil:
		return fmt.Errorf("claude ended without a result: %v", waitErr)
	case result.IsError:
		return errors.New(result.Result)
	case waitErr != nil:
		return fmt.Errorf("claude failed after its result: %w", waitErr)
	}
	return nil
}

// eventStream writes the SSE answer; headers leave with the first chunk so that an early failure can still set a status.
type eventStream struct {
	w       http.ResponseWriter
	flusher http.Flusher
	started bool
}

// send writes one chunk.
func (s *eventStream) send(chunk any) {
	if !s.started {
		s.started = true
		s.w.Header().Set("Content-Type", "text/event-stream")
		s.w.Header().Set("Cache-Control", "no-cache")
	}
	payload, err := json.Marshal(chunk)
	if err != nil {
		panic(err)
	}
	// A failed write means the client left; the context stops the CLI.
	_, _ = fmt.Fprintf(s.w, "data: %s\n\n", payload)
	s.flusher.Flush()
}

// text sends one piece of answer; an empty one would start the stream for nothing, losing the error status.
func (s *eventStream) text(content string) {
	if content == "" {
		return
	}
	s.send(map[string]any{"choices": []any{map[string]any{"index": 0, "delta": map[string]string{"content": content}}}})
}

// finish sends the usage and closes the stream. The cache parts count as input: they are read by the model
// and weigh on the quota, like a local model's whole prompt.
func (s *eventStream) finish(usage cliUsage) {
	s.send(map[string]any{"choices": []any{}, "usage": map[string]int{
		"prompt_tokens":     usage.Input + usage.CacheCreation + usage.CacheRead,
		"completion_tokens": usage.Output,
	}})
	_, _ = fmt.Fprint(s.w, "data: [DONE]\n\n")
	s.flusher.Flush()
}

// writeError answers in the OpenAI error shape, which Axolot shows as is.
func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]any{"error": map[string]string{"message": message}})
}

// writeJSON writes one JSON response.
func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		slog.Warn("write response", "err", err)
	}
}

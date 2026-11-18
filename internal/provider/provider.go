// Package provider talks to model servers (Ollama, any OpenAI-compatible API) behind one interface.
package provider

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// maxErrorBody caps how much of a failed response ends up in an error message.
const maxErrorBody = 2048

// Temperature is low but not zero, with no fixed seed: repeating a question must yield a new measure,
// otherwise the cumulative base would count the same answer many times and show false precision.
const Temperature = 0.3

// Message is one chat turn, in Ollama's wire format; each provider converts what its API words differently.
// Images are base64 pictures. A "tool" message answers the call named by ToolCallID (OpenAI) and ToolName (Ollama).
type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content"`
	Images     []string   `json:"images,omitempty"`
	ToolCalls  []ToolCall `json:"-"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	ToolName   string     `json:"tool_name,omitempty"`
}

// Tool is a function offered to the model; Parameters is its JSON Schema.
type Tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

// ToolCall is one tool the model asks to run.
// Arguments is the JSON text the model wrote, kept as is: it may be invalid, the caller reports that to the model.
type ToolCall struct {
	ID        string
	Name      string
	Arguments string
}

// ChatRequest describes one generation; Schema forces structured output when set.
type ChatRequest struct {
	Model     string
	Messages  []Message
	Schema    json.RawMessage
	Tools     []Tool
	MaxTokens int
}

// ChatResult carries the output and metrics; nil pointers mean the server did not report the metric.
// CLIStartupMs and CLIExitMs are the Claude CLI's own time, in TTFT and TotalMs, reported by the bridge only.
// DefaultTemperature means the model ignored Temperature and sampled at its own default.
type ChatResult struct {
	Output             string
	ToolCalls          []ToolCall
	TTFTMs             *int64
	TotalMs            int64
	LoadMs             *int64
	PromptTokens       *int
	OutputTokens       *int
	TokPerSec          *float64
	CLIStartupMs       *int64
	CLIExitMs          *int64
	DefaultTemperature bool
}

// Model is an installed or served model; a nil Vision means the server cannot tell whether it reads images.
type Model struct {
	Name      string `json:"name"`
	SizeBytes int64  `json:"size_bytes"`
	Family    string `json:"family,omitempty"`
	Quant     string `json:"quantization,omitempty"`
	Vision    *bool  `json:"vision,omitempty"`
}

// PullProgress is one step of a model download.
type PullProgress struct {
	Status    string `json:"status"`
	Completed int64  `json:"completed"`
	Total     int64  `json:"total"`
}

// Provider is the contract every model server implements.
type Provider interface {
	Chat(ctx context.Context, req ChatRequest, onDelta func(string)) (ChatResult, error)
	Embed(ctx context.Context, model string, inputs []string) ([][]float32, error)
	Models(ctx context.Context) ([]Model, error)
}

// Puller is implemented by servers able to download models.
type Puller interface {
	Pull(ctx context.Context, name string, onProgress func(PullProgress)) error
}

// New builds the provider matching kind.
// @param kind - "ollama" or "openai"
// @param baseURL - server root (with /v1 for openai)
// @param apiKey - bearer token, empty when none
func New(kind, baseURL, apiKey string) (Provider, error) {
	switch kind {
	case "ollama":
		return &Ollama{BaseURL: baseURL}, nil
	case "openai":
		return &OpenAI{BaseURL: baseURL, APIKey: apiKey}, nil
	}
	return nil, fmt.Errorf("unknown provider kind %q", kind)
}

// wireTools wraps each tool the way both APIs declare a function.
func wireTools(tools []Tool) []map[string]any {
	out := make([]map[string]any, len(tools))
	for i, tool := range tools {
		out[i] = map[string]any{"type": "function", "function": tool}
	}
	return out
}

// postJSON sends body and returns the response once its status is checked; caller closes the body.
// No client timeout: generations stream for minutes, so callers bound them through ctx.
func postJSON(ctx context.Context, url, apiKey string, body any) (*http.Response, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("encode request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	return send(req, apiKey)
}

// getJSON fetches url and decodes the response into out.
func getJSON(ctx context.Context, url, apiKey string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	resp, err := send(req, apiKey)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decode %s: %w", url, err)
	}
	return nil
}

// send performs req and turns any non-2xx status into an error holding the body excerpt.
func send(req *http.Request, apiKey string) (*http.Response, error) {
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("call %s: %w", req.URL.Path, err)
	}
	if resp.StatusCode/100 != 2 {
		defer resp.Body.Close()
		excerpt, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBody))
		return nil, fmt.Errorf("%s returned %d: %s", req.URL.Path, resp.StatusCode, bytes.TrimSpace(excerpt))
	}
	return resp, nil
}

// lines yields each non-empty line of a streamed body; long lines are allowed for large deltas.
func lines(r io.Reader, fn func([]byte) error) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		if err := fn(line); err != nil {
			return err
		}
	}
	return scanner.Err()
}

// msSince returns elapsed milliseconds since start.
func msSince(start time.Time) int64 {
	return time.Since(start).Milliseconds()
}

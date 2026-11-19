package provider

import (
	"bytes"
	"cmp"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// reasoningMaxTokens leaves a reasoning model room for the thinking it spends before answering.
const reasoningMaxTokens = 16384

// OpenAI calls any OpenAI-compatible server (vLLM, llama.cpp, cloud APIs).
type OpenAI struct {
	BaseURL string
	APIKey  string
}

// openAIChunk is one SSE event of /chat/completions.
type openAIChunk struct {
	Choices []struct {
		Delta struct {
			Content   string `json:"content"`
			ToolCalls []struct {
				Index    int    `json:"index"`
				ID       string `json:"id"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"delta"`
	} `json:"choices"`
	Usage *struct {
		PromptTokens     int    `json:"prompt_tokens"`
		CompletionTokens int    `json:"completion_tokens"`
		CLIStartupMs     *int64 `json:"cli_startup_ms"`
		CLIExitMs        *int64 `json:"cli_exit_ms"`
	} `json:"usage"`
}

// Chat streams a generation; a model rejecting the sampling parameters is retried the way reasoning models want them.
func (o *OpenAI) Chat(ctx context.Context, req ChatRequest, onDelta func(string)) (ChatResult, error) {
	result, err := o.chat(ctx, req, onDelta, false)
	if err != nil && result.TTFTMs == nil && rejectsSampling(err) {
		return o.chat(ctx, req, onDelta, true)
	}
	return result, err
}

// rejectsSampling tells a refused temperature or max_tokens, as o-series, gpt-5 and recent Claude models answer.
// @param err - chat error, retried only when nothing was streamed yet
// @returns bool - true when a reasoning-style retry may succeed
func rejectsSampling(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "temperature") || strings.Contains(msg, "max_tokens")
}

// chat runs one /chat/completions call; token rate is derived client-side from reported usage.
// @param reasoning - drop Temperature and send a widened max_completion_tokens instead of max_tokens
func (o *OpenAI) chat(ctx context.Context, req ChatRequest, onDelta func(string), reasoning bool) (ChatResult, error) {
	body := map[string]any{
		"model":          req.Model,
		"messages":       openAIMessages(req.Messages),
		"stream":         true,
		"stream_options": map[string]any{"include_usage": true},
	}
	if reasoning {
		body["max_completion_tokens"] = max(req.MaxTokens, reasoningMaxTokens)
	} else {
		body["max_tokens"] = req.MaxTokens
		body["temperature"] = Temperature
	}
	if len(req.Schema) > 0 {
		schema, err := strictSchema(req.Schema)
		if err != nil {
			return ChatResult{}, err
		}
		body["response_format"] = map[string]any{
			"type":        "json_schema",
			"json_schema": map[string]any{"name": "answer", "strict": true, "schema": schema},
		}
	}
	if len(req.Tools) > 0 {
		body["tools"] = wireTools(req.Tools)
	}

	start := time.Now()
	resp, err := postJSON(ctx, o.BaseURL+"/chat/completions", o.APIKey, body)
	if err != nil {
		return ChatResult{}, err
	}
	defer resp.Body.Close()

	var output strings.Builder
	result := ChatResult{DefaultTemperature: reasoning}
	err = lines(resp.Body, func(line []byte) error {
		payload, ok := bytes.CutPrefix(line, []byte("data:"))
		payload = bytes.TrimSpace(payload)
		if !ok || string(payload) == "[DONE]" {
			return nil
		}
		var chunk openAIChunk
		if err := json.Unmarshal(payload, &chunk); err != nil {
			return fmt.Errorf("decode chat chunk: %w", err)
		}
		if len(chunk.Choices) > 0 {
			delta := chunk.Choices[0].Delta
			if delta.Content != "" || len(delta.ToolCalls) > 0 {
				setTTFT(&result, start)
			}
			if delta.Content != "" {
				output.WriteString(delta.Content)
				onDelta(delta.Content)
			}
			for _, piece := range delta.ToolCalls {
				if piece.Index < 0 {
					return fmt.Errorf("invalid tool call index %d", piece.Index)
				}
				if piece.Index > len(result.ToolCalls) {
					return fmt.Errorf("tool call %d streamed before call %d", piece.Index, len(result.ToolCalls))
				}
				index := piece.Index
				// Servers leaving index out send each call whole, all read as index 0: a new ID is a new call.
				if index < len(result.ToolCalls) && piece.ID != "" && result.ToolCalls[index].ID != "" && piece.ID != result.ToolCalls[index].ID {
					index = len(result.ToolCalls)
				}
				if index == len(result.ToolCalls) {
					result.ToolCalls = append(result.ToolCalls, ToolCall{})
				}
				call := &result.ToolCalls[index]
				call.ID = cmp.Or(piece.ID, call.ID)
				call.Name = cmp.Or(piece.Function.Name, call.Name)
				call.Arguments += piece.Function.Arguments
			}
		}
		if chunk.Usage != nil {
			result.PromptTokens = &chunk.Usage.PromptTokens
			result.OutputTokens = &chunk.Usage.CompletionTokens
			result.CLIStartupMs, result.CLIExitMs = chunk.Usage.CLIStartupMs, chunk.Usage.CLIExitMs
		}
		return nil
	})
	result.Output = output.String()
	result.TotalMs = msSince(start)
	setOpenAIRate(&result)
	return result, err
}

// strictSchema returns a copy of schema closed to extra keys, which cloud APIs require in strict mode.
// The case keeps its own schema: grading still checks the answer against it.
// @param schema - case schema, every property required (enforced by the case tests)
// @returns map[string]any - closed copy for response_format
func strictSchema(schema json.RawMessage) (map[string]any, error) {
	var root map[string]any
	if err := json.Unmarshal(schema, &root); err != nil {
		return nil, fmt.Errorf("decode schema: %w", err)
	}
	closeObjects(root)
	return root, nil
}

// closeObjects sets additionalProperties to false on every object of the schema tree.
// @param node - schema node, modified in place
func closeObjects(node map[string]any) {
	if node["type"] == "object" {
		node["additionalProperties"] = false
	}
	if properties, ok := node["properties"].(map[string]any); ok {
		for _, property := range properties {
			if child, ok := property.(map[string]any); ok {
				closeObjects(child)
			}
		}
	}
	if items, ok := node["items"].(map[string]any); ok {
		closeObjects(items)
	}
}

// openAIMessages converts each message to this API's wire format.
func openAIMessages(messages []Message) []any {
	out := make([]any, len(messages))
	for i, message := range messages {
		out[i] = openAIMessage(message)
	}
	return out
}

// openAIMessage rewords what this API words differently: tool calls with their arguments as a string,
// tool answers without Ollama's tool_name (strict servers reject it), images as content parts.
// @param message - one message of the conversation
// @returns any - message as this API reads it
func openAIMessage(message Message) any {
	switch {
	case len(message.ToolCalls) > 0:
		calls := make([]map[string]any, len(message.ToolCalls))
		for i, call := range message.ToolCalls {
			calls[i] = map[string]any{"id": call.ID, "type": "function", "function": map[string]string{"name": call.Name, "arguments": cmp.Or(call.Arguments, "{}")}}
		}
		return map[string]any{"role": message.Role, "content": message.Content, "tool_calls": calls}
	case message.Role == "tool":
		return map[string]string{"role": message.Role, "tool_call_id": message.ToolCallID, "content": message.Content}
	case len(message.Images) == 0:
		return message
	}
	parts := []map[string]any{{"type": "text", "text": message.Content}}
	for _, image := range message.Images {
		parts = append(parts, map[string]any{"type": "image_url", "image_url": map[string]string{"url": imageDataURL(image)}})
	}
	return map[string]any{"role": message.Role, "content": parts}
}

// imageDataURL wraps a base64 picture, its media type sniffed from the first bytes.
func imageDataURL(image string) string {
	head, _ := base64.StdEncoding.DecodeString(image[:min(len(image), 88)])
	return "data:" + http.DetectContentType(head) + ";base64," + image
}

// setOpenAIRate computes tokens/s over the generation phase, once TTFT and usage are both known.
// The CLI exit comes after the last token, so it is left out of that phase.
func setOpenAIRate(result *ChatResult) {
	if result.OutputTokens == nil || result.TTFTMs == nil {
		return
	}
	generationMs := result.TotalMs - *result.TTFTMs
	if result.CLIExitMs != nil {
		generationMs -= *result.CLIExitMs
	}
	if generationMs > 0 {
		rate := float64(*result.OutputTokens) / (float64(generationMs) / 1000)
		result.TokPerSec = &rate
	}
}

// Embed returns one vector per input.
func (o *OpenAI) Embed(ctx context.Context, model string, inputs []string) ([][]float32, error) {
	resp, err := postJSON(ctx, o.BaseURL+"/embeddings", o.APIKey, map[string]any{"model": model, "input": inputs})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var out struct {
		Data []struct {
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode embeddings: %w", err)
	}
	if len(out.Data) != len(inputs) {
		return nil, fmt.Errorf("got %d embeddings for %d inputs", len(out.Data), len(inputs))
	}
	vectors := make([][]float32, len(out.Data))
	for i, d := range out.Data {
		vectors[i] = d.Embedding
	}
	return vectors, nil
}

// Models lists what the server exposes; the API gives no size.
func (o *OpenAI) Models(ctx context.Context) ([]Model, error) {
	var out struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := getJSON(ctx, o.BaseURL+"/models", o.APIKey, &out); err != nil {
		return nil, err
	}
	models := make([]Model, 0, len(out.Data))
	for _, m := range out.Data {
		models = append(models, Model{Name: m.ID})
	}
	return models, nil
}

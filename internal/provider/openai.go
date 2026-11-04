package provider

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// OpenAI calls any OpenAI-compatible server (vLLM, llama.cpp, cloud APIs).
type OpenAI struct {
	BaseURL string
	APIKey  string
}

// openAIChunk is one SSE event of /chat/completions.
type openAIChunk struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
	} `json:"choices"`
	Usage *struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
}

// Chat streams a generation; token rate is derived client-side from reported usage.
func (o *OpenAI) Chat(ctx context.Context, req ChatRequest, onDelta func(string)) (ChatResult, error) {
	body := map[string]any{
		"model":          req.Model,
		"messages":       openAIMessages(req.Messages),
		"stream":         true,
		"stream_options": map[string]any{"include_usage": true},
		"temperature":    Temperature,
		"max_tokens":     req.MaxTokens,
	}
	if len(req.Schema) > 0 {
		body["response_format"] = map[string]any{
			"type":        "json_schema",
			"json_schema": map[string]any{"name": "answer", "schema": json.RawMessage(req.Schema)},
		}
	}

	start := time.Now()
	resp, err := postJSON(ctx, o.BaseURL+"/chat/completions", o.APIKey, body)
	if err != nil {
		return ChatResult{}, err
	}
	defer resp.Body.Close()

	var output strings.Builder
	var result ChatResult
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
		if len(chunk.Choices) > 0 && chunk.Choices[0].Delta.Content != "" {
			if result.TTFTMs == nil {
				ttft := msSince(start)
				result.TTFTMs = &ttft
			}
			output.WriteString(chunk.Choices[0].Delta.Content)
			onDelta(chunk.Choices[0].Delta.Content)
		}
		if chunk.Usage != nil {
			result.PromptTokens = &chunk.Usage.PromptTokens
			result.OutputTokens = &chunk.Usage.CompletionTokens
		}
		return nil
	})
	result.Output = output.String()
	result.TotalMs = msSince(start)
	setOpenAIRate(&result)
	return result, err
}

// openAIMessages turns a message with images into content parts, the only form this API accepts for them.
func openAIMessages(messages []Message) []any {
	out := make([]any, 0, len(messages))
	for _, message := range messages {
		if len(message.Images) == 0 {
			out = append(out, message)
			continue
		}
		parts := []map[string]any{{"type": "text", "text": message.Content}}
		for _, image := range message.Images {
			parts = append(parts, map[string]any{"type": "image_url", "image_url": map[string]string{"url": imageDataURL(image)}})
		}
		out = append(out, map[string]any{"role": message.Role, "content": parts})
	}
	return out
}

// imageDataURL wraps a base64 picture, its media type sniffed from the first bytes.
func imageDataURL(image string) string {
	head, _ := base64.StdEncoding.DecodeString(image[:min(len(image), 88)])
	return "data:" + http.DetectContentType(head) + ";base64," + image
}

// setOpenAIRate computes tokens/s over the generation phase, once TTFT and usage are both known.
func setOpenAIRate(result *ChatResult) {
	if result.OutputTokens == nil || result.TTFTMs == nil {
		return
	}
	if generationMs := result.TotalMs - *result.TTFTMs; generationMs > 0 {
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

package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
)

// ollamaNumCtx is fixed so every model gets the same context window.
const ollamaNumCtx = 8192

// ContextWindow is the tokens a model can read and write in one generation; callers size what they send from it.
const ContextWindow = ollamaNumCtx

// Ollama calls a local or remote Ollama server.
type Ollama struct {
	BaseURL string
}

// ollamaChunk is one line of the /api/chat stream.
type ollamaChunk struct {
	Message struct {
		Content string `json:"content"`
	} `json:"message"`
	Done            bool   `json:"done"`
	Error           string `json:"error"`
	LoadDuration    int64  `json:"load_duration"`
	PromptEvalCount int    `json:"prompt_eval_count"`
	EvalCount       int    `json:"eval_count"`
	EvalDuration    int64  `json:"eval_duration"`
}

// Chat streams a generation with thinking off; models rejecting the option are retried without it.
func (o *Ollama) Chat(ctx context.Context, req ChatRequest, onDelta func(string)) (ChatResult, error) {
	result, err := o.chat(ctx, req, onDelta, true)
	if err != nil && strings.Contains(err.Error(), "does not support thinking") {
		return o.chat(ctx, req, onDelta, false)
	}
	return result, err
}

// chat runs one /api/chat call.
// @param withThinkOff - send think=false, rejected by models without a thinking mode
func (o *Ollama) chat(ctx context.Context, req ChatRequest, onDelta func(string), withThinkOff bool) (ChatResult, error) {
	body := map[string]any{
		"model":    req.Model,
		"messages": req.Messages,
		"stream":   true,
		"options": map[string]any{
			"temperature": Temperature,
			"num_ctx":     ollamaNumCtx,
			"num_predict": req.MaxTokens,
		},
	}
	if withThinkOff {
		body["think"] = false
	}
	if len(req.Schema) > 0 {
		body["format"] = json.RawMessage(req.Schema)
	}

	start := time.Now()
	resp, err := postJSON(ctx, o.BaseURL+"/api/chat", "", body)
	if err != nil {
		return ChatResult{}, err
	}
	defer resp.Body.Close()

	var output strings.Builder
	var result ChatResult
	err = lines(resp.Body, func(line []byte) error {
		var chunk ollamaChunk
		if err := json.Unmarshal(line, &chunk); err != nil {
			return fmt.Errorf("decode chat chunk: %w", err)
		}
		if chunk.Error != "" {
			return errors.New(chunk.Error)
		}
		if chunk.Message.Content != "" {
			if result.TTFTMs == nil {
				ttft := msSince(start)
				result.TTFTMs = &ttft
			}
			output.WriteString(chunk.Message.Content)
			onDelta(chunk.Message.Content)
		}
		if chunk.Done {
			setOllamaMetrics(&result, chunk)
		}
		return nil
	})
	result.Output = output.String()
	result.TotalMs = msSince(start)
	return result, err
}

// setOllamaMetrics copies the server-side counters of the final chunk.
func setOllamaMetrics(result *ChatResult, chunk ollamaChunk) {
	load := chunk.LoadDuration / int64(time.Millisecond)
	result.LoadMs = &load
	result.PromptTokens = &chunk.PromptEvalCount
	result.OutputTokens = &chunk.EvalCount
	if chunk.EvalDuration > 0 {
		rate := float64(chunk.EvalCount) / (float64(chunk.EvalDuration) / float64(time.Second))
		result.TokPerSec = &rate
	}
}

// Embed returns one vector per input.
func (o *Ollama) Embed(ctx context.Context, model string, inputs []string) ([][]float32, error) {
	resp, err := postJSON(ctx, o.BaseURL+"/api/embed", "", map[string]any{"model": model, "input": inputs})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var out struct {
		Embeddings [][]float32 `json:"embeddings"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode embeddings: %w", err)
	}
	if len(out.Embeddings) != len(inputs) {
		return nil, fmt.Errorf("got %d embeddings for %d inputs", len(out.Embeddings), len(inputs))
	}
	return out.Embeddings, nil
}

// Models lists the models installed on the server.
func (o *Ollama) Models(ctx context.Context) ([]Model, error) {
	var out struct {
		Models []struct {
			Name    string `json:"name"`
			Size    int64  `json:"size"`
			Details struct {
				Family string `json:"family"`
				Quant  string `json:"quantization_level"`
			} `json:"details"`
		} `json:"models"`
	}
	if err := getJSON(ctx, o.BaseURL+"/api/tags", "", &out); err != nil {
		return nil, err
	}
	models := make([]Model, 0, len(out.Models))
	for _, m := range out.Models {
		models = append(models, Model{Name: m.Name, SizeBytes: m.Size, Family: m.Details.Family, Quant: m.Details.Quant, Vision: o.vision(ctx, m.Name)})
	}
	return models, nil
}

// vision asks the server whether a model reads images, nil when it cannot tell.
func (o *Ollama) vision(ctx context.Context, name string) *bool {
	resp, err := postJSON(ctx, o.BaseURL+"/api/show", "", map[string]any{"model": name})
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	var out struct {
		Capabilities []string `json:"capabilities"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil || out.Capabilities == nil {
		return nil
	}
	capable := slices.Contains(out.Capabilities, "vision")
	return &capable
}

// Pull downloads a model, reporting progress until the server closes the stream.
func (o *Ollama) Pull(ctx context.Context, name string, onProgress func(PullProgress)) error {
	resp, err := postJSON(ctx, o.BaseURL+"/api/pull", "", map[string]any{"model": name, "stream": true})
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return lines(resp.Body, func(line []byte) error {
		var chunk struct {
			PullProgress
			Error string `json:"error"`
		}
		if err := json.Unmarshal(line, &chunk); err != nil {
			return fmt.Errorf("decode pull chunk: %w", err)
		}
		if chunk.Error != "" {
			return errors.New(chunk.Error)
		}
		onProgress(chunk.PullProgress)
		return nil
	})
}

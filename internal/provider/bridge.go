package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
)

// Abandoner is implemented by servers keeping a run waiting for tool results, to be told when none will come.
type Abandoner interface {
	Abandon(ctx context.Context, callID string) error
}

// Bridge is the Claude bridge: an OpenAI server whose Claude Code run waits for the results of its tool calls.
type Bridge struct {
	OpenAI
}

// Abandon ends the run waiting for the result of tool call callID; a run already over is no error.
func (b *Bridge) Abandon(ctx context.Context, callID string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, b.BaseURL+"/tool_calls/"+url.PathEscape(callID), nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	resp, err := send(req, b.APIKey)
	if err != nil {
		return err
	}
	return resp.Body.Close()
}

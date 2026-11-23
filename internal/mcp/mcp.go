// Package mcp reaches MCP servers over streamable HTTP and lists their tools.
package mcp

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// client identifies Axolot to every server.
var client = sdk.NewClient(&sdk.Implementation{Name: "axolot", Version: "1"}, nil)

// ToolNames connects to a server, lists the names of all its tools, then closes the session.
// @param token - plain bearer token, empty when the server needs none
func ToolNames(ctx context.Context, endpoint, token string) ([]string, error) {
	session, err := connect(ctx, endpoint, token)
	if err != nil {
		return nil, err
	}
	defer session.Close()
	names := []string{}
	for tool, err := range session.Tools(ctx, nil) {
		if err != nil {
			return nil, fmt.Errorf("list tools: %w", err)
		}
		names = append(names, tool.Name)
	}
	return names, nil
}

// connect opens a request-response session: no standing SSE stream, no reconnect, the caller's context bounds it.
func connect(ctx context.Context, endpoint, token string) (*sdk.ClientSession, error) {
	httpClient := http.DefaultClient
	if token != "" {
		parsed, err := url.Parse(endpoint)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", endpoint, err)
		}
		httpClient = &http.Client{Transport: bearer{token: token, host: parsed.Host, next: http.DefaultTransport}}
	}
	transport := &sdk.StreamableClientTransport{Endpoint: endpoint, HTTPClient: httpClient, DisableStandaloneSSE: true, MaxRetries: -1}
	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		return nil, fmt.Errorf("connect %s: %w", endpoint, err)
	}
	return session, nil
}

// bearer adds the server token to requests for the server's own host.
// The transport sees every redirect hop, so http.Client's own cross-host header stripping never applies here.
type bearer struct {
	token string
	host  string
	next  http.RoundTripper
}

// RoundTrip sends a copy of the request carrying the token, or the request untouched when it left the host.
func (b bearer) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.URL.Host != b.host {
		return b.next.RoundTrip(r)
	}
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", "Bearer "+b.token)
	return b.next.RoundTrip(r)
}

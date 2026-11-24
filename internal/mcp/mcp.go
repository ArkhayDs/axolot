// Package mcp reaches MCP servers over streamable HTTP, lists their tools and calls them.
package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/modelcontextprotocol/go-sdk/oauthex"

	"github.com/arkhayds/axolot/internal/provider"
)

// client identifies Axolot to every server.
var client = sdk.NewClient(&sdk.Implementation{Name: "axolot", Version: "1"}, nil)

// ToolNames connects to a server, lists the names of all its tools, then closes the session.
// @param token - plain bearer token, empty when the server needs none
func ToolNames(ctx context.Context, endpoint, token string) ([]string, error) {
	session, err := Open(ctx, endpoint, token)
	if err != nil {
		return nil, err
	}
	defer session.Close()
	tools, err := session.Tools(ctx)
	if err != nil {
		return nil, err
	}
	names := make([]string, len(tools))
	for i, tool := range tools {
		names[i] = tool.Name
	}
	return names, nil
}

// Session is an open connection to one server; the caller closes it.
type Session struct {
	session *sdk.ClientSession
}

// Open connects to a server in request-response mode: no standing SSE stream, no reconnect, the caller's context bounds it.
// @param token - plain bearer token, empty when the server needs none
func Open(ctx context.Context, endpoint, token string) (*Session, error) {
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", endpoint, err)
	}
	httpClient := &http.Client{Transport: auth{token: token, host: parsed.Host, next: http.DefaultTransport}}
	transport := &sdk.StreamableClientTransport{Endpoint: endpoint, HTTPClient: httpClient, DisableStandaloneSSE: true, MaxRetries: -1}
	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		return nil, fmt.Errorf("connect %s: %w", endpoint, err)
	}
	return &Session{session: session}, nil
}

// Close ends the session.
func (s *Session) Close() error {
	return s.session.Close()
}

// Tools lists the server's tools as a model is offered them.
func (s *Session) Tools(ctx context.Context) ([]provider.Tool, error) {
	tools := []provider.Tool{}
	for tool, err := range s.session.Tools(ctx, nil) {
		if err != nil {
			return nil, fmt.Errorf("list tools: %w", err)
		}
		schema, err := json.Marshal(tool.InputSchema)
		if err != nil {
			return nil, fmt.Errorf("encode schema of %s: %w", tool.Name, err)
		}
		tools = append(tools, provider.Tool{Name: tool.Name, Description: tool.Description, Parameters: schema})
	}
	return tools, nil
}

// Call runs one tool and returns its text; a tool reporting a failure gives an error carrying that text.
// shortcut: images and other non-text content are dropped, read them if a server answers with some.
// @param arguments - JSON object written by the model, empty for none
func (s *Session) Call(ctx context.Context, name, arguments string) (string, error) {
	args := map[string]any{}
	if strings.TrimSpace(arguments) != "" {
		if err := json.Unmarshal([]byte(arguments), &args); err != nil {
			return "", fmt.Errorf("arguments are not a JSON object: %w", err)
		}
	}
	result, err := s.session.CallTool(ctx, &sdk.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		return "", fmt.Errorf("call %s: %w", name, err)
	}
	var texts []string
	for _, content := range result.Content {
		if text, ok := content.(*sdk.TextContent); ok {
			texts = append(texts, text.Text)
		}
	}
	text := strings.Join(texts, "\n")
	if result.IsError {
		return "", errors.New(text)
	}
	return text, nil
}

// ErrUnauthorized reports a server refusing the token, or asking for one.
var ErrUnauthorized = errors.New("unauthorized")

// ErrSignInOffered is a 401 naming the server's OAuth metadata: a browser sign-in could get a token.
// shortcut: servers publishing their metadata only at the well-known URL, without the header, are missed; probe it if one shows up.
var ErrSignInOffered = fmt.Errorf("%w: browser sign-in offered", ErrUnauthorized)

// auth adds the token to requests for the server's own host and turns a 401 or 403 into ErrUnauthorized.
// The transport sees every redirect hop, so http.Client's own cross-host header stripping never applies here.
type auth struct {
	token string
	host  string
	next  http.RoundTripper
}

// RoundTrip sends the request, carrying the token only while it stays on the server's host.
func (a auth) RoundTrip(r *http.Request) (*http.Response, error) {
	if a.token != "" && r.URL.Host == a.host {
		r = r.Clone(r.Context())
		r.Header.Set("Authorization", "Bearer "+a.token)
	}
	resp, err := a.next.RoundTrip(r)
	if err != nil || (resp.StatusCode != http.StatusUnauthorized && resp.StatusCode != http.StatusForbidden) {
		return resp, err
	}
	resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized && offersSignIn(resp.Header) {
		return nil, ErrSignInOffered
	}
	return nil, ErrUnauthorized
}

// offersSignIn tells whether a 401 points to the server's OAuth metadata.
func offersSignIn(h http.Header) bool {
	challenges, err := oauthex.ParseWWWAuthenticate(h.Values("WWW-Authenticate"))
	return err == nil && slices.ContainsFunc(challenges, func(c oauthex.Challenge) bool { return c.Params["resource_metadata"] != "" })
}

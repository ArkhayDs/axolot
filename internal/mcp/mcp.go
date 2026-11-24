// Package mcp reaches MCP servers over streamable HTTP and lists their tools.
package mcp

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/modelcontextprotocol/go-sdk/oauthex"
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

// ErrUnauthorized reports a server refusing the token, or asking for one.
var ErrUnauthorized = errors.New("unauthorized")

// ErrSignInOffered is a 401 naming the server's OAuth metadata: a browser sign-in could get a token.
// shortcut: servers publishing their metadata only at the well-known URL, without the header, are missed; probe it if one shows up.
var ErrSignInOffered = fmt.Errorf("%w: browser sign-in offered", ErrUnauthorized)

// connect opens a request-response session: no standing SSE stream, no reconnect, the caller's context bounds it.
func connect(ctx context.Context, endpoint, token string) (*sdk.ClientSession, error) {
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
	return session, nil
}

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

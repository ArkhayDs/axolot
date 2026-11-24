// Package mcptest serves fake MCP servers for tests of the packages that reach them.
package mcptest

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// NewOAuthServer is an MCP server and its own authorization server on one host, like Sentinelle.
// The MCP endpoint is URL+"/mcp". Registration hands out client "c1", the code "code-1" buys the bearer "access-1"
// with refresh token "refresh-1", which rotates to "access-2" and "refresh-2".
func NewOAuthServer(t *testing.T) *httptest.Server {
	t.Helper()
	server := sdk.NewServer(&sdk.Implementation{Name: "test", Version: "1"}, nil)
	server.AddTool(&sdk.Tool{Name: "get_kpis", InputSchema: json.RawMessage(`{"type":"object"}`)},
		func(context.Context, *sdk.CallToolRequest) (*sdk.CallToolResult, error) {
			return &sdk.CallToolResult{}, nil
		})
	mcpHandler := sdk.NewStreamableHTTPHandler(func(*http.Request) *sdk.Server { return server }, nil)
	mux := http.NewServeMux()
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	answer := func(w http.ResponseWriter, status int, body any) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(body)
	}
	mux.HandleFunc("/mcp", func(w http.ResponseWriter, r *http.Request) {
		if bearer := r.Header.Get("Authorization"); bearer != "Bearer access-1" && bearer != "Bearer access-2" {
			w.Header().Set("WWW-Authenticate", `Bearer resource_metadata="`+ts.URL+`/.well-known/oauth-protected-resource"`)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		mcpHandler.ServeHTTP(w, r)
	})
	mux.HandleFunc("/.well-known/oauth-protected-resource", func(w http.ResponseWriter, r *http.Request) {
		answer(w, http.StatusOK, map[string]any{"resource": ts.URL + "/mcp", "authorization_servers": []string{ts.URL}, "scopes_supported": []string{"read"}})
	})
	mux.HandleFunc("/.well-known/oauth-authorization-server", func(w http.ResponseWriter, r *http.Request) {
		answer(w, http.StatusOK, map[string]any{
			"issuer":                           ts.URL,
			"authorization_endpoint":           ts.URL + "/authorize",
			"token_endpoint":                   ts.URL + "/token",
			"registration_endpoint":            ts.URL + "/register",
			"response_types_supported":         []string{"code"},
			"code_challenge_methods_supported": []string{"S256"},
		})
	})
	mux.HandleFunc("/register", func(w http.ResponseWriter, r *http.Request) {
		var meta map[string]any
		_ = json.NewDecoder(r.Body).Decode(&meta)
		meta["client_id"] = "c1"
		answer(w, http.StatusCreated, meta)
	})
	mux.HandleFunc("/authorize", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		http.Redirect(w, r, q.Get("redirect_uri")+"?code=code-1&state="+url.QueryEscape(q.Get("state")), http.StatusFound)
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.FormValue("code") == "code-1" && r.FormValue("code_verifier") != "":
			answer(w, http.StatusOK, map[string]any{"access_token": "access-1", "token_type": "Bearer", "refresh_token": "refresh-1", "expires_in": 3600})
		case r.FormValue("grant_type") == "refresh_token" && r.FormValue("refresh_token") == "refresh-1":
			answer(w, http.StatusOK, map[string]any{"access_token": "access-2", "token_type": "Bearer", "refresh_token": "refresh-2", "expires_in": 3600})
		default:
			answer(w, http.StatusBadRequest, map[string]string{"error": "invalid_grant"})
		}
	})
	return ts
}

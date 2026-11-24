package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	sdkauth "github.com/modelcontextprotocol/go-sdk/auth"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// newOAuthServer is an MCP server and its own authorization server on one host, like Sentinelle.
// Registration hands out client "c1", the code "code-1" buys the bearer "access-1".
func newOAuthServer(t *testing.T) *httptest.Server {
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
		if r.Header.Get("Authorization") != "Bearer access-1" {
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
		if r.FormValue("code") != "code-1" || r.FormValue("code_verifier") == "" {
			answer(w, http.StatusBadRequest, map[string]string{"error": "invalid_grant"})
			return
		}
		answer(w, http.StatusOK, map[string]any{"access_token": "access-1", "token_type": "Bearer", "refresh_token": "refresh-1", "expires_in": 3600})
	})
	return ts
}

// browser follows the sign-in URL as a user who accepts would, and reads the code off the redirect.
func browser(ctx context.Context, args *sdkauth.AuthorizationArgs) (*sdkauth.AuthorizationResult, error) {
	noFollow := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := noFollow.Get(args.URL)
	if err != nil {
		return nil, err
	}
	resp.Body.Close()
	back, err := url.Parse(resp.Header.Get("Location"))
	if err != nil {
		return nil, err
	}
	return &sdkauth.AuthorizationResult{Code: back.Query().Get("code"), State: back.Query().Get("state")}, nil
}

func TestLoginReturnsCredsThatListTheTools(t *testing.T) {
	ts := newOAuthServer(t)
	creds, err := Login(context.Background(), ts.URL+"/mcp", "http://localhost:5090/api/mcp/callback", browser)
	if err != nil {
		t.Fatal(err)
	}
	if creds.Config.ClientID != "c1" || creds.Config.Endpoint.TokenURL != ts.URL+"/token" || creds.Token.RefreshToken != "refresh-1" {
		t.Fatalf("creds=%+v token=%+v", creds.Config, creds.Token)
	}
	names, err := ToolNames(context.Background(), ts.URL+"/mcp", creds.Token.AccessToken)
	if err != nil || len(names) != 1 {
		t.Fatalf("names=%v err=%v", names, err)
	}
}

func TestLoginFailsWhenTheUserNeverComesBack(t *testing.T) {
	ts := newOAuthServer(t)
	gone := errors.New("sign-in abandoned")
	fetch := func(context.Context, *sdkauth.AuthorizationArgs) (*sdkauth.AuthorizationResult, error) {
		return nil, gone
	}
	if _, err := Login(context.Background(), ts.URL+"/mcp", "http://localhost:5090/api/mcp/callback", fetch); !errors.Is(err, gone) {
		t.Fatalf("err=%v", err)
	}
}

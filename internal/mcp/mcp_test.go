package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/arkhayds/axolot/internal/mcp/mcptest"
	"github.com/arkhayds/axolot/internal/provider"
)

// newServer serves get_kpis, which echoes its arguments, broken, which always fails, and mute, which fails with no text;
// it answers 401 to requests without the bearer token "secret".
func newServer(t *testing.T) *httptest.Server {
	t.Helper()
	server := sdk.NewServer(&sdk.Implementation{Name: "test", Version: "1"}, nil)
	server.AddTool(&sdk.Tool{
		Name:        "get_kpis",
		Description: "KPIs of a scope",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"scope":{"type":"string"}}}`),
	}, func(_ context.Context, req *sdk.CallToolRequest) (*sdk.CallToolResult, error) {
		return &sdk.CallToolResult{Content: []sdk.Content{&sdk.TextContent{Text: "kpis for " + string(req.Params.Arguments)}}}, nil
	})
	server.AddTool(&sdk.Tool{Name: "broken", InputSchema: json.RawMessage(`{"type":"object"}`)},
		func(context.Context, *sdk.CallToolRequest) (*sdk.CallToolResult, error) {
			return &sdk.CallToolResult{IsError: true, Content: []sdk.Content{&sdk.TextContent{Text: "scope unknown"}}}, nil
		})
	server.AddTool(&sdk.Tool{Name: "mute", InputSchema: json.RawMessage(`{"type":"object"}`)},
		func(context.Context, *sdk.CallToolRequest) (*sdk.CallToolResult, error) {
			return &sdk.CallToolResult{IsError: true, Content: []sdk.Content{}}, nil
		})
	handler := sdk.NewStreamableHTTPHandler(func(*http.Request) *sdk.Server { return server }, nil)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer secret" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		handler.ServeHTTP(w, r)
	}))
	t.Cleanup(ts.Close)
	return ts
}

func TestToolNamesListsWithTheBearerToken(t *testing.T) {
	ts := newServer(t)
	names, err := ToolNames(context.Background(), ts.URL, "secret")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(names, []string{"broken", "get_kpis", "mute"}) {
		t.Fatalf("names=%v", names)
	}
}

// openSession opens a session on newServer, closed with the test.
func openSession(t *testing.T) *Session {
	t.Helper()
	session, err := Open(context.Background(), newServer(t).URL, "secret")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { session.Close() })
	return session
}

func TestToolsCarryTheirSchema(t *testing.T) {
	tools, err := openSession(t).Tools(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	i := slices.IndexFunc(tools, func(tool provider.Tool) bool { return tool.Name == "get_kpis" })
	if i < 0 || tools[i].Description != "KPIs of a scope" || !strings.Contains(string(tools[i].Parameters), `"scope"`) {
		t.Fatalf("tools=%+v", tools)
	}
}

func TestCallReturnsTheToolText(t *testing.T) {
	session := openSession(t)
	for arguments, want := range map[string]string{
		`{"scope":"store-12"}`: `kpis for {"scope":"store-12"}`,
		"":                     "kpis for {}",
		"null":                 "kpis for {}",
	} {
		text, err := session.Call(context.Background(), "get_kpis", arguments)
		if err != nil || text != want {
			t.Errorf("arguments %q: text=%q err=%v", arguments, text, err)
		}
	}
}

func TestCallReportsFailures(t *testing.T) {
	session := openSession(t)
	for name, tc := range map[string]struct{ tool, arguments, want string }{
		"tool failure":    {"broken", "", "scope unknown"},
		"silent failure":  {"mute", "", "without saying why"},
		"invalid JSON":    {"get_kpis", `{"scope":`, "not a JSON object"},
		"not an object":   {"get_kpis", `["store-12"]`, "not a JSON object"},
		"unknown to them": {"missing", "", "missing"},
	} {
		if _, err := session.Call(context.Background(), tc.tool, tc.arguments); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: err=%v", name, err)
		}
	}
}

func TestToolNamesTellsWhetherASignInWouldHelp(t *testing.T) {
	forbidden := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("WWW-Authenticate", `Bearer error="insufficient_scope", resource_metadata="https://x/meta"`)
		w.WriteHeader(http.StatusForbidden)
	}))
	t.Cleanup(forbidden.Close)
	for name, tc := range map[string]struct {
		url     string
		offered bool
	}{
		"fixed token missing":         {newServer(t).URL, false},
		"oauth server":                {mcptest.NewOAuthServer(t).URL + "/mcp", true},
		"signed in, scope too narrow": {forbidden.URL, false},
	} {
		_, err := ToolNames(context.Background(), tc.url, "")
		if !errors.Is(err, ErrUnauthorized) || errors.Is(err, ErrSignInOffered) != tc.offered {
			t.Errorf("%s: err=%v", name, err)
		}
	}
}

// TestToolNamesCancelsTheRequestAtTheDeadline reads the body first: net/http only notices a client gone once it has.
// A read error only means the client already left, which the test waits for anyway.
func TestToolNamesCancelsTheRequestAtTheDeadline(t *testing.T) {
	cancelled := make(chan struct{})
	release := make(chan struct{})
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		select {
		case <-r.Context().Done():
			close(cancelled)
		case <-release:
		}
	}))
	t.Cleanup(ts.Close)
	t.Cleanup(func() { close(release) })
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := ToolNames(ctx, ts.URL, ""); err == nil {
		t.Fatal("expected an error past the deadline")
	}
	select {
	case <-cancelled:
	case <-time.After(10 * time.Second):
		t.Fatal("request still open after the deadline")
	}
}

func TestToolNamesKeepsTheTokenFromAnotherHost(t *testing.T) {
	var leaked string
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		leaked = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusUnauthorized)
	}))
	t.Cleanup(other.Close)
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL, http.StatusTemporaryRedirect)
	}))
	t.Cleanup(redirect.Close)
	if _, err := ToolNames(context.Background(), redirect.URL, "secret"); err == nil {
		t.Fatal("expected an error from the other host")
	}
	if leaked != "" {
		t.Fatalf("token sent to another host: %q", leaked)
	}
}

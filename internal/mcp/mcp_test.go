package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// newServer serves one tool, answering 401 to requests without the bearer token "secret".
func newServer(t *testing.T) *httptest.Server {
	t.Helper()
	server := sdk.NewServer(&sdk.Implementation{Name: "test", Version: "1"}, nil)
	server.AddTool(&sdk.Tool{
		Name:        "get_kpis",
		Description: "KPIs of a scope",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"scope":{"type":"string"}}}`),
	}, func(context.Context, *sdk.CallToolRequest) (*sdk.CallToolResult, error) {
		return &sdk.CallToolResult{}, nil
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
	if len(names) != 1 || names[0] != "get_kpis" {
		t.Fatalf("names=%v", names)
	}
}

func TestToolNamesFailsWithoutTheToken(t *testing.T) {
	ts := newServer(t)
	if _, err := ToolNames(context.Background(), ts.URL, ""); err == nil {
		t.Fatal("expected an error on 401")
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

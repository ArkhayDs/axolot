package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/arkhayds/axolot/internal/secret"
	"github.com/arkhayds/axolot/internal/store"
)

func TestValidateMcpServer(t *testing.T) {
	for _, tc := range []struct {
		name, url string
		ok        bool
	}{
		{"sentinelle", "https://sentinelle.example/mcp", true},
		{"my_srv-2", "http://host.docker.internal:8000/mcp", true},
		{"Sentinelle", "https://x/mcp", false},
		{"two words", "https://x/mcp", false},
		{"", "https://x/mcp", false},
		{"sentinelle", "ftp://x/mcp", false},
		{"sentinelle", "/mcp", false},
	} {
		if got := validateMcpServer(tc.name, tc.url) == ""; got != tc.ok {
			t.Errorf("%q %q: valid=%v, want %v", tc.name, tc.url, got, tc.ok)
		}
	}
}

// mcpServerWithToken serves one tool to requests carrying the bearer token "secret".
func mcpServerWithToken(t *testing.T) *httptest.Server {
	t.Helper()
	server := sdk.NewServer(&sdk.Implementation{Name: "test", Version: "1"}, nil)
	server.AddTool(&sdk.Tool{Name: "get_kpis", InputSchema: json.RawMessage(`{"type":"object"}`)},
		func(context.Context, *sdk.CallToolRequest) (*sdk.CallToolResult, error) {
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

func TestProbeMcpOpensTheSealedToken(t *testing.T) {
	ts := mcpServerWithToken(t)
	box, err := secret.NewBox("test")
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := box.Seal("secret")
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{cfg: Config{Box: box}}

	view := s.probeMcp(context.Background(), store.McpServer{URL: ts.URL, TokenEnc: sealed})
	if view.Error != "" || len(view.Tools) != 1 || view.Tools[0] != "get_kpis" {
		t.Fatalf("view=%+v", view)
	}
}

func TestProbeMcpReportsFailures(t *testing.T) {
	ts := mcpServerWithToken(t)
	box, err := secret.NewBox("test")
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{cfg: Config{Box: box}}

	for name, m := range map[string]store.McpServer{
		"no token":         {URL: ts.URL},
		"unreadable token": {URL: ts.URL, TokenEnc: []byte("garbage")},
	} {
		view := s.probeMcp(context.Background(), m)
		if view.Error == "" || view.Tools == nil || len(view.Tools) != 0 {
			t.Errorf("%s: view=%+v", name, view)
		}
	}
}

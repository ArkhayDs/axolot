package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/arkhayds/axolot/internal/store"
)

// TestClientReachesTheBridgeFromTheEnvironment uses a row with a stale URL and sealed key, as saved before both lived only in .env.
func TestClientReachesTheBridgeFromTheEnvironment(t *testing.T) {
	var got string
	bridge := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("Authorization")
		w.Write([]byte(`{"data":[]}`))
	}))
	defer bridge.Close()
	s := &Server{cfg: Config{BridgeURL: bridge.URL + "/v1", BridgeToken: "current"}}

	client, err := s.client(store.Provider{Kind: "bridge", BaseURL: "http://stale:5092/v1", HasAPIKey: true, APIKeyEnc: []byte("stale")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Models(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got != "Bearer current" {
		t.Errorf("Authorization = %q, want the environment token", got)
	}
}

func TestProbeLabelsModelsWithTheirConnectionType(t *testing.T) {
	bridge := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"data":[{"id":"claude-haiku-5-5"}]}`))
	}))
	defer bridge.Close()
	s := &Server{cfg: Config{BridgeURL: bridge.URL + "/v1"}}

	view := s.probe(context.Background(), store.Provider{Kind: "bridge"}, 0)
	if len(view.Models) != 1 || view.Models[0].Name != "claude-haiku-5-5" || view.Models[0].Label != "claude-haiku-5-5 (abo)" {
		t.Fatalf("models=%+v error=%q", view.Models, view.Error)
	}
}

func TestConnectionTypeFollowsTheConnection(t *testing.T) {
	for _, tc := range []struct {
		p    store.Provider
		want string
	}{
		{store.Provider{Kind: "ollama", BaseURL: "http://host.docker.internal:11434"}, store.ConnectionLocal},
		{store.Provider{Kind: "openai", BaseURL: "https://api.mistral.ai/v1"}, store.ConnectionAPI},
		{store.Provider{Kind: "bridge", BaseURL: "http://host.docker.internal:5092/v1"}, store.ConnectionSubscription},
	} {
		if got := connectionType(tc.p); got != tc.want {
			t.Errorf("%s %s: got %q, want %q", tc.p.Kind, tc.p.BaseURL, got, tc.want)
		}
	}
}

func TestNotCrossSiteRefusesOtherSites(t *testing.T) {
	for site, want := range map[string]int{"": http.StatusOK, "none": http.StatusOK, "same-origin": http.StatusOK, "same-site": http.StatusForbidden, "cross-site": http.StatusForbidden} {
		req := httptest.NewRequest(http.MethodGet, "/api/mcp/1/tools", nil)
		if site != "" {
			req.Header.Set("Sec-Fetch-Site", site)
		}
		rec := httptest.NewRecorder()
		notCrossSite(func(w http.ResponseWriter, r *http.Request) {})(rec, req)
		if rec.Code != want {
			t.Errorf("%q: status=%d, want %d", site, rec.Code, want)
		}
	}
}

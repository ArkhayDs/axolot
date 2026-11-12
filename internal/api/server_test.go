package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/arkhayds/axolot/internal/store"
)

// TestClientSendsTheBridgeTokenFromTheEnvironment uses a row with a stale sealed key, as saved before the token lived only in .env.
func TestClientSendsTheBridgeTokenFromTheEnvironment(t *testing.T) {
	var got string
	bridge := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("Authorization")
		w.Write([]byte(`{"data":[]}`))
	}))
	defer bridge.Close()
	s := &Server{cfg: Config{BridgeURL: bridge.URL + "/v1", BridgeToken: "current"}}

	client, err := s.client(store.Provider{Kind: "openai", BaseURL: bridge.URL + "/v1", HasAPIKey: true, APIKeyEnc: []byte("stale")})
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

func TestConnectionTypeFollowsTheConnection(t *testing.T) {
	s := &Server{cfg: Config{BridgeURL: "http://host.docker.internal:5092/v1"}}
	for _, tc := range []struct {
		p    store.Provider
		want string
	}{
		{store.Provider{Kind: "ollama", BaseURL: "http://host.docker.internal:11434"}, store.ConnectionLocal},
		{store.Provider{Kind: "openai", BaseURL: "https://api.mistral.ai/v1"}, store.ConnectionAPI},
		{store.Provider{Kind: "openai", BaseURL: "http://host.docker.internal:5092/v1"}, store.ConnectionSubscription},
	} {
		if got := s.connectionType(tc.p); got != tc.want {
			t.Errorf("%s %s: got %q, want %q", tc.p.Kind, tc.p.BaseURL, got, tc.want)
		}
	}
}

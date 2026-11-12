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

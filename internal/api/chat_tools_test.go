package api

import (
	"context"
	"slices"
	"testing"

	"github.com/arkhayds/axolot/internal/secret"
	"github.com/arkhayds/axolot/internal/store"
)

// toolboxServer is a Server able to open sealed tokens, with the sealed token "secret".
func toolboxServer(t *testing.T) (*Server, []byte) {
	t.Helper()
	box, err := secret.NewBox("test")
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := box.Seal("secret")
	if err != nil {
		t.Fatal(err)
	}
	return &Server{cfg: Config{Box: box}, oauthSources: map[int64]*oauthSource{}}, sealed
}

func TestToolboxOffersEnabledServersToolsUnderTheirName(t *testing.T) {
	ts := mcpServerWithToken(t)
	s, sealed := toolboxServer(t)

	tools := s.openToolbox(context.Background(), []store.McpServer{
		{Name: "sentinelle", URL: ts.URL, Enabled: true, TokenEnc: sealed},
		{Name: "off", URL: ts.URL, Enabled: false, TokenEnc: sealed},
	})
	defer tools.close()

	if len(tools.tools) != 1 || tools.tools[0].Name != "sentinelle__get_kpis" || len(tools.warnings) != 0 {
		t.Fatalf("tools=%+v warnings=%v", tools.tools, tools.warnings)
	}
	got, err := tools.call(context.Background(), "sentinelle__get_kpis", `{}`)
	if err != nil || got != "kpis" {
		t.Fatalf("got %q, err %v", got, err)
	}
	if _, err := tools.call(context.Background(), "get_kpis", `{}`); err == nil {
		t.Fatal("unknown tool called")
	}
}

func TestToolboxWarnsAboutServersItCannotOpen(t *testing.T) {
	ts := mcpServerWithToken(t)
	s, sealed := toolboxServer(t)

	tools := s.openToolbox(context.Background(), []store.McpServer{
		{Name: "ok", URL: ts.URL, Enabled: true, TokenEnc: sealed},
		{Name: "down", URL: ts.URL, Enabled: true},
		{Name: "lost", URL: ts.URL, Enabled: true, OAuthEnc: []byte("garbage")},
	})
	defer tools.close()

	want := []string{"down injoignable, outils absents", "lost : connexion requise, outils absents"}
	if !slices.Equal(tools.warnings, want) || len(tools.tools) != 1 {
		t.Fatalf("tools=%+v warnings=%v", tools.tools, tools.warnings)
	}
}

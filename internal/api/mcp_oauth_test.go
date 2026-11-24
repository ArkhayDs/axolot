package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"golang.org/x/oauth2"

	"github.com/arkhayds/axolot/internal/mcp"
	"github.com/arkhayds/axolot/internal/mcp/mcptest"
	"github.com/arkhayds/axolot/internal/secret"
	"github.com/arkhayds/axolot/internal/store"
)

const callbackURL = "http://localhost:5090/api/mcp/callback"

// newOAuthTestServer is a Server without a database: enough for sign-ins and probes.
func newOAuthTestServer(t *testing.T) *Server {
	t.Helper()
	box, err := secret.NewBox("test")
	if err != nil {
		t.Fatal(err)
	}
	return &Server{
		cfg:          Config{Box: box, BaseContext: context.Background()},
		logins:       map[string]*mcpLogin{},
		oauthSources: map[int64]*oauthSource{},
	}
}

// signInPage opens the sign-in page as a user who accepts would, and returns where it sends the browser back.
func signInPage(t *testing.T, pageURL string) string {
	t.Helper()
	noFollow := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := noFollow.Get(pageURL)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	return resp.Header.Get("Location")
}

// callback serves the browser's return on the given URL.
func callback(s *Server, back string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	s.mcpCallback(rec, httptest.NewRequest(http.MethodGet, back, nil))
	return rec
}

func TestMcpLoginEndsOnTheCallback(t *testing.T) {
	ts := mcptest.NewOAuthServer(t)
	s := newOAuthTestServer(t)
	kept := make(chan *mcp.Creds, 1)
	pageURL, err := s.beginLogin(ts.URL+"/mcp", callbackURL, func(_ context.Context, creds *mcp.Creds) error {
		kept <- creds
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	rec := callback(s, signInPage(t, pageURL))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "BroadcastChannel") {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body)
	}
	if creds := <-kept; creds.Token.AccessToken != "access-1" {
		t.Fatalf("token=%+v", creds.Token)
	}
	if again := callback(s, signInPage(t, pageURL)); again.Code != http.StatusBadRequest {
		t.Fatalf("second callback status=%d", again.Code)
	}
}

func TestMcpCallbackReportsARefusedSignIn(t *testing.T) {
	ts := mcptest.NewOAuthServer(t)
	s := newOAuthTestServer(t)
	pageURL, err := s.beginLogin(ts.URL+"/mcp", callbackURL, func(context.Context, *mcp.Creds) error {
		t.Error("credentials kept after a refusal")
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	parsed, _ := url.Parse(pageURL)

	rec := callback(s, callbackURL+"?error=access_denied&state="+url.QueryEscape(parsed.Query().Get("state")))
	if rec.Code != http.StatusBadGateway || strings.Contains(rec.Body.String(), "<script>") {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body)
	}
}

func TestMcpCallbackRefusesAnUnknownState(t *testing.T) {
	if rec := callback(newOAuthTestServer(t), callbackURL+"?code=x&state=forged"); rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d", rec.Code)
	}
}

func TestMcpLoginFailsOnAServerWithoutSignIn(t *testing.T) {
	ts := mcpServerWithToken(t)
	s := newOAuthTestServer(t)
	if _, err := s.beginLogin(ts.URL, callbackURL, nil); err == nil {
		t.Fatal("expected an error without OAuth metadata")
	}
}

func TestOAuthSourceSavesTheRotatedToken(t *testing.T) {
	ts := mcptest.NewOAuthServer(t)
	source := newOAuthSource(mcp.Creds{
		Config: oauth2.Config{ClientID: "c1", Endpoint: oauth2.Endpoint{TokenURL: ts.URL + "/token"}},
		Token:  &oauth2.Token{AccessToken: "access-1", RefreshToken: "refresh-1", Expiry: time.Now().Add(-time.Minute)},
	})
	var saved []mcp.Creds
	save := func(_ context.Context, creds mcp.Creds) error {
		saved = append(saved, creds)
		return nil
	}

	for range 2 {
		token, err := source.token(context.Background(), save)
		if err != nil || token != "access-2" {
			t.Fatalf("token=%q err=%v", token, err)
		}
	}
	if len(saved) != 1 || saved[0].Token.RefreshToken != "refresh-2" {
		t.Fatalf("saved=%+v", saved)
	}
}

func TestProbeMcpAsksToSignIn(t *testing.T) {
	ts := mcptest.NewOAuthServer(t)
	s := newOAuthTestServer(t)
	revoked := &oauth2.Token{AccessToken: "old", RefreshToken: "revoked", Expiry: time.Now().Add(-time.Minute)}
	for name, m := range map[string]store.McpServer{
		"never signed in":       {URL: ts.URL + "/mcp"},
		"unreadable sign-in":    {ID: 1, URL: ts.URL + "/mcp", OAuthEnc: []byte("garbage")},
		"refresh token refused": {ID: 2, URL: ts.URL + "/mcp", OAuthEnc: sealCreds(t, s, ts, revoked)},
	} {
		view := s.probeMcp(context.Background(), m)
		if !view.Login || view.Error != "Connexion requise" {
			t.Errorf("%s: view=%+v", name, view)
		}
	}
}

func TestProbeMcpUsesTheSavedSignIn(t *testing.T) {
	ts := mcptest.NewOAuthServer(t)
	s := newOAuthTestServer(t)
	valid := &oauth2.Token{AccessToken: "access-1", RefreshToken: "refresh-1", Expiry: time.Now().Add(time.Hour)}
	view := s.probeMcp(context.Background(), store.McpServer{ID: 1, URL: ts.URL + "/mcp", OAuthEnc: sealCreds(t, s, ts, valid)})
	if view.Error != "" || view.Login || len(view.Tools) != 1 {
		t.Fatalf("view=%+v", view)
	}
}

// sealCreds seals a sign-in against the fake server's token endpoint.
func sealCreds(t *testing.T, s *Server, ts *httptest.Server, token *oauth2.Token) []byte {
	t.Helper()
	plain, err := json.Marshal(mcp.Creds{Config: oauth2.Config{ClientID: "c1", Endpoint: oauth2.Endpoint{TokenURL: ts.URL + "/token"}}, Token: token})
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := s.cfg.Box.Seal(string(plain))
	if err != nil {
		t.Fatal(err)
	}
	return sealed
}

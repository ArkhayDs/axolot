package api

import (
	"context"
	"encoding/json"
	"errors"
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

	back := signInPage(t, pageURL)
	recs := make(chan *httptest.ResponseRecorder, 2)
	for range 2 {
		go func() { recs <- callback(s, back) }()
	}
	for range 2 {
		if rec := <-recs; rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "BroadcastChannel") {
			t.Fatalf("status=%d body=%s", rec.Code, rec.Body)
		}
	}
	if creds := <-kept; creds.Token.AccessToken != "access-1" {
		t.Fatalf("token=%+v", creds.Token)
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
	attempts := 0
	var saved []mcp.Creds
	save := func(ctx context.Context, creds mcp.Creds) error {
		attempts++
		if ctx.Err() != nil || attempts == 1 {
			return errors.New("database down")
		}
		saved = append(saved, creds)
		return nil
	}
	gone, cancel := context.WithCancel(context.Background())
	cancel()

	for _, ctx := range []context.Context{context.Background(), gone, context.Background()} {
		token, err := source.token(ctx, save)
		if err != nil || token != "access-2" {
			t.Fatalf("token=%q err=%v", token, err)
		}
	}
	if attempts != 2 || len(saved) != 1 || saved[0].Token.RefreshToken != "refresh-2" {
		t.Fatalf("attempts=%d saved=%+v", attempts, saved)
	}
}

func TestProbeMcpAsksToSignIn(t *testing.T) {
	ts := mcptest.NewOAuthServer(t)
	s := newOAuthTestServer(t)
	revoked := &oauth2.Token{AccessToken: "old", RefreshToken: "revoked", Expiry: time.Now().Add(-time.Minute)}
	noRefresh := &oauth2.Token{AccessToken: "old", Expiry: time.Now().Add(-time.Minute)}
	for name, m := range map[string]store.McpServer{
		"never signed in":       {URL: ts.URL + "/mcp"},
		"unreadable sign-in":    {ID: 1, URL: ts.URL + "/mcp", OAuthEnc: []byte("garbage")},
		"refresh token refused": {ID: 2, URL: ts.URL + "/mcp", OAuthEnc: sealCreds(t, s, ts.URL+"/token", revoked)},
		"no refresh token":      {ID: 3, URL: ts.URL + "/mcp", OAuthEnc: sealCreds(t, s, ts.URL+"/token", noRefresh)},
	} {
		view := s.probeMcp(context.Background(), m)
		if !view.Login || view.Error != "Connexion requise" {
			t.Errorf("%s: view=%+v", name, view)
		}
	}
}

func TestProbeMcpKeepsTheSignInWhenTheTokenServerIsDown(t *testing.T) {
	ts := mcptest.NewOAuthServer(t)
	busy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	t.Cleanup(busy.Close)
	s := newOAuthTestServer(t)
	expired := &oauth2.Token{AccessToken: "old", RefreshToken: "refresh-1", Expiry: time.Now().Add(-time.Minute)}
	for id, tokenURL := range map[int64]string{1: "http://127.0.0.1:1/token", 2: busy.URL} {
		view := s.probeMcp(context.Background(), store.McpServer{ID: id, URL: ts.URL + "/mcp", OAuthEnc: sealCreds(t, s, tokenURL, expired)})
		if view.Login || view.Error == "" {
			t.Errorf("%s: view=%+v", tokenURL, view)
		}
	}
}

// TestSaveRefreshedSkipsAReplacedSignIn relies on the test server having no database: a save reaching it would panic.
func TestSaveRefreshedSkipsAReplacedSignIn(t *testing.T) {
	ts := mcptest.NewOAuthServer(t)
	s := newOAuthTestServer(t)
	stale := newOAuthSource(mcp.Creds{
		Config: oauth2.Config{ClientID: "c1", Endpoint: oauth2.Endpoint{TokenURL: ts.URL + "/token"}},
		Token:  &oauth2.Token{AccessToken: "access-1", RefreshToken: "refresh-1", Expiry: time.Now().Add(-time.Minute)},
	})
	s.oauthSources[1] = &oauthSource{}

	if token, err := stale.token(context.Background(), s.saveRefreshed(1, stale)); err != nil || token != "access-2" {
		t.Fatalf("token=%q err=%v", token, err)
	}
}

func TestProbeMcpUsesTheSavedSignIn(t *testing.T) {
	ts := mcptest.NewOAuthServer(t)
	s := newOAuthTestServer(t)
	valid := &oauth2.Token{AccessToken: "access-1", RefreshToken: "refresh-1", Expiry: time.Now().Add(time.Hour)}
	view := s.probeMcp(context.Background(), store.McpServer{ID: 1, URL: ts.URL + "/mcp", OAuthEnc: sealCreds(t, s, ts.URL+"/token", valid)})
	if view.Error != "" || view.Login || len(view.Tools) != 1 {
		t.Fatalf("view=%+v", view)
	}
}

// sealCreds seals a sign-in that refreshes at tokenURL.
func sealCreds(t *testing.T, s *Server, tokenURL string, token *oauth2.Token) []byte {
	t.Helper()
	plain, err := json.Marshal(mcp.Creds{Config: oauth2.Config{ClientID: "c1", Endpoint: oauth2.Endpoint{TokenURL: tokenURL}}, Token: token})
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := s.cfg.Box.Seal(string(plain))
	if err != nil {
		t.Fatal(err)
	}
	return sealed
}

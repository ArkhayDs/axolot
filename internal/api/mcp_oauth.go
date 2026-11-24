package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"net/url"
	"sync"
	"time"

	sdkauth "github.com/modelcontextprotocol/go-sdk/auth"
	"golang.org/x/oauth2"

	"github.com/arkhayds/axolot/internal/mcp"
	"github.com/arkhayds/axolot/internal/store"
)

// loginTimeout leaves the user time to sign in on the server's page.
const loginTimeout = 5 * time.Minute

// loginStartTimeout covers discovery and client registration until the sign-in page is known, then the token exchange.
const loginStartTimeout = 15 * time.Second

// mcpLogin is a sign-in waiting for the browser to come back to the callback.
// It stays known for loginTimeout after that return, so a repeated hit of the callback shows the same outcome.
type mcpLogin struct {
	code     chan *sdkauth.AuthorizationResult // nil = sign-in refused on the server's page
	answered sync.Once
	finished chan struct{} // closed once err is set
	err      error
}

// loginMcpServer starts a browser sign-in and answers the page to open in a new tab.
func (s *Server) loginMcpServer(w http.ResponseWriter, r *http.Request) {
	m, ok := s.mcpServerByID(w, r)
	if !ok {
		return
	}
	pageURL, err := s.beginLogin(m.URL, "http://"+r.Host+"/api/mcp/callback", func(ctx context.Context, creds *mcp.Creds) error {
		if err := s.saveOAuth(ctx, m.ID, *creds); err != nil {
			return err
		}
		s.oauthMu.Lock()
		s.oauthSources[m.ID] = newOAuthSource(*creds)
		s.oauthMu.Unlock()
		slog.Info("mcp signed in", "server", m.Name, "client", creds.Config.ClientID)
		return nil
	})
	if err != nil {
		writeError(w, http.StatusBadGateway, "Connexion impossible, détail dans les journaux de l'application")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"url": pageURL})
}

// beginLogin runs mcp.Login in the background and returns once the sign-in page is known; failures are logged here.
// @param redirectURL - this app's callback, as the browser reaches it
// @param finish - keeps the credentials once the browser came back
// @returns sign-in page URL
func (s *Server) beginLogin(endpoint, redirectURL string, finish func(context.Context, *mcp.Creds) error) (string, error) {
	ctx, cancel := context.WithTimeout(s.cfg.BaseContext, loginTimeout)
	login := &mcpLogin{code: make(chan *sdkauth.AuthorizationResult, 1), finished: make(chan struct{})}
	pageURL := make(chan string, 1)
	fetch := func(ctx context.Context, args *sdkauth.AuthorizationArgs) (*sdkauth.AuthorizationResult, error) {
		parsed, err := url.Parse(args.URL)
		if err != nil {
			return nil, fmt.Errorf("parse sign-in url: %w", err)
		}
		state := parsed.Query().Get("state")
		forget := func() {
			s.oauthMu.Lock()
			delete(s.logins, state)
			s.oauthMu.Unlock()
		}
		s.oauthMu.Lock()
		s.logins[state] = login
		s.oauthMu.Unlock()
		pageURL <- args.URL
		select {
		case result := <-login.code:
			time.AfterFunc(loginTimeout, forget)
			if result == nil {
				return nil, errors.New("sign-in refused on the server's page")
			}
			return result, nil
		case <-ctx.Done():
			forget()
			return nil, ctx.Err()
		}
	}
	go func() {
		defer cancel()
		creds, err := mcp.Login(ctx, endpoint, redirectURL, fetch)
		if err == nil {
			err = finish(ctx, creds)
		}
		if err != nil {
			slog.Warn("mcp login failed", "endpoint", endpoint, "err", err)
		}
		login.err = err
		close(login.finished)
	}()
	select {
	case page := <-pageURL:
		return page, nil
	case <-login.finished:
		return "", login.err
	case <-time.After(loginStartTimeout):
		cancel()
		return "", fmt.Errorf("no sign-in page within %v", loginStartTimeout)
	}
}

// mcpCallback is where the server's sign-in page sends the browser back; the first hit hands the code to the waiting login,
// every hit waits for its outcome.
func (s *Server) mcpCallback(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	s.oauthMu.Lock()
	login := s.logins[q.Get("state")]
	s.oauthMu.Unlock()
	if login == nil {
		callbackPage(w, expiredPage)
		return
	}
	login.answered.Do(func() {
		var result *sdkauth.AuthorizationResult
		if q.Get("error") == "" {
			result = &sdkauth.AuthorizationResult{Code: q.Get("code"), State: q.Get("state"), Iss: q.Get("iss")}
		}
		login.code <- result
	})
	select {
	case <-login.finished:
	case <-time.After(loginStartTimeout):
		callbackPage(w, failedPage)
		return
	case <-r.Context().Done():
		return
	}
	if login.err != nil {
		callbackPage(w, failedPage)
		return
	}
	callbackPage(w, signedInPage)
}

// callbackView is one outcome of a sign-in, as the callback page shows it.
type callbackView struct {
	status                    int
	Tone, Icon, Title, Detail string // Tone: good, warning or critical
}

var (
	signedInPage = callbackView{http.StatusOK, "good", "✓", "Connecté", "Cet onglet va se fermer."}
	expiredPage  = callbackView{http.StatusBadRequest, "warning", "!", "Connexion expirée", "La relancer depuis le panneau Serveurs MCP."}
	failedPage   = callbackView{http.StatusBadGateway, "critical", "✕", "Connexion échouée", "Détail dans les journaux de l'application."}
)

// callbackTmpl closes the tab on success, after telling the Chat tab to reload its servers.
// shortcut: colors and font copied from web/src/styles/01-tokens.css (the page is outside the front bundle), update both if the palette changes.
var callbackTmpl = template.Must(template.New("callback").Parse(`<!doctype html>
<html lang="fr">
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>Axolot</title>
<style>
html{color-scheme:dark;background:#060a12}
body{margin:0;min-height:100vh;display:grid;place-items:center;padding:1rem;box-sizing:border-box;color:#ecf4f2;font:400 .9375rem/1.5 -apple-system,BlinkMacSystemFont,'SF Pro Text','Segoe UI',system-ui,sans-serif;-webkit-font-smoothing:antialiased}
main{max-width:26rem;padding:2rem;background:#0f1720;border:1px solid #25313e;border-radius:18px}
small{color:#8c9dac;font-size:.8125rem}
h1{margin:.5rem 0 .25rem;font-size:1.125rem;font-weight:650}
.good{color:#55ad6e}.warning{color:#d9ab4e}.critical{color:#e27a76}
p{margin:0;color:#b5c3cf}
</style>
<main>
<small>Axolot · Serveurs MCP</small>
<h1 class="{{.Tone}}"><span aria-hidden="true">{{.Icon}}</span> {{.Title}}</h1>
<p>{{.Detail}}</p>
</main>
{{if eq .Tone "good"}}<script>new BroadcastChannel("axolot-mcp").postMessage("login");window.close()</script>{{end}}
`))

// callbackPage writes the page of a sign-in outcome.
func callbackPage(w http.ResponseWriter, v callbackView) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(v.status)
	if err := callbackTmpl.Execute(w, v); err != nil {
		slog.Warn("write mcp callback page", "err", err)
	}
}

// saveOAuth seals and stores a server's OAuth credentials.
func (s *Server) saveOAuth(ctx context.Context, id int64, creds mcp.Creds) error {
	plain, err := json.Marshal(creds)
	if err != nil {
		return fmt.Errorf("encode oauth creds: %w", err)
	}
	sealed, err := s.cfg.Box.Seal(string(plain))
	if err != nil {
		return err
	}
	return s.cfg.Store.SetMcpServerOAuth(ctx, id, sealed)
}

// oauthToken gives the bearer of a server signed in from the browser, refreshing it when expired.
func (s *Server) oauthToken(ctx context.Context, m store.McpServer) (string, error) {
	s.oauthMu.Lock()
	source := s.oauthSources[m.ID]
	if source == nil {
		var creds mcp.Creds
		plain, err := s.cfg.Box.Open(m.OAuthEnc)
		if err == nil {
			err = json.Unmarshal([]byte(plain), &creds)
		}
		if err == nil && creds.Token == nil {
			err = errors.New("no token saved")
		}
		if err != nil {
			s.oauthMu.Unlock()
			return "", fmt.Errorf("%w: open oauth creds: %v", errSignInLost, err)
		}
		source = newOAuthSource(creds)
		s.oauthSources[m.ID] = source
	}
	s.oauthMu.Unlock()
	return source.token(ctx, func(ctx context.Context, creds mcp.Creds) error { return s.saveOAuth(ctx, m.ID, creds) })
}

// errSignInLost means only a new browser sign-in can bring the server back; any other token error may pass on its own.
var errSignInLost = errors.New("saved sign-in unusable")

// oauthSource is the one refreshing token source of a server: some servers rotate the refresh token,
// so two refreshes at once would leave one of them with a dead token.
type oauthSource struct {
	mu     sync.Mutex
	creds  mcp.Creds
	tokens oauth2.TokenSource
}

// newOAuthSource refreshes through a client with a timeout: oauth2 keeps this context for every refresh.
func newOAuthSource(creds mcp.Creds) *oauthSource {
	ctx := context.WithValue(context.Background(), oauth2.HTTPClient, &http.Client{Timeout: mcpProbeTimeout})
	return &oauthSource{creds: creds, tokens: creds.Config.TokenSource(ctx, creds.Token)}
}

// token returns a valid access token, saving it after a refresh so a restart keeps the rotated refresh token.
// A failed save is retried on the next call; the save outlives ctx, since the refresh already spent the old refresh token.
func (o *oauthSource) token(ctx context.Context, save func(context.Context, mcp.Creds) error) (string, error) {
	tok, err := o.tokens.Token()
	var refused *oauth2.RetrieveError
	if errors.As(err, &refused) && refused.Response != nil && refused.Response.StatusCode < http.StatusInternalServerError {
		return "", fmt.Errorf("%w: refresh refused: %v", errSignInLost, err)
	}
	if err != nil {
		return "", fmt.Errorf("refresh token: %w", err)
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if tok.AccessToken != o.creds.Token.AccessToken {
		creds := o.creds
		creds.Token = tok
		if err := save(context.WithoutCancel(ctx), creds); err != nil {
			slog.Error("save refreshed mcp token, retried on next use", "err", err)
			return tok.AccessToken, nil
		}
		o.creds = creds
	}
	return tok.AccessToken, nil
}

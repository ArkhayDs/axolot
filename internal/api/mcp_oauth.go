package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
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
type mcpLogin struct {
	code chan *sdkauth.AuthorizationResult // nil = sign-in refused on the server's page
	done chan error
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
		return nil
	})
	if err != nil {
		writeError(w, http.StatusBadGateway, "Ce serveur ne propose pas de connexion, détail dans les journaux de l'application")
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
	login := &mcpLogin{code: make(chan *sdkauth.AuthorizationResult, 1), done: make(chan error, 1)}
	pageURL := make(chan string, 1)
	failed := make(chan error, 1)
	fetch := func(ctx context.Context, args *sdkauth.AuthorizationArgs) (*sdkauth.AuthorizationResult, error) {
		parsed, err := url.Parse(args.URL)
		if err != nil {
			return nil, fmt.Errorf("parse sign-in url: %w", err)
		}
		state := parsed.Query().Get("state")
		s.oauthMu.Lock()
		s.logins[state] = login
		s.oauthMu.Unlock()
		defer func() {
			s.oauthMu.Lock()
			delete(s.logins, state)
			s.oauthMu.Unlock()
		}()
		pageURL <- args.URL
		select {
		case result := <-login.code:
			if result == nil {
				return nil, errors.New("sign-in refused on the server's page")
			}
			return result, nil
		case <-ctx.Done():
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
		login.done <- err
		failed <- err
	}()
	select {
	case page := <-pageURL:
		return page, nil
	case err := <-failed:
		return "", err
	case <-time.After(loginStartTimeout):
		cancel()
		return "", fmt.Errorf("no sign-in page within %v", loginStartTimeout)
	}
}

// mcpCallback is where the server's sign-in page sends the browser back; it hands the code to the waiting login.
func (s *Server) mcpCallback(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	s.oauthMu.Lock()
	login := s.logins[q.Get("state")]
	delete(s.logins, q.Get("state"))
	s.oauthMu.Unlock()
	if login == nil {
		callbackPage(w, http.StatusBadRequest, "Connexion expirée ou déjà terminée. La relancer depuis le panneau Serveurs MCP.", false)
		return
	}
	var result *sdkauth.AuthorizationResult
	if q.Get("error") == "" {
		result = &sdkauth.AuthorizationResult{Code: q.Get("code"), State: q.Get("state"), Iss: q.Get("iss")}
	}
	login.code <- result
	var err error
	select {
	case err = <-login.done:
	case <-time.After(loginStartTimeout):
		err = errors.New("no token in time")
	case <-r.Context().Done():
		return
	}
	if err != nil {
		callbackPage(w, http.StatusBadGateway, "Connexion échouée, détail dans les journaux de l'application.", false)
		return
	}
	callbackPage(w, http.StatusOK, "Connecté. Cet onglet va se fermer.", true)
}

// callbackPage tells the user how the sign-in ended; on success it tells the Chat tab and closes itself.
func callbackPage(w http.ResponseWriter, status int, message string, ok bool) {
	script := ""
	if ok {
		script = `<script>new BroadcastChannel("axolot-mcp").postMessage("login");window.close()</script>`
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	fmt.Fprintf(w, `<!doctype html><meta charset="utf-8"><title>Axolot</title><p>%s</p>%s`, html.EscapeString(message), script)
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
			return "", fmt.Errorf("open oauth creds: %w", err)
		}
		source = newOAuthSource(creds)
		s.oauthSources[m.ID] = source
	}
	s.oauthMu.Unlock()
	return source.token(ctx, func(ctx context.Context, creds mcp.Creds) error { return s.saveOAuth(ctx, m.ID, creds) })
}

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
func (o *oauthSource) token(ctx context.Context, save func(context.Context, mcp.Creds) error) (string, error) {
	tok, err := o.tokens.Token()
	if err != nil {
		return "", fmt.Errorf("refresh token: %w", err)
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if tok.AccessToken != o.creds.Token.AccessToken {
		o.creds.Token = tok
		if err := save(ctx, o.creds); err != nil {
			slog.Error("save refreshed mcp token, sign-in needed again after a restart", "err", err)
		}
	}
	return tok.AccessToken, nil
}

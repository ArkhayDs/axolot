package mcp

import (
	"context"
	"errors"
	"fmt"

	sdkauth "github.com/modelcontextprotocol/go-sdk/auth"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/modelcontextprotocol/go-sdk/oauthex"
	"golang.org/x/oauth2"
)

// Creds are what a browser sign-in leaves to reach a server again: registered client and tokens.
// Refresh with Config.TokenSource(ctx, Token).
type Creds struct {
	Config oauth2.Config `json:"config"`
	Token  *oauth2.Token `json:"token"`
}

// Login signs in to a server asking for OAuth: client registration, then authorization code with PKCE.
// @param redirectURL - loopback callback the browser comes back to
// @param fetch - hands the sign-in URL to the browser, waits for the callback
func Login(ctx context.Context, endpoint, redirectURL string, fetch sdkauth.AuthorizationCodeFetcher) (*Creds, error) {
	var creds *Creds
	handler, err := sdkauth.NewAuthorizationCodeHandler(&sdkauth.AuthorizationCodeHandlerConfig{
		DynamicClientRegistrationConfig: &sdkauth.DynamicClientRegistrationConfig{Metadata: &oauthex.ClientRegistrationMetadata{
			ClientName:   "Axolot",
			RedirectURIs: []string{redirectURL},
			GrantTypes:   []string{"authorization_code", "refresh_token"},
		}},
		AuthorizationCodeFetcher: fetch,
		RequestRefreshToken:      true,
		NewTokenSource: func(ctx context.Context, cfg *oauth2.Config, token *oauth2.Token) (oauth2.TokenSource, error) {
			creds = &Creds{Config: *cfg, Token: token}
			return cfg.TokenSource(ctx, token), nil
		},
	})
	if err != nil {
		return nil, fmt.Errorf("oauth handler: %w", err)
	}
	transport := &sdk.StreamableClientTransport{Endpoint: endpoint, OAuthHandler: handler, DisableStandaloneSSE: true, MaxRetries: -1}
	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		return nil, fmt.Errorf("login %s: %w", endpoint, err)
	}
	session.Close()
	if creds == nil {
		return nil, errors.New("server answered without asking to sign in")
	}
	return creds, nil
}

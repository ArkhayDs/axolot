package mcp

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"testing"

	sdkauth "github.com/modelcontextprotocol/go-sdk/auth"

	"github.com/arkhayds/axolot/internal/mcp/mcptest"
)

// browser follows the sign-in URL as a user who accepts would, and reads the code off the redirect.
func browser(ctx context.Context, args *sdkauth.AuthorizationArgs) (*sdkauth.AuthorizationResult, error) {
	noFollow := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := noFollow.Get(args.URL)
	if err != nil {
		return nil, err
	}
	resp.Body.Close()
	back, err := url.Parse(resp.Header.Get("Location"))
	if err != nil {
		return nil, err
	}
	return &sdkauth.AuthorizationResult{Code: back.Query().Get("code"), State: back.Query().Get("state")}, nil
}

func TestLoginReturnsCredsThatListTheTools(t *testing.T) {
	ts := mcptest.NewOAuthServer(t)
	creds, err := Login(context.Background(), ts.URL+"/mcp", "http://localhost:5090/api/mcp/callback", browser)
	if err != nil {
		t.Fatal(err)
	}
	if creds.Config.ClientID != "c1" || creds.Config.Endpoint.TokenURL != ts.URL+"/token" || creds.Token.RefreshToken != "refresh-1" {
		t.Fatalf("creds=%+v token=%+v", creds.Config, creds.Token)
	}
	names, err := ToolNames(context.Background(), ts.URL+"/mcp", creds.Token.AccessToken)
	if err != nil || len(names) != 1 {
		t.Fatalf("names=%v err=%v", names, err)
	}
}

func TestLoginFailsWhenTheUserNeverComesBack(t *testing.T) {
	ts := mcptest.NewOAuthServer(t)
	gone := errors.New("sign-in abandoned")
	asked := 0
	fetch := func(context.Context, *sdkauth.AuthorizationArgs) (*sdkauth.AuthorizationResult, error) {
		asked++
		return nil, gone
	}
	if _, err := Login(context.Background(), ts.URL+"/mcp", "http://localhost:5090/api/mcp/callback", fetch); !errors.Is(err, gone) || asked != 1 {
		t.Fatalf("err=%v asked=%d", err, asked)
	}
}

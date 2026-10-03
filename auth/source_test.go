package auth_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stainedhead/agent-cli-core/auth"
	"github.com/stainedhead/agent-cli-core/auth/authtest"
	"github.com/stainedhead/agent-cli-core/output"
)

func TestDaemonTokenSourceRequiresProviderAndClient(t *testing.T) {
	f := authtest.New(authtest.Valid)
	if _, err := auth.NewDaemonTokenSource(f, ""); err == nil {
		t.Fatal("empty provider must error: no default provider")
	}
	if _, err := auth.NewDaemonTokenSource(nil, "p"); err == nil {
		t.Fatal("nil client must error")
	}
}

func TestDaemonTokenSourceFetchesNamedProvider(t *testing.T) {
	f := authtest.New(authtest.Valid)
	src, err := auth.NewDaemonTokenSource(f, "alpha")
	if err != nil {
		t.Fatal(err)
	}
	tok, err := src.Token(context.Background())
	if err != nil || tok.IsZero() {
		t.Fatalf("%v %v", tok, err)
	}
	if got := f.Providers(); len(got) != 1 || got[0] != "alpha" {
		t.Fatalf("providers %v", got)
	}
	if _, err := src.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if f.Refreshes() != 1 {
		t.Fatalf("refreshes %d", f.Refreshes())
	}
}

func TestDaemonTokenSourceErrorMapping(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name string
		sc   authtest.Scenario
		is   error
	}{
		{"reauth", authtest.ReauthRequired, auth.ErrReauthRequired},
		{"revoked", authtest.Revoked, auth.ErrRevoked},
	}
	for _, c := range cases {
		src, _ := auth.NewDaemonTokenSource(authtest.New(c.sc), "alpha", auth.WithRemediation("run the enroll command"))
		for _, op := range []func() error{
			func() error { _, e := src.Token(ctx); return e },
			func() error { _, e := src.Refresh(ctx); return e },
		} {
			err := op()
			var ar *auth.ActionRequiredError
			if !errors.Is(err, c.is) || !errors.As(err, &ar) || ar.Provider != "alpha" ||
				!strings.Contains(ar.Hint(), "run the enroll command") || output.ExitOf(err) != output.ExitAuth {
				t.Errorf("%s: %v", c.name, err)
			}
		}
	}
}

func TestDaemonTokenSourceUnreachableNoFallback(t *testing.T) {
	f := authtest.New(authtest.Unreachable, authtest.WithSocket("/tmp/none.sock"))
	src, _ := auth.NewDaemonTokenSource(f, "alpha")
	_, err := src.Token(context.Background())
	var ue *auth.UnreachableError
	if !errors.As(err, &ue) || ue.Socket != "/tmp/none.sock" || output.ExitOf(err) != 3 {
		t.Fatalf("%v", err)
	}
	if !strings.Contains(err.Error(), "/tmp/none.sock") {
		t.Fatalf("socket not named: %v", err)
	}
	if f.Fetches() != 1 {
		t.Fatalf("exactly one attempt, got %d", f.Fetches())
	}
}

type badClient struct {
	tok auth.Token
	err error
}

func (b badClient) Fetch(context.Context, string) (auth.Token, error)   { return b.tok, b.err }
func (b badClient) Refresh(context.Context, string) (auth.Token, error) { return b.tok, b.err }

func TestDaemonTokenSourceOtherFailures(t *testing.T) {
	ctx := context.Background()
	src, _ := auth.NewDaemonTokenSource(badClient{err: errors.New("daemon says no")}, "p")
	var te *auth.TokenError
	if _, err := src.Token(ctx); !errors.As(err, &te) || te.Op != "fetch" {
		t.Fatalf("%v", err)
	}
	if _, err := src.Refresh(ctx); !errors.As(err, &te) || te.Op != "refresh" {
		t.Fatalf("%v", err)
	}
	empty, _ := auth.NewDaemonTokenSource(badClient{}, "p")
	if _, err := empty.Token(ctx); !errors.As(err, &te) {
		t.Fatalf("empty token must fail: %v", err)
	}
	if _, err := empty.Refresh(ctx); !errors.As(err, &te) {
		t.Fatalf("empty refresh token must fail: %v", err)
	}
}

func TestAuthorizerSetsHeaderItself(t *testing.T) {
	f := authtest.New(authtest.Valid)
	src, _ := auth.NewDaemonTokenSource(f, "alpha")
	a := auth.NewAuthorizer(src)
	srv := httptest.NewServer(f.Handler())
	defer srv.Close()
	req, _ := http.NewRequest(http.MethodGet, srv.URL, nil)
	if err := a.Authorize(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(req.Header.Get("Authorization"), "Bearer ") {
		t.Fatalf("header %q", req.Header.Get("Authorization"))
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status %d", resp.StatusCode)
	}
}

func TestAuthorizerErrors(t *testing.T) {
	ctx := context.Background()
	a := auth.NewAuthorizer(nil)
	req, _ := http.NewRequest(http.MethodGet, "http://x.invalid", nil)
	if err := a.Authorize(ctx, req); err == nil {
		t.Fatal("nil source must error")
	}
	if err := a.Refresh(ctx); err == nil {
		t.Fatal("nil source must error")
	}
	f := authtest.New(authtest.ReauthRequired)
	src, _ := auth.NewDaemonTokenSource(f, "alpha")
	a = auth.NewAuthorizer(src)
	if err := a.Authorize(ctx, nil); err == nil {
		t.Fatal("nil request must error")
	}
	if err := a.Authorize(ctx, req); !errors.Is(err, auth.ErrReauthRequired) || req.Header.Get("Authorization") != "" {
		t.Fatalf("%v header %q", err, req.Header.Get("Authorization"))
	}
	if err := a.Refresh(ctx); !errors.Is(err, auth.ErrReauthRequired) {
		t.Fatal(err)
	}
}

type fixedSource struct{}

func (fixedSource) Token(context.Context) (auth.Token, error) { return auth.NewToken("fixed"), nil }

func TestAuthorizerSourceWithoutRefresh(t *testing.T) {
	a := auth.NewAuthorizer(fixedSource{})
	err := a.Refresh(context.Background())
	if !errors.Is(err, auth.ErrRefreshUnsupported) || output.ExitOf(err) != 3 {
		t.Fatalf("%v", err)
	}
	req, _ := http.NewRequest(http.MethodGet, "http://x.invalid", nil)
	if err := a.Authorize(context.Background(), req); err != nil || req.Header.Get("Authorization") != "Bearer fixed" {
		t.Fatalf("%v %q", err, req.Header.Get("Authorization"))
	}
}

func TestAuthorizerEmptyTokenFails(t *testing.T) {
	src, _ := auth.NewDaemonTokenSource(badClient{}, "p")
	_ = src
	a := auth.NewAuthorizer(emptySource{})
	req, _ := http.NewRequest(http.MethodGet, "http://x.invalid", nil)
	if err := a.Authorize(context.Background(), req); err == nil {
		t.Fatal("empty token must fail")
	}
}

type emptySource struct{}

func (emptySource) Token(context.Context) (auth.Token, error) { return auth.Token{}, nil }

// The frozen cross-package contract, restated structurally here so auth never
// imports httpx.
type tokenRefresher interface {
	Authorize(ctx context.Context, req *http.Request) error
	Refresh(ctx context.Context) error
}

var _ tokenRefresher = (*auth.Authorizer)(nil)
var _ auth.TokenSource = (*auth.DaemonTokenSource)(nil)
var _ auth.Refresher = (*auth.DaemonTokenSource)(nil)
var _ auth.DaemonClient = (*authtest.Fake)(nil)

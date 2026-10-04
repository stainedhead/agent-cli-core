package auth_test

import (
	"context"
	"errors"
	"fmt"
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

// catErr is an error that carries its own category and hint, as the adapter's
// typed errors do.
type catErr struct {
	msg, hint string
	cat       output.Category
	cause     error
}

func (e *catErr) Error() string             { return e.msg }
func (e *catErr) Hint() string              { return e.hint }
func (e *catErr) Category() output.Category { return e.cat }
func (e *catErr) Unwrap() error             { return e.cause }

func TestDaemonTokenSourcePreservesCategorizedErrors(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name string
		err  error
		exit output.ExitCode
	}{
		{"rate limited", &catErr{msg: "daemon busy", hint: "retry in 30s", cat: output.CategoryRateLimited}, output.ExitRateLimited},
		{"auth", &catErr{msg: "not configured", hint: "configure it", cat: output.CategoryAuth}, output.ExitAuth},
		{"wrapped", fmt.Errorf("adapter: %w", &catErr{msg: "busy", hint: "retry in 5s", cat: output.CategoryRateLimited}), output.ExitRateLimited},
	}
	for _, c := range cases {
		src, _ := auth.NewDaemonTokenSource(badClient{err: c.err}, "p")
		for op, f := range map[string]func() error{
			"fetch":   func() error { _, e := src.Token(ctx); return e },
			"refresh": func() error { _, e := src.Refresh(ctx); return e },
		} {
			err := f()
			var want *catErr
			if !errors.As(err, &want) || !errors.Is(err, c.err) {
				t.Errorf("%s/%s: cause chain lost: %v", c.name, op, err)
			}
			var te *auth.TokenError
			if errors.As(err, &te) {
				t.Errorf("%s/%s: must not be wrapped in TokenError", c.name, op)
			}
			if got := output.ExitOf(err); got != c.exit {
				t.Errorf("%s/%s: exit %d, want %d", c.name, op, got, c.exit)
			}
			if env := output.FromError(err); env.Error.Hint != want.hint {
				t.Errorf("%s/%s: hint %q, want %q", c.name, op, env.Error.Hint, want.hint)
			}
		}
	}
}

func TestDaemonTokenSourceUncategorizedStillWrapped(t *testing.T) {
	src, _ := auth.NewDaemonTokenSource(badClient{err: errors.New("daemon says no")}, "p")
	_, err := src.Token(context.Background())
	var te *auth.TokenError
	if !errors.As(err, &te) || output.ExitOf(err) != output.ExitAuth {
		t.Fatalf("%v", err)
	}
}

func TestDaemonTokenSourceCategorizedErrorDoesNotLeak(t *testing.T) {
	const secret = "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dBjftJeZ4CVPmB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	e := &catErr{msg: "failed with Authorization: Bearer " + secret, hint: "token " + secret, cat: output.CategoryRateLimited, cause: errors.New("raw " + secret)}
	src, _ := auth.NewDaemonTokenSource(badClient{err: e}, "p")
	_, err := src.Token(context.Background())
	env := output.FromError(err)
	for _, s := range []string{err.Error(), env.Error.Message, env.Error.Hint, fmt.Sprintf("%v %+v", err, err)} {
		if strings.Contains(s, secret) {
			t.Fatalf("secret leaked: %s", s)
		}
	}
	if output.ExitOf(err) != output.ExitRateLimited {
		t.Fatalf("exit %d", output.ExitOf(err))
	}
}

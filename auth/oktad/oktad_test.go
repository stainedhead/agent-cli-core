package oktad_test

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stainedhead/agent-cli-core/auth"
	"github.com/stainedhead/agent-cli-core/auth/oktad"
	"github.com/stainedhead/agent-cli-core/output"
	"github.com/stainedhead/agent-okta-d/pkg/client"
	"github.com/stainedhead/agent-okta-d/pkg/client/clienttest"
)

const secretToken = "SENTINEL-TOKEN-8f3a9c1d"

func newFake(t *testing.T) *clienttest.Server {
	t.Helper()
	srv := clienttest.New(t)
	t0 := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	srv.SetCredential("graph", clienttest.Credential{TokenType: "Bearer", AccessToken: secretToken, IssuedAt: t0, ExpiresAt: t0.Add(time.Hour), Audience: "aud"})
	return srv
}

func newClient(t *testing.T, srv *clienttest.Server) *oktad.Client {
	t.Helper()
	c := oktad.New(oktad.WithSocketPath(srv.SocketPath()), oktad.WithTimeout(5*time.Second))
	t.Cleanup(c.Close)
	return c
}

// authorizeHeader shows the token reaches the Authorization header and only
// there, through the public auth path.
func TestFetchAndRefreshYieldUsableRedactedToken(t *testing.T) {
	srv := newFake(t)
	srv.SetRefreshed("graph", clienttest.Credential{TokenType: "Bearer", AccessToken: secretToken + "-2", ExpiresAt: time.Now().Add(time.Hour)})
	c := newClient(t, srv)

	var _ auth.DaemonClient = c
	src, err := auth.NewDaemonTokenSource(c, "graph")
	if err != nil {
		t.Fatal(err)
	}
	tok, err := c.Fetch(context.Background(), "graph")
	if err != nil {
		t.Fatal(err)
	}
	if tok.IsZero() {
		t.Fatal("zero token")
	}
	for _, s := range []string{
		fmt.Sprint(tok), fmt.Sprintf("%v %+v %#v %s %q", tok, tok, tok, tok, tok),
	} {
		if strings.Contains(s, "SENTINEL") {
			t.Fatalf("token leaked: %s", s)
		}
	}
	if _, err := src.Token(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := src.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := len(srv.Requests()); got != 3 {
		t.Fatalf("requests = %d, want 3", got)
	}
}

func TestMapping(t *testing.T) {
	type check func(t *testing.T, err error)
	cat := func(want output.Category) check {
		return func(t *testing.T, err error) {
			t.Helper()
			if got := output.CategoryOf(err); got != want {
				t.Errorf("category = %s, want %s (err %v)", got, want, err)
			}
		}
	}
	is := func(target error) check {
		return func(t *testing.T, err error) {
			t.Helper()
			if !errors.Is(err, target) {
				t.Errorf("errors.Is(%v, %v) = false", err, target)
			}
		}
	}
	rows := []struct {
		name   string
		e      clienttest.Error
		exit   output.ExitCode
		checks []check
	}{
		{"reauth", clienttest.Error{Code: clienttest.CodeReauthRequired}, 3, []check{is(auth.ErrReauthRequired), is(client.ErrReauthRequired), cat(output.CategoryAuth)}},
		{"revoked", clienttest.Error{Code: clienttest.CodeRevoked}, 3, []check{is(auth.ErrRevoked), is(client.ErrRevoked), cat(output.CategoryAuth)}},
		{"degraded with hint", clienttest.Error{Code: clienttest.CodeDegraded, RetryAfter: 30 * time.Second}, 8, []check{is(client.ErrDegraded), cat(output.CategoryRateLimited)}},
		{"degraded no hint", clienttest.Error{Code: clienttest.CodeDegraded}, 8, []check{cat(output.CategoryRateLimited)}},
		{"other with retry-after", clienttest.Error{Code: clienttest.CodeInternal, RetryAfter: 7 * time.Second}, 8, []check{cat(output.CategoryRateLimited)}},
		{"not configured", clienttest.Error{Code: clienttest.CodeNotConfigured}, 3, []check{is(client.ErrNotConfigured), cat(output.CategoryAuth)}},
		{"unauthorized", clienttest.Error{Code: clienttest.CodeUnauthorized}, 3, []check{is(client.ErrUnauthorized), cat(output.CategoryAuth)}},
		{"internal", clienttest.Error{Code: clienttest.CodeInternal}, 1, []check{cat(output.CategoryGeneral)}},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			srv := newFake(t)
			srv.SetProviderError("graph", row.e)
			c := newClient(t, srv)
			for _, op := range []struct {
				n string
				f func(context.Context, string) (auth.Token, error)
			}{{"fetch", c.Fetch}, {"refresh", c.Refresh}} {
				_, err := op.f(context.Background(), "graph")
				if err == nil {
					t.Fatalf("%s: nil error", op.n)
				}
				if got := output.ExitFor(output.CategoryOf(err)); got != row.exit {
					t.Errorf("%s: exit = %d, want %d (%v)", op.n, got, row.exit, err)
				}
				for _, ck := range row.checks {
					ck(t, err)
				}
				env := output.FromError(err)
				if env.Error == nil || strings.Contains(env.Error.Message+env.Error.Hint, secretToken) {
					t.Errorf("bad envelope %+v", env.Error)
				}
			}
		})
	}
}

func TestTransientCarriesRetryAfter(t *testing.T) {
	srv := newFake(t)
	srv.SetProviderError("graph", clienttest.Error{Code: clienttest.CodeDegraded, RetryAfter: 30 * time.Second})
	c := newClient(t, srv)
	_, err := c.Fetch(context.Background(), "graph")

	var te *oktad.TransientError
	if !errors.As(err, &te) {
		t.Fatalf("not a TransientError: %v", err)
	}
	if te.RetryAfter() != 30*time.Second {
		t.Errorf("RetryAfter = %v", te.RetryAfter())
	}
	if d, ok := client.RetryAfter(err); !ok || d != 30*time.Second {
		t.Errorf("client.RetryAfter through the chain = %v, %v", d, ok)
	}
	if !strings.Contains(te.Hint(), "30") {
		t.Errorf("hint does not name the wait: %q", te.Hint())
	}
	env := output.FromError(fmt.Errorf("wrapped: %w", err))
	if env.Error.Code != output.CategoryRateLimited || !strings.Contains(env.Error.Hint, "30") {
		t.Errorf("envelope %+v", env.Error)
	}
	if env.ExitCode() != output.ExitRateLimited {
		t.Errorf("exit = %d", env.ExitCode())
	}
	if !errors.Is(err, client.ErrDegraded) {
		t.Error("cause lost")
	}
}

func TestTransientWithoutHint(t *testing.T) {
	te := &oktad.TransientError{Err: errors.New("boom")}
	if te.RetryAfter() != 0 || te.Hint() == "" || te.Error() == "" || !errors.Is(te, te.Err) {
		t.Errorf("%v / %q", te, te.Hint())
	}
	if strings.Contains(te.Hint(), "0s") {
		t.Errorf("hint names a zero wait: %q", te.Hint())
	}
	if te.Category() != output.CategoryRateLimited {
		t.Error("category")
	}
	if (&oktad.TransientError{}).Error() == "" {
		t.Error("empty error text")
	}
}

func TestAccessErrorHints(t *testing.T) {
	srv := newFake(t)
	c := newClient(t, srv)
	_, err := c.Fetch(context.Background(), "nope")
	var ae *oktad.AccessError
	if !errors.As(err, &ae) {
		t.Fatalf("got %T %v", err, err)
	}
	if ae.Provider != "nope" || !strings.Contains(ae.Hint(), "configured") || ae.Category() != output.CategoryAuth {
		t.Errorf("%+v / %q", ae, ae.Hint())
	}
	srv.SetProviderError("graph", clienttest.Error{Code: clienttest.CodeUnauthorized})
	_, err = c.Fetch(context.Background(), "graph")
	if !errors.As(err, &ae) || !strings.Contains(ae.Hint(), "authorize") || !strings.Contains(ae.Error(), `"graph"`) {
		t.Fatalf("got %v", err)
	}
	if ae.Unwrap() == nil {
		t.Error("no cause")
	}
}

func TestUnreachable(t *testing.T) {
	sock := clienttest.DeadSocketPath(t)
	c := oktad.New(oktad.WithSocketPath(sock))
	defer c.Close()
	_, err := c.Fetch(context.Background(), "graph")
	var ue *auth.UnreachableError
	if !errors.As(err, &ue) {
		t.Fatalf("got %T %v", err, err)
	}
	if ue.Socket != sock || output.ExitFor(output.CategoryOf(err)) != output.ExitAuth {
		t.Errorf("socket %q exit %v", ue.Socket, output.CategoryOf(err))
	}
	if !errors.Is(err, client.ErrDaemonUnavailable) {
		t.Error("cause lost")
	}
	// Through a TokenSource the unreachable error is preserved.
	src, _ := auth.NewDaemonTokenSource(c, "graph")
	_, err = src.Token(context.Background())
	if !errors.As(err, &ue) {
		t.Fatalf("source: %T", err)
	}
}

func TestCancellationIsNotUnreachable(t *testing.T) {
	srv := newFake(t)
	c := newClient(t, srv)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, f := range []func(context.Context, string) (auth.Token, error){c.Fetch, c.Refresh} {
		_, err := f(ctx, "graph")
		var ue *auth.UnreachableError
		if errors.As(err, &ue) || !errors.Is(err, context.Canceled) {
			t.Fatalf("got %v", err)
		}
		if output.CategoryOf(err) != output.CategoryGeneral {
			t.Errorf("category %s", output.CategoryOf(err))
		}
	}
	// Dead socket and cancelled context: still cancellation, not unreachable.
	dead := oktad.New(oktad.WithSocketPath(clienttest.DeadSocketPath(t)))
	_, err := dead.Fetch(ctx, "graph")
	if !errors.Is(err, context.Canceled) || output.CategoryOf(err) != output.CategoryGeneral {
		t.Fatalf("got %v", err)
	}
	dctx, dcancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer dcancel()
	_, err = c.Fetch(dctx, "graph")
	if !errors.Is(err, context.DeadlineExceeded) || output.CategoryOf(err) != output.CategoryGeneral {
		t.Fatalf("deadline: %v", err)
	}
}

func TestInvalidResponseIsGeneral(t *testing.T) {
	srv := newFake(t)
	srv.SetRaw(200, "not json")
	c := newClient(t, srv)
	_, err := c.Fetch(context.Background(), "graph")
	if err == nil || output.CategoryOf(err) != output.CategoryGeneral || !errors.Is(err, client.ErrInvalidResponse) {
		t.Fatalf("got %v (%s)", err, output.CategoryOf(err))
	}
	srv.SetRaw(200, `{"token_type":"Bearer","access_token":""}`)
	_, err = c.Fetch(context.Background(), "graph")
	if err == nil || output.CategoryOf(err) != output.CategoryGeneral {
		t.Fatalf("empty token: %v", err)
	}
}

func TestEmptyProviderAndZeroValue(t *testing.T) {
	srv := newFake(t)
	c := newClient(t, srv)
	if _, err := c.Fetch(context.Background(), ""); err == nil || output.CategoryOf(err) != output.CategoryGeneral {
		t.Fatalf("empty provider: %v", err)
	}
	if _, err := c.Refresh(context.Background(), ""); err == nil {
		t.Fatal("empty provider refresh accepted")
	}
	if n := len(srv.Requests()); n != 0 {
		t.Fatalf("empty provider reached the socket (%d requests)", n)
	}
	var z oktad.Client
	for _, f := range []func(context.Context, string) (auth.Token, error){z.Fetch, z.Refresh} {
		if _, err := f(context.Background(), "graph"); err == nil || output.CategoryOf(err) != output.CategoryGeneral {
			t.Fatalf("zero value: %v", err)
		}
	}
	z.Close() // must not panic
	c.Close()
	c.Close() // idempotent
	if _, err := c.Fetch(context.Background(), "graph"); err != nil {
		t.Fatalf("client unusable after Close: %v", err)
	}
}

func TestOptionsAndEnvDefault(t *testing.T) {
	c := oktad.New(oktad.WithSocketPath("/x/y.sock"), oktad.WithTimeout(time.Second))
	if c.SocketPath() != "/x/y.sock" {
		t.Errorf("socket %q", c.SocketPath())
	}
	t.Setenv(client.EnvSocket, "/env/z.sock")
	if got := oktad.New().SocketPath(); got != "/env/z.sock" {
		t.Errorf("env default = %q", got)
	}
	if got := oktad.New(oktad.WithSocketPath("/opt.sock")).SocketPath(); got != "/opt.sock" {
		t.Errorf("option must beat env: %q", got)
	}
	t.Setenv(client.EnvSocket, "")
	if got := oktad.New().SocketPath(); got != client.DefaultSocketPath() {
		t.Errorf("default = %q", got)
	}
	// Non-positive timeout and empty socket are ignored (defaults kept).
	_ = oktad.New(oktad.WithTimeout(0), oktad.WithSocketPath(""))
	if got := oktad.New(oktad.WithSocketPath("")).SocketPath(); got != client.DefaultSocketPath() {
		t.Errorf("empty option = %q", got)
	}
}

func TestNoTokenInErrorsOrLogs(t *testing.T) {
	srv := newFake(t)
	srv.SetProviderError("graph", clienttest.Error{Code: clienttest.CodeDegraded, RetryAfter: time.Second})
	c := newClient(t, srv)
	_, err := c.Fetch(context.Background(), "graph")
	var sb strings.Builder
	l := slog.New(slog.NewTextHandler(&sb, nil))
	l.Info("x", "err", err)
	out := fmt.Sprintf("%v %+v %#v %s", err, err, err, sb.String())
	if strings.Contains(out, secretToken) {
		t.Fatal("token in error output")
	}
}

func TestConcurrent(t *testing.T) {
	srv := newFake(t)
	c := newClient(t, srv)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 5; j++ {
				if _, err := c.Fetch(context.Background(), "graph"); err != nil {
					t.Error(err)
					return
				}
				if _, err := c.Refresh(context.Background(), "graph"); err != nil {
					t.Error(err)
					return
				}
			}
		}()
	}
	wg.Wait()
}

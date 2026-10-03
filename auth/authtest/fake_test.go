package authtest_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stainedhead/agent-cli-core/auth"
	"github.com/stainedhead/agent-cli-core/auth/authtest"
)

// roundTrip performs the PRD's 401 protocol by hand: authorize, send, and on
// 401 refresh once and resend once. It returns the final status.
func roundTrip(t *testing.T, f *authtest.Fake) (int, error) {
	t.Helper()
	srv := httptest.NewServer(f.Handler())
	defer srv.Close()
	src, err := auth.NewDaemonTokenSource(f, "alpha")
	if err != nil {
		t.Fatal(err)
	}
	a := auth.NewAuthorizer(src)
	ctx := context.Background()
	status := 0
	for attempt := 0; attempt < 2; attempt++ {
		req, _ := http.NewRequest(http.MethodGet, srv.URL, nil)
		if err := a.Authorize(ctx, req); err != nil {
			return 0, err
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		status = resp.StatusCode
		if status != http.StatusUnauthorized {
			return status, nil
		}
		if attempt == 0 {
			if err := a.Refresh(ctx); err != nil {
				return status, err
			}
		}
	}
	return status, nil
}

func TestScenarios(t *testing.T) {
	cases := []struct {
		sc      authtest.Scenario
		status  int
		wantErr error
		reqs    int
	}{
		{authtest.Valid, 200, nil, 1},
		{authtest.ExpiredNeedsRefresh, 200, nil, 2},
		{authtest.ReauthRequired, 0, auth.ErrReauthRequired, 0},
		{authtest.Revoked, 0, auth.ErrRevoked, 0},
		{authtest.Unreachable, 0, nil, 0},
		{authtest.UnauthorizedThenSuccess, 200, nil, 2},
		{authtest.UnauthorizedTwice, 401, nil, 2},
	}
	for _, c := range cases {
		f := authtest.New(c.sc)
		status, err := roundTrip(t, f)
		if c.sc == authtest.Unreachable {
			var ue *auth.UnreachableError
			if !errors.As(err, &ue) {
				t.Errorf("%v: %v", c.sc, err)
			}
			continue
		}
		if !errors.Is(err, c.wantErr) || (c.wantErr == nil && err != nil) || status != c.status || f.Requests() != c.reqs {
			t.Errorf("%v: status %d err %v reqs %d", c.sc, status, err, f.Requests())
		}
	}
}

func TestScenarioString(t *testing.T) {
	seen := map[string]bool{}
	for sc := authtest.Valid; sc <= authtest.UnauthorizedTwice; sc++ {
		s := sc.String()
		if s == "" || seen[s] {
			t.Fatalf("bad name %q", s)
		}
		seen[s] = true
	}
	if authtest.Scenario(99).String() == "" {
		t.Fatal("unknown scenario needs a name")
	}
}

func TestFakeCountersAndSocket(t *testing.T) {
	f := authtest.New(authtest.Unreachable)
	_, err := f.Fetch(context.Background(), "p")
	var ue *auth.UnreachableError
	if !errors.As(err, &ue) || ue.Socket == "" {
		t.Fatalf("default socket must be named: %v", err)
	}
	if _, err := f.Refresh(context.Background(), "p"); err == nil {
		t.Fatal("refresh must fail")
	}
	if f.Fetches() != 1 || f.Refreshes() != 1 {
		t.Fatal("counters")
	}
}

func TestHandlerRejectsMissingAndWrongToken(t *testing.T) {
	f := authtest.New(authtest.Valid)
	for _, h := range []string{"", "Bearer nope", "Basic abc"} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/", nil)
		if h != "" {
			req.Header.Set("Authorization", h)
		}
		f.Handler().ServeHTTP(rec, req)
		if rec.Code != 401 {
			t.Errorf("%q -> %d", h, rec.Code)
		}
	}
}

package httpx_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	"github.com/stainedhead/agent-cli-core/httpx"
	"github.com/stainedhead/agent-cli-core/internal/clock"
	"github.com/stainedhead/agent-cli-core/output"
)

type staticAuth struct{ refreshed bool }

func (s *staticAuth) Authorize(_ context.Context, r *http.Request) error {
	tok := "old"
	if s.refreshed {
		tok = "new"
	}
	r.Header.Set("Authorization", "Bearer "+tok)
	return nil
}

func (s *staticAuth) Refresh(context.Context) error { s.refreshed = true; return nil }

func fastClock() *clock.Fake {
	c := clock.NewFake(time.Unix(0, 0))
	c.SetAutoAdvance(true)
	return c
}

func Example() {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		if calls == 1 {
			w.Header().Set("Retry-After", "2")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		_, _ = fmt.Fprint(w, "ok")
	}))
	defer srv.Close()

	c := httpx.NewClient(httpx.Config{Clock: fastClock()})
	resp, err := c.Get(srv.URL)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer func() { _ = resp.Body.Close() }()
	fmt.Println(resp.StatusCode, calls)
	// Output: 200 2
}

func ExampleNewClient_rateLimited() {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	c := httpx.NewClient(httpx.Config{Clock: fastClock(), MaxRetries: 1})
	_, err := c.Get(srv.URL)
	var rl *httpx.RateLimitedError
	fmt.Println(errors.As(err, &rl), output.ExitOf(err))
	// Output: true 8
}

func ExampleMarkSafeToRetry() {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		if calls == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
	}))
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodPost, srv.URL, strings.NewReader("{}"))
	req.Header.Set("Idempotency-Key", "abc-123")
	req = httpx.MarkSafeToRetry(req) // the caller vouches the POST is safe to repeat
	resp, err := httpx.NewClient(httpx.Config{Clock: fastClock()}).Do(req)
	if err != nil {
		fmt.Println(err)
		return
	}
	_ = resp.Body.Close()
	fmt.Println(resp.StatusCode, calls)
	// Output: 200 2
}

func ExampleTokenRefresher() {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer new" {
			w.WriteHeader(http.StatusUnauthorized)
		}
	}))
	defer srv.Close()

	auth := &staticAuth{}
	c := httpx.NewClient(httpx.Config{Clock: fastClock(), Refresher: auth})
	resp, err := c.Get(srv.URL)
	if err != nil {
		fmt.Println(err)
		return
	}
	_ = resp.Body.Close()
	fmt.Println(resp.StatusCode, auth.refreshed)
	// Output: 200 true
}

func ExampleConfig_forbidden() {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Error-Code", "E403-7")
		w.WriteHeader(http.StatusForbidden)
		_, _ = fmt.Fprint(w, "sensitive detail that must never be surfaced")
	}))
	defer srv.Close()

	c := httpx.NewClient(httpx.Config{
		VendorCode: func(h http.Header) string { return h.Get("X-Error-Code") },
	})
	_, err := c.Get(srv.URL)
	var fe *httpx.ForbiddenError
	if errors.As(err, &fe) {
		fmt.Println(fe.VendorCode, output.ExitOf(err))
	}
	// Output: E403-7 4
}

func ExampleConfig_trace() {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer srv.Close()

	var buf bytes.Buffer
	c := httpx.NewClient(httpx.Config{Trace: &buf, Refresher: &staticAuth{}})
	resp, err := c.Get(srv.URL + "/items?token=abcdef123456")
	if err != nil {
		fmt.Println(err)
		return
	}
	_ = resp.Body.Close()
	fmt.Println(strings.Contains(buf.String(), "Bearer old"), strings.Contains(buf.String(), "abcdef123456"))
	// Output: false false
}

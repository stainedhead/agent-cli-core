package httpx

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stainedhead/agent-cli-core/internal/clock"
	"github.com/stainedhead/agent-cli-core/output"
)

type fakeRefresher struct {
	authorized atomic.Int32
	refreshed  atomic.Int32
	token      atomic.Value
	refreshErr error
	authErr    error
}

func (f *fakeRefresher) Authorize(_ context.Context, r *http.Request) error {
	if f.authErr != nil {
		return f.authErr
	}
	f.authorized.Add(1)
	t, _ := f.token.Load().(string)
	if t == "" {
		t = "tok0"
	}
	r.Header.Set("Authorization", "Bearer "+t)
	return nil
}

func (f *fakeRefresher) Refresh(context.Context) error {
	f.refreshed.Add(1)
	if f.refreshErr != nil {
		return f.refreshErr
	}
	f.token.Store("tok1")
	return nil
}

func newFake() *clock.Fake {
	c := clock.NewFake(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	c.SetAutoAdvance(true)
	return c
}

// script serves the given statuses in order, then repeats the last one.
func script(t *testing.T, statuses []int, hdr map[string]string) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var n atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		i := int(n.Add(1)) - 1
		if i >= len(statuses) {
			i = len(statuses) - 1
		}
		for k, v := range hdr {
			w.Header().Set(k, v)
		}
		w.WriteHeader(statuses[i])
		_, _ = w.Write([]byte("secret-body-content"))
	}))
	t.Cleanup(srv.Close)
	return srv, &n
}

func client(cfg Config) *http.Client { return NewClient(cfg) }

func get(t *testing.T, c *http.Client, url string) (*http.Response, error) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, url, nil)
	return c.Do(req)
}

func TestSuccessNoRetry(t *testing.T) {
	srv, n := script(t, []int{200}, nil)
	fc := newFake()
	resp, err := get(t, client(Config{Clock: fc, Rand: func() float64 { return 0.5 }}), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != 200 || n.Load() != 1 || len(fc.Slept()) != 0 {
		t.Fatalf("status=%d calls=%d slept=%v", resp.StatusCode, n.Load(), fc.Slept())
	}
}

func TestRetryTransientThenSuccess(t *testing.T) {
	srv, n := script(t, []int{502, 504, 200}, nil)
	fc := newFake()
	cfg := Config{Clock: fc, BaseDelay: time.Second, Jitter: 0.5, Rand: func() float64 { return 1 }}
	resp, err := get(t, client(cfg), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if n.Load() != 3 {
		t.Fatalf("calls=%d", n.Load())
	}
	// Rand=1 -> +50% jitter: 1s*1.5, 2s*1.5.
	s := fc.Slept()
	if len(s) != 2 || s[0] != 1500*time.Millisecond || s[1] != 3*time.Second {
		t.Fatalf("slept=%v", s)
	}
}

func TestJitterBounds(t *testing.T) {
	for _, r := range []float64{0, 0.5, 0.999} {
		srv, _ := script(t, []int{502, 200}, nil)
		fc := newFake()
		rr := r
		resp, err := get(t, client(Config{Clock: fc, BaseDelay: time.Second, Jitter: 0.2, Rand: func() float64 { return rr }}), srv.URL)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		d := fc.Slept()[0]
		if d < 800*time.Millisecond || d > 1200*time.Millisecond {
			t.Fatalf("rand %v: delay %v out of bounds", r, d)
		}
	}
}

func TestRetryAfterSecondsHonoredAndCapped(t *testing.T) {
	srv, n := script(t, []int{429, 503, 200}, map[string]string{"Retry-After": "5"})
	fc := newFake()
	resp, err := get(t, client(Config{Clock: fc, MaxWait: 3 * time.Second}), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	s := fc.Slept()
	if n.Load() != 3 || len(s) != 2 || s[0] != 3*time.Second || s[1] != 3*time.Second {
		t.Fatalf("calls=%d slept=%v", n.Load(), s)
	}
}

func TestRetryAfterBelowCeilingExact(t *testing.T) {
	srv, _ := script(t, []int{429, 200}, map[string]string{"Retry-After": "2"})
	fc := newFake()
	resp, err := get(t, client(Config{Clock: fc, MaxWait: time.Minute}), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if s := fc.Slept(); len(s) != 1 || s[0] != 2*time.Second {
		t.Fatalf("slept=%v", s)
	}
}

func TestRetryAfterHTTPDate(t *testing.T) {
	fc := newFake()
	when := fc.Now().Add(7 * time.Second).UTC().Format(http.TimeFormat)
	srv, _ := script(t, []int{503, 200}, map[string]string{"Retry-After": when})
	resp, err := get(t, client(Config{Clock: fc, MaxWait: time.Minute}), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if s := fc.Slept(); len(s) != 1 || s[0] != 7*time.Second {
		t.Fatalf("slept=%v", s)
	}
}

func TestParseRetryAfter(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	cases := map[string]struct {
		d  time.Duration
		ok bool
	}{
		"":    {0, false},
		"abc": {0, false},
		"-5":  {0, true},
		"0":   {0, true},
		"12":  {12 * time.Second, true},
		now.Add(-time.Hour).Format(http.TimeFormat): {0, true},
	}
	for in, want := range cases {
		d, ok := parseRetryAfter(in, now)
		if d != want.d || ok != want.ok {
			t.Errorf("%q: got %v,%v want %v,%v", in, d, ok, want.d, want.ok)
		}
	}
}

func TestRateLimitedExhausted(t *testing.T) {
	srv, n := script(t, []int{429}, map[string]string{"Retry-After": "1"})
	fc := newFake()
	_, err := get(t, client(Config{Clock: fc, MaxRetries: 2}), srv.URL)
	var rl *RateLimitedError
	if !errors.As(err, &rl) {
		t.Fatalf("err=%v", err)
	}
	if n.Load() != 3 || rl.Attempts != 3 || rl.Status != 429 {
		t.Fatalf("calls=%d %+v", n.Load(), rl)
	}
	if output.ExitOf(err) != output.ExitRateLimited {
		t.Fatalf("exit=%d", output.ExitOf(err))
	}
	if strings.Contains(err.Error(), "secret-body-content") {
		t.Fatal("body leaked")
	}
	if rl.Hint() == "" {
		t.Fatal("no hint")
	}
}

func TestNoRetriesConfigured(t *testing.T) {
	srv, n := script(t, []int{503}, nil)
	_, err := get(t, client(Config{Clock: newFake(), MaxRetries: -1}), srv.URL)
	var rl *RateLimitedError
	if !errors.As(err, &rl) || n.Load() != 1 {
		t.Fatalf("err=%v calls=%d", err, n.Load())
	}
}

func TestNonIdempotentNotRetried(t *testing.T) {
	srv, n := script(t, []int{503, 200}, nil)
	req, _ := http.NewRequest(http.MethodPost, srv.URL, strings.NewReader("x"))
	_, err := client(Config{Clock: newFake()}).Do(req)
	var rl *RateLimitedError
	if !errors.As(err, &rl) || n.Load() != 1 || rl.Attempts != 1 {
		t.Fatalf("err=%v calls=%d", err, n.Load())
	}
}

func TestMarkedSafePostRetriedWithBodyReplay(t *testing.T) {
	var bodies []string
	var n atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		bodies = append(bodies, string(b))
		if n.Add(1) == 1 {
			w.WriteHeader(503)
			return
		}
		w.WriteHeader(200)
	}))
	defer srv.Close()
	req, _ := http.NewRequest(http.MethodPost, srv.URL, strings.NewReader("payload"))
	req = MarkSafeToRetry(req)
	if !IsMarkedSafe(req) {
		t.Fatal("not marked")
	}
	resp, err := client(Config{Clock: newFake()}).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if len(bodies) != 2 || bodies[0] != "payload" || bodies[1] != "payload" {
		t.Fatalf("bodies=%v", bodies)
	}
}

func TestUnreplayableBodyNotRetried(t *testing.T) {
	srv, n := script(t, []int{503, 200}, nil)
	req, _ := http.NewRequest(http.MethodPut, srv.URL, io.NopCloser(strings.NewReader("x")))
	req.GetBody = nil
	_, err := client(Config{Clock: newFake()}).Do(req)
	if err == nil || n.Load() != 1 {
		t.Fatalf("err=%v calls=%d", err, n.Load())
	}
}

func TestGetBodyErrorStopsRetry(t *testing.T) {
	srv, n := script(t, []int{503, 200}, nil)
	req, _ := http.NewRequest(http.MethodPut, srv.URL, strings.NewReader("x"))
	req.GetBody = func() (io.ReadCloser, error) { return nil, errors.New("boom") }
	_, err := client(Config{Clock: newFake()}).Do(req)
	if err == nil || n.Load() != 1 {
		t.Fatalf("err=%v calls=%d", err, n.Load())
	}
}

func TestOtherStatusesPassThrough(t *testing.T) {
	for _, code := range []int{404, 409, 500, 201} {
		srv, n := script(t, []int{code}, nil)
		resp, err := get(t, client(Config{Clock: newFake()}), srv.URL)
		if err != nil {
			t.Fatalf("%d: %v", code, err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != code || n.Load() != 1 {
			t.Fatalf("%d: calls=%d", code, n.Load())
		}
	}
}

func Test401RefreshOnceThenSuccess(t *testing.T) {
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.Header.Get("Authorization"))
		if r.Header.Get("Authorization") == "Bearer tok0" {
			w.WriteHeader(401)
			return
		}
		w.WriteHeader(200)
	}))
	defer srv.Close()
	fr := &fakeRefresher{}
	fc := newFake()
	resp, err := get(t, client(Config{Clock: fc, Refresher: fr}), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if fr.refreshed.Load() != 1 || len(seen) != 2 || seen[1] != "Bearer tok1" || len(fc.Slept()) != 0 {
		t.Fatalf("refreshed=%d seen=%v slept=%v", fr.refreshed.Load(), seen, fc.Slept())
	}
}

func TestSecond401IsAuthError(t *testing.T) {
	srv, n := script(t, []int{401}, nil)
	fr := &fakeRefresher{}
	_, err := get(t, client(Config{Clock: newFake(), Refresher: fr}), srv.URL)
	var ae *AuthError
	if !errors.As(err, &ae) || n.Load() != 2 || fr.refreshed.Load() != 1 {
		t.Fatalf("err=%v calls=%d refreshed=%d", err, n.Load(), fr.refreshed.Load())
	}
	if output.ExitOf(err) != output.ExitAuth || ae.Hint() == "" {
		t.Fatalf("exit=%d", output.ExitOf(err))
	}
}

func Test401WithoutRefresher(t *testing.T) {
	srv, n := script(t, []int{401}, nil)
	_, err := get(t, client(Config{Clock: newFake()}), srv.URL)
	if output.ExitOf(err) != output.ExitAuth || n.Load() != 1 {
		t.Fatalf("err=%v calls=%d", err, n.Load())
	}
}

func Test401RefreshFailure(t *testing.T) {
	srv, _ := script(t, []int{401}, nil)
	fr := &fakeRefresher{refreshErr: errors.New("daemon down")}
	_, err := get(t, client(Config{Clock: newFake(), Refresher: fr}), srv.URL)
	var ae *AuthError
	if !errors.As(err, &ae) || !strings.Contains(err.Error(), "daemon down") {
		t.Fatalf("err=%v", err)
	}
}

func TestAuthorizeError(t *testing.T) {
	srv, n := script(t, []int{200}, nil)
	fr := &fakeRefresher{authErr: errors.New("no token")}
	_, err := get(t, client(Config{Clock: newFake(), Refresher: fr}), srv.URL)
	if err == nil || !strings.Contains(err.Error(), "no token") || n.Load() != 0 {
		t.Fatalf("err=%v calls=%d", err, n.Load())
	}
}

func Test401SharesAttemptBudget(t *testing.T) {
	// 503, then 401 (refresh), then 503 would need a 4th attempt: budget 2 retries.
	srv, n := script(t, []int{503, 401, 503, 200}, nil)
	fr := &fakeRefresher{}
	_, err := get(t, client(Config{Clock: newFake(), Refresher: fr, MaxRetries: 2}), srv.URL)
	var rl *RateLimitedError
	if !errors.As(err, &rl) || n.Load() != 3 {
		t.Fatalf("err=%v calls=%d", err, n.Load())
	}
}

func Test401NoBudgetLeft(t *testing.T) {
	srv, n := script(t, []int{401, 200}, nil)
	fr := &fakeRefresher{}
	_, err := get(t, client(Config{Clock: newFake(), Refresher: fr, MaxRetries: -1}), srv.URL)
	if output.ExitOf(err) != output.ExitAuth || n.Load() != 1 || fr.refreshed.Load() != 0 {
		t.Fatalf("err=%v calls=%d", err, n.Load())
	}
}

func Test401NonIdempotentStillRefreshes(t *testing.T) {
	srv, n := script(t, []int{401, 200}, nil)
	fr := &fakeRefresher{}
	req, _ := http.NewRequest(http.MethodPost, srv.URL, strings.NewReader("x"))
	resp, err := client(Config{Clock: newFake(), Refresher: fr}).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if n.Load() != 2 {
		t.Fatalf("calls=%d", n.Load())
	}
}

func Test401UnreplayableBody(t *testing.T) {
	srv, n := script(t, []int{401, 200}, nil)
	fr := &fakeRefresher{}
	req, _ := http.NewRequest(http.MethodPost, srv.URL, io.NopCloser(strings.NewReader("x")))
	_, err := client(Config{Clock: newFake(), Refresher: fr}).Do(req)
	if output.ExitOf(err) != output.ExitAuth || n.Load() != 1 {
		t.Fatalf("err=%v calls=%d", err, n.Load())
	}
}

func Test403Forbidden(t *testing.T) {
	srv, n := script(t, []int{403}, map[string]string{"X-Err": "ACL_42"})
	cfg := Config{Clock: newFake(), VendorCode: func(h http.Header) string { return h.Get("X-Err") }}
	_, err := get(t, client(cfg), srv.URL)
	var fe *ForbiddenError
	if !errors.As(err, &fe) || fe.VendorCode != "ACL_42" || n.Load() != 1 {
		t.Fatalf("err=%v", err)
	}
	if output.ExitOf(err) != output.ExitForbidden {
		t.Fatalf("exit=%d", output.ExitOf(err))
	}
	if strings.Contains(err.Error(), "secret-body-content") || !strings.Contains(err.Error(), "ACL_42") {
		t.Fatalf("msg=%q", err.Error())
	}
}

func Test403NoVendorCode(t *testing.T) {
	srv, _ := script(t, []int{403}, nil)
	_, err := get(t, client(Config{Clock: newFake()}), srv.URL)
	var fe *ForbiddenError
	if !errors.As(err, &fe) || fe.VendorCode != "" || fe.Hint() == "" {
		t.Fatalf("err=%v", err)
	}
}

func Test403VendorCodeSanitized(t *testing.T) {
	long := strings.Repeat("a", 200)
	srv, _ := script(t, []int{403}, map[string]string{"X-Err": long})
	cfg := Config{Clock: newFake(), VendorCode: func(h http.Header) string { return h.Get("X-Err") }}
	_, err := get(t, client(cfg), srv.URL)
	var fe *ForbiddenError
	if !errors.As(err, &fe) || len(fe.VendorCode) > maxVendorCode {
		t.Fatalf("code len=%d", len(fe.VendorCode))
	}
}

type failingRT struct {
	n    atomic.Int32
	errs []error
}

func (f *failingRT) RoundTrip(*http.Request) (*http.Response, error) {
	i := int(f.n.Add(1)) - 1
	if i < len(f.errs) && f.errs[i] != nil {
		return nil, f.errs[i]
	}
	return &http.Response{StatusCode: 200, Body: http.NoBody, Header: http.Header{}}, nil
}

func TestNetworkErrorRetriedForIdempotent(t *testing.T) {
	rt := &failingRT{errs: []error{errors.New("reset"), nil}}
	tr := NewTransport(rt, Config{Clock: newFake()})
	req, _ := http.NewRequest(http.MethodGet, "https://x.invalid/", nil)
	resp, err := tr.RoundTrip(req)
	if err != nil || rt.n.Load() != 2 {
		t.Fatalf("err=%v n=%d", err, rt.n.Load())
	}
	_ = resp.Body.Close()
}

func TestNetworkErrorExhausted(t *testing.T) {
	boom := errors.New("reset")
	rt := &failingRT{errs: []error{boom, boom, boom, boom, boom}}
	tr := NewTransport(rt, Config{Clock: newFake(), MaxRetries: 1})
	req, _ := http.NewRequest(http.MethodGet, "https://x.invalid/", nil)
	_, err := tr.RoundTrip(req)
	var rl *RateLimitedError
	if !errors.As(err, &rl) || !errors.Is(err, boom) || rt.n.Load() != 2 || rl.Status != 0 {
		t.Fatalf("err=%v n=%d", err, rt.n.Load())
	}
}

func TestNetworkErrorNotRetriedForPost(t *testing.T) {
	boom := errors.New("reset")
	rt := &failingRT{errs: []error{boom}}
	tr := NewTransport(rt, Config{Clock: newFake()})
	req, _ := http.NewRequest(http.MethodPost, "https://x.invalid/", nil)
	_, err := tr.RoundTrip(req)
	if !errors.Is(err, boom) || rt.n.Load() != 1 {
		t.Fatalf("err=%v n=%d", err, rt.n.Load())
	}
	var rl *RateLimitedError
	if errors.As(err, &rl) {
		t.Fatal("non-retried error must not be reclassified")
	}
}

func TestContextCanceledNotRetried(t *testing.T) {
	rt := &failingRT{errs: []error{context.Canceled, nil}}
	tr := NewTransport(rt, Config{Clock: newFake()})
	req, _ := http.NewRequest(http.MethodGet, "https://x.invalid/", nil)
	_, err := tr.RoundTrip(req)
	if !errors.Is(err, context.Canceled) || rt.n.Load() != 1 {
		t.Fatalf("err=%v n=%d", err, rt.n.Load())
	}
}

func TestCancelDuringSleep(t *testing.T) {
	srv, n := script(t, []int{503, 200}, map[string]string{"Retry-After": "30"})
	fc := clock.NewFake(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)) // manual: sleeps block
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)
		_, err := client(Config{Clock: fc, MaxWait: time.Minute}).Do(req)
		done <- err
	}()
	fc.BlockUntil(1)
	cancel()
	err := <-done
	if !errors.Is(err, context.Canceled) || n.Load() != 1 {
		t.Fatalf("err=%v calls=%d", err, n.Load())
	}
}

func TestCanceledBeforeStart(t *testing.T) {
	srv, n := script(t, []int{200}, nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)
	_, err := client(Config{Clock: newFake()}).Do(req)
	if !errors.Is(err, context.Canceled) || n.Load() != 0 {
		t.Fatalf("err=%v calls=%d", err, n.Load())
	}
}

func TestDefaultsAndBackoffCap(t *testing.T) {
	c := Config{}.withDefaults()
	if c.MaxRetries != 3 || c.BaseDelay <= 0 || c.MaxDelay <= 0 || c.MaxWait <= 0 || c.Clock == nil || c.Rand == nil || c.Redactor == nil {
		t.Fatalf("%+v", c)
	}
	if d := c.backoff(30); d > c.MaxDelay*2 {
		t.Fatalf("backoff not capped: %v", d)
	}
	neg := Config{MaxRetries: -1}.withDefaults()
	if neg.MaxRetries != 0 {
		t.Fatalf("%d", neg.MaxRetries)
	}
	if j := (Config{Jitter: 5}).withDefaults().Jitter; j != 1 {
		t.Fatalf("jitter %v", j)
	}
	if j := (Config{Jitter: -1}).withDefaults().Jitter; j != 0 {
		t.Fatalf("jitter %v", j)
	}
}

func TestNewClientKeepsBase(t *testing.T) {
	rt := &failingRT{}
	tr := NewTransport(nil, Config{})
	if tr.base != http.DefaultTransport {
		t.Fatal("nil base should default")
	}
	tr = NewTransport(rt, Config{})
	if tr.base != rt {
		t.Fatal("base not kept")
	}
}

func TestRealClockDefault(t *testing.T) {
	srv, _ := script(t, []int{200}, nil)
	resp, err := get(t, NewClient(Config{}), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
}

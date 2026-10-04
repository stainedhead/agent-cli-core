package httpx_test

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stainedhead/agent-cli-core/clock"
	"github.com/stainedhead/agent-cli-core/httpx"
)

func fakeClock() *clock.Fake {
	c := clock.NewFake(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	c.SetAutoAdvance(true)
	return c
}

func serve(t *testing.T, status int, hdr map[string]string, body string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for k, v := range hdr {
			w.Header().Set(k, v)
		}
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func get(t *testing.T, srv *httptest.Server, cfg httpx.Config) error {
	t.Helper()
	cfg.Clock = fakeClock()
	resp, err := httpx.NewClient(cfg).Get(srv.URL)
	if err == nil {
		_ = resp.Body.Close()
	}
	return err
}

func forbidden(t *testing.T, err error) *httpx.ForbiddenError {
	t.Helper()
	var fe *httpx.ForbiddenError
	if !errors.As(err, &fe) {
		t.Fatalf("want *ForbiddenError, got %v", err)
	}
	return fe
}

const graphBody = `{"error":{"code":"ErrorAccessDenied","message":"Access is denied."}}`

func graphHook(status int, prefix []byte) string {
	s := string(prefix)
	const k = `"code":"`
	i := strings.Index(s, k)
	if i < 0 {
		return ""
	}
	rest := s[i+len(k):]
	if j := strings.IndexByte(rest, '"'); j >= 0 {
		return rest[:j]
	}
	return ""
}

func TestVendorCodeFromBody(t *testing.T) {
	srv := serve(t, 403, nil, graphBody)
	var gotStatus int
	err := get(t, srv, httpx.Config{VendorCodeFromBody: func(s int, p []byte) string {
		gotStatus = s
		return graphHook(s, p)
	}})
	fe := forbidden(t, err)
	if fe.VendorCode != "ErrorAccessDenied" || gotStatus != 403 {
		t.Fatalf("code=%q status=%d", fe.VendorCode, gotStatus)
	}
	if strings.Contains(err.Error(), "Access is denied") {
		t.Fatal("body text leaked into the error")
	}
}

func TestVendorCodeHeaderWinsAndBodyNotRead(t *testing.T) {
	srv := serve(t, 403, map[string]string{"X-Err": "FROM_HEADER"}, graphBody)
	var called atomic.Bool
	err := get(t, srv, httpx.Config{
		VendorCode:         func(h http.Header) string { return h.Get("X-Err") },
		VendorCodeFromBody: func(int, []byte) string { called.Store(true); return "FROM_BODY" },
	})
	if fe := forbidden(t, err); fe.VendorCode != "FROM_HEADER" || called.Load() {
		t.Fatalf("code=%q hookCalled=%v", fe.VendorCode, called.Load())
	}
}

func TestVendorCodeBodyFallsBackWhenHeaderEmpty(t *testing.T) {
	srv := serve(t, 403, nil, graphBody)
	err := get(t, srv, httpx.Config{
		VendorCode:         func(h http.Header) string { return h.Get("X-Err") },
		VendorCodeFromBody: graphHook,
	})
	if fe := forbidden(t, err); fe.VendorCode != "ErrorAccessDenied" {
		t.Fatalf("code=%q", fe.VendorCode)
	}
}

func TestVendorBodyPrefixBounded(t *testing.T) {
	srv := serve(t, 403, nil, strings.Repeat("x", 70000))
	var n int
	_ = get(t, srv, httpx.Config{VendorCodeFromBody: func(_ int, p []byte) string { n = len(p); return "" }})
	if n != httpx.DefaultVendorBodyLimit {
		t.Fatalf("hook saw %d bytes, want %d", n, httpx.DefaultVendorBodyLimit)
	}
	_ = get(t, srv, httpx.Config{VendorBodyLimit: 100, VendorCodeFromBody: func(_ int, p []byte) string { n = len(p); return "" }})
	if n != 100 {
		t.Fatalf("custom limit: %d", n)
	}
	_ = get(t, srv, httpx.Config{VendorBodyLimit: 1 << 30, VendorCodeFromBody: func(_ int, p []byte) string { n = len(p); return "" }})
	if n != httpx.MaxVendorBodyLimit {
		t.Fatalf("limit not capped: %d", n)
	}
}

func TestVendorBodyShortAndEmpty(t *testing.T) {
	for _, body := range []string{"", "abc"} {
		srv := serve(t, 403, nil, body)
		var got string
		err := get(t, srv, httpx.Config{VendorCodeFromBody: func(_ int, p []byte) string { got = string(p); return "C" }})
		if fe := forbidden(t, err); fe.VendorCode != "C" || got != body {
			t.Fatalf("body %q: code=%q got=%q", body, fe.VendorCode, got)
		}
	}
}

func TestVendorCodeFromBodyBoundedAndScrubbed(t *testing.T) {
	srv := serve(t, 403, nil, "x")
	err := get(t, srv, httpx.Config{VendorCodeFromBody: func(int, []byte) string {
		return "  " + strings.Repeat("é", 100) + "\x00\n\x1b[31m"
	}})
	fe := forbidden(t, err)
	if len(fe.VendorCode) > 64 || strings.ContainsAny(fe.VendorCode, "\x00\n\x1b") || !strings.HasPrefix(fe.VendorCode, "é") {
		t.Fatalf("code = %q", fe.VendorCode)
	}
	for _, r := range fe.VendorCode {
		if r == '�' {
			t.Fatalf("cut in the middle of a rune: %q", fe.VendorCode)
		}
	}
	// A credential echoed back through the body hook is scrubbed.
	err = get(t, srv, httpx.Config{VendorCodeFromBody: func(int, []byte) string {
		return "Bearer abcdefghijklmnopqrstuvwxyz012345"
	}})
	if strings.Contains(forbidden(t, err).VendorCode, "abcdefghijklmnopqrstuvwxyz012345") {
		t.Fatal("secret in vendor code")
	}
}

func TestVendorBodyHookOnlyFor403(t *testing.T) {
	for _, status := range []int{401, 429} {
		srv := serve(t, status, nil, graphBody)
		var called atomic.Bool
		err := get(t, srv, httpx.Config{
			MaxRetries:         -1,
			VendorCodeFromBody: func(int, []byte) string { called.Store(true); return "X" },
		})
		if err == nil || called.Load() {
			t.Fatalf("status %d: err=%v called=%v", status, err, called.Load())
		}
	}
}

// brokenBody errors after a few bytes.
type brokenBody struct{ sent bool }

func (b *brokenBody) Read(p []byte) (int, error) {
	if !b.sent {
		b.sent = true
		return copy(p, "partial"), nil
	}
	return 0, errors.New("connection reset")
}
func (b *brokenBody) Close() error { return nil }

type rt func(*http.Request) (*http.Response, error)

func (f rt) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestVendorBodyReadErrorSkipsHook(t *testing.T) {
	base := rt(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 403, Header: http.Header{}, Body: &brokenBody{}, Request: r}, nil
	})
	var called atomic.Bool
	tr := httpx.NewTransport(base, httpx.Config{Clock: fakeClock(), VendorCodeFromBody: func(int, []byte) string { called.Store(true); return "X" }})
	req, _ := http.NewRequest("GET", "http://127.0.0.1/", nil)
	_, err := tr.RoundTrip(req)
	if fe := forbidden(t, err); fe.VendorCode != "" || called.Load() {
		t.Fatalf("code=%q called=%v", fe.VendorCode, called.Load())
	}
}

func TestVendorBodyNilBody(t *testing.T) {
	base := rt(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 403, Header: http.Header{}, Request: r}, nil
	})
	tr := httpx.NewTransport(base, httpx.Config{Clock: fakeClock(), VendorCodeFromBody: func(_ int, p []byte) string {
		return fmt.Sprint(len(p))
	}})
	req, _ := http.NewRequest("GET", "http://127.0.0.1/", nil)
	_, err := tr.RoundTrip(req)
	if fe := forbidden(t, err); fe.VendorCode != "0" {
		t.Fatalf("code=%q", fe.VendorCode)
	}
}

func ExampleConfig_vendorCodeFromBody() {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(w, `{"error":{"code":"ErrorAccessDenied"}}`)
	}))
	defer srv.Close()
	c := httpx.NewClient(httpx.Config{
		// Only the first bytes of the body are offered; the body is never kept.
		VendorCodeFromBody: func(status int, prefix []byte) string {
			if i := strings.Index(string(prefix), `"code":"`); i >= 0 {
				rest := string(prefix[i+8:])
				return rest[:strings.IndexByte(rest, '"')]
			}
			return ""
		},
	})
	_, err := c.Get(srv.URL)
	var fe *httpx.ForbiddenError
	if errors.As(err, &fe) {
		fmt.Println(fe.VendorCode)
	}
	// Output: ErrorAccessDenied
}

package httpx

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stainedhead/agent-cli-core/output"
)

// TestCrossHostRedirectDropsAuthorization is the FR-001 repro: a redirect to
// another host must never carry the Authorization header.
func TestCrossHostRedirectDropsAuthorization(t *testing.T) {
	var leaked atomic.Int32
	evil := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			leaked.Add(1)
		}
	}))
	defer evil.Close()
	// Use a different host name for the same loopback listener.
	evilURL := strings.Replace(evil.URL, "127.0.0.1", "localhost", 1)
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, evilURL, http.StatusFound)
	}))
	defer origin.Close()

	c := NewClient(Config{Refresher: &fakeRefresher{}, Clock: newFake()})
	_, err := get(t, c, origin.URL)
	if err == nil {
		t.Fatal("expected cross-host redirect to be refused")
	}
	var fh *ForbiddenHostError
	if !errors.As(err, &fh) {
		t.Fatalf("want *ForbiddenHostError, got %T: %v", err, err)
	}
	if leaked.Load() != 0 {
		t.Fatal("Authorization reached the redirect target")
	}
	if output.CategoryOf(err) != output.CategoryForbidden {
		t.Fatalf("category = %v", output.CategoryOf(err))
	}
	if fh.Code() != "auth/forbidden-host" || fh.Hint() == "" {
		t.Fatalf("code/hint: %q %q", fh.Code(), fh.Hint())
	}
	if strings.Contains(fh.Error(), "Bearer") {
		t.Fatal("error leaks credential")
	}
}

func TestCrossHostRedirectAllowedWhenListed(t *testing.T) {
	var got atomic.Value
	dst := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.Store(r.Header.Get("Authorization"))
	}))
	defer dst.Close()
	dstURL := strings.Replace(dst.URL, "127.0.0.1", "localhost", 1)
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, dstURL, http.StatusFound)
	}))
	defer origin.Close()
	ou, _ := url.Parse(origin.URL)
	du, _ := url.Parse(dstURL)
	c := NewClient(Config{Refresher: &fakeRefresher{}, Clock: newFake(), AllowedHosts: []string{ou.Host, du.Hostname()}})
	resp, err := get(t, c, origin.URL)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if got.Load() != "Bearer tok0" {
		t.Fatalf("listed host should be authorized, got %v", got.Load())
	}
}

func TestSameHostRedirectWorks(t *testing.T) {
	var hits atomic.Int32
	var auth atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/final" {
			hits.Add(1)
			auth.Store(r.Header.Get("Authorization"))
			return
		}
		http.Redirect(w, r, "/final", http.StatusFound)
	}))
	defer srv.Close()
	c := NewClient(Config{Refresher: &fakeRefresher{}, Clock: newFake()})
	resp, err := get(t, c, srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if hits.Load() != 1 || auth.Load() != "Bearer tok0" {
		t.Fatalf("hits=%d auth=%v", hits.Load(), auth.Load())
	}
}

func TestRedirectLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/loop", http.StatusFound)
	}))
	defer srv.Close()
	c := NewClient(Config{Clock: newFake()})
	if _, err := get(t, c, srv.URL); err == nil || !strings.Contains(err.Error(), "redirects") {
		t.Fatalf("want redirect-limit error, got %v", err)
	}
}

func TestPlainHTTPNonLoopbackRefused(t *testing.T) {
	var calls atomic.Int32
	base := rtFunc(func(*http.Request) (*http.Response, error) {
		calls.Add(1)
		return &http.Response{StatusCode: 200, Body: http.NoBody}, nil
	})
	fr := &fakeRefresher{}
	tr := NewTransport(base, Config{Refresher: fr})
	req, _ := http.NewRequest(http.MethodGet, "http://api.example.com/x", nil)
	_, err := tr.RoundTrip(req)
	var fh *ForbiddenHostError
	if !errors.As(err, &fh) {
		t.Fatalf("want *ForbiddenHostError, got %v", err)
	}
	if calls.Load() != 0 || fr.authorized.Load() != 0 {
		t.Fatal("request must not be authorized or sent")
	}
	if !strings.Contains(fh.Error(), "api.example.com") {
		t.Fatalf("error should name host: %v", fh)
	}
}

func TestPlainHTTPAllowedExplicitly(t *testing.T) {
	base := rtFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: http.NoBody}, nil
	})
	tr := NewTransport(base, Config{AllowInsecureHTTP: true})
	req, _ := http.NewRequest(http.MethodGet, "http://api.example.com/x", nil)
	if _, err := tr.RoundTrip(req); err != nil {
		t.Fatal(err)
	}
}

func TestLoopbackHTTPAllowed(t *testing.T) {
	base := rtFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: http.NoBody}, nil
	})
	for _, u := range []string{"http://127.0.0.1:1/", "http://localhost/", "http://[::1]:8080/"} {
		tr := NewTransport(base, Config{})
		req, _ := http.NewRequest(http.MethodGet, u, nil)
		if _, err := tr.RoundTrip(req); err != nil {
			t.Fatalf("%s: %v", u, err)
		}
	}
}

func TestHTTPSAllowedAndPinnedToFirstHost(t *testing.T) {
	base := rtFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: http.NoBody}, nil
	})
	tr := NewTransport(base, Config{})
	r1, _ := http.NewRequest(http.MethodGet, "https://API.example.com/a", nil)
	if _, err := tr.RoundTrip(r1); err != nil {
		t.Fatal(err)
	}
	r2, _ := http.NewRequest(http.MethodGet, "https://api.example.com:443/b", nil)
	if _, err := tr.RoundTrip(r2); err == nil {
		// host:port differs from pinned host; matching is by host or host:port entry
		t.Log("same hostname with explicit port accepted")
	}
	r3, _ := http.NewRequest(http.MethodGet, "https://other.example.com/c", nil)
	_, err := tr.RoundTrip(r3)
	var fh *ForbiddenHostError
	if !errors.As(err, &fh) {
		t.Fatalf("second host must be refused, got %v", err)
	}
}

func TestAllowedHostsExplicit(t *testing.T) {
	base := rtFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: http.NoBody}, nil
	})
	tr := NewTransport(base, Config{AllowedHosts: []string{"a.example.com", "b.example.com:8443"}})
	for _, u := range []string{"https://a.example.com/", "https://a.example.com:9/", "https://b.example.com:8443/"} {
		req, _ := http.NewRequest(http.MethodGet, u, nil)
		if _, err := tr.RoundTrip(req); err != nil {
			t.Fatalf("%s: %v", u, err)
		}
	}
	req, _ := http.NewRequest(http.MethodGet, "https://b.example.com:9/", nil)
	if _, err := tr.RoundTrip(req); err == nil {
		t.Fatal("b.example.com on another port must be refused")
	}
	req, _ = http.NewRequest(http.MethodGet, "https://c.example.com/", nil)
	if _, err := tr.RoundTrip(req); err == nil {
		t.Fatal("unlisted host must be refused")
	}
}

func TestForbiddenHostErrorText(t *testing.T) {
	e := &ForbiddenHostError{Host: "x.example.com"}
	if !strings.Contains(e.Error(), "x.example.com") {
		t.Fatal(e.Error())
	}
	e = &ForbiddenHostError{Host: "x.example.com", Insecure: true}
	if !strings.Contains(e.Error(), "plain http") {
		t.Fatal(e.Error())
	}
}

type rtFunc func(*http.Request) (*http.Response, error)

func (f rtFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestRefusalIsTraced(t *testing.T) {
	var buf strings.Builder
	tr := NewTransport(rtFunc(func(*http.Request) (*http.Response, error) { return nil, errors.New("must not send") }), Config{Trace: &buf})
	req, _ := http.NewRequest(http.MethodGet, "http://api.example.com/x?token=abc", nil)
	if _, err := tr.RoundTrip(req); err == nil {
		t.Fatal("want refusal")
	}
	if !strings.Contains(buf.String(), "forbidden host") || strings.Contains(buf.String(), "token=abc") {
		t.Fatalf("trace: %s", buf.String())
	}
}

func TestEmptyHostRefused(t *testing.T) {
	tr := NewTransport(rtFunc(func(*http.Request) (*http.Response, error) { return nil, nil }), Config{})
	req, _ := http.NewRequest(http.MethodGet, "/relative", nil)
	var fh *ForbiddenHostError
	if _, err := tr.RoundTrip(req); !errors.As(err, &fh) {
		t.Fatalf("got %v", err)
	}
}

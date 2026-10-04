package httpx

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stainedhead/agent-cli-core/internal/redact"
	"github.com/stainedhead/agent-cli-core/output"
)

const leakSecret = "abc-123-xyz-registered"

func sendErrWith(secret string) error {
	return &url.Error{
		Op:  "Get",
		URL: "https://user:pw@api.example.com/v1/items?page=2&sig=" + secret,
		Err: errors.New("dial tcp: connection reset while sending " + secret),
	}
}

func TestRateLimitedSendErrorScrubbed(t *testing.T) {
	rt := &failingRT{errs: []error{sendErrWith(leakSecret), sendErrWith(leakSecret)}}
	tr := NewTransport(rt, Config{Clock: newFake(), MaxRetries: 1, Redactor: redact.New(leakSecret)})
	req, _ := http.NewRequest(http.MethodGet, "https://x.invalid/", nil)
	_, err := tr.RoundTrip(req)
	var rl *RateLimitedError
	if !errors.As(err, &rl) {
		t.Fatalf("err=%v", err)
	}
	for _, bad := range []string{leakSecret, "pw@", "sig=", "page=2"} {
		if strings.Contains(err.Error(), bad) {
			t.Fatalf("%q leaked: %v", bad, err)
		}
	}
	if !strings.Contains(err.Error(), "api.example.com/v1/items") {
		t.Fatalf("host/path should remain: %v", err)
	}
}

func TestNonRetriedSendErrorScrubbed(t *testing.T) {
	rt := &failingRT{errs: []error{sendErrWith(leakSecret)}}
	tr := NewTransport(rt, Config{Clock: newFake(), Redactor: redact.New(leakSecret)})
	req, _ := http.NewRequest(http.MethodPost, "https://x.invalid/", nil)
	_, err := tr.RoundTrip(req)
	if err == nil || strings.Contains(err.Error(), leakSecret) || strings.Contains(err.Error(), "sig=") {
		t.Fatalf("leak: %v", err)
	}
	var ue *url.Error
	if !errors.As(err, &ue) {
		t.Fatalf("chain must remain inspectable: %v", err)
	}
}

func TestHeldCredentialScrubbedFromSendError(t *testing.T) {
	fr := &fakeRefresher{}
	fr.token.Store("zzplainsecretzz")
	rt := &failingRT{errs: []error{errors.New("proxy echoed zzplainsecretzz")}}
	tr := NewTransport(rt, Config{Clock: newFake(), Refresher: fr})
	req, _ := http.NewRequest(http.MethodPost, "https://x.invalid/", nil)
	_, err := tr.RoundTrip(req)
	if err == nil || strings.Contains(err.Error(), "zzplainsecretzz") {
		t.Fatalf("leak: %v", err)
	}
}

func TestRateLimitedErrorErrDefenseInDepth(t *testing.T) {
	e := &RateLimitedError{Attempts: 2, Err: errors.New("failed: Authorization: Bearer abc.def")}
	if strings.Contains(e.Error(), "abc.def") {
		t.Fatalf("leak: %v", e)
	}
}

type catRefreshErr struct{}

func (catRefreshErr) Error() string             { return "daemon degraded" }
func (catRefreshErr) Hint() string              { return "retry in 30 seconds" }
func (catRefreshErr) Category() output.Category { return output.CategoryRateLimited }

func TestAuthErrorKeepsRefreshFailureCategoryAndHint(t *testing.T) {
	e := &AuthError{Err: catRefreshErr{}}
	if output.ExitOf(e) != output.ExitRateLimited {
		t.Fatalf("exit %d, want rate limited", output.ExitOf(e))
	}
	if env := output.FromError(e); env.Error.Hint != "retry in 30 seconds" {
		t.Fatalf("hint %q", env.Error.Hint)
	}
	plain := &AuthError{Err: errors.New("boom")}
	if output.ExitOf(plain) != output.ExitAuth || plain.Hint() == "" {
		t.Fatalf("uncategorized refresh failure must stay auth with the generic hint")
	}
}

func TestRefreshFailureSurfacesThroughTransport401(t *testing.T) {
	fr := &fakeRefresher{refreshErr: catRefreshErr{}}
	fr.token.Store("tok")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusUnauthorized) }))
	defer srv.Close()
	c := NewClient(Config{Refresher: fr, Clock: newFake()})
	_, err := c.Get(srv.URL)
	if err == nil || output.ExitOf(err) != output.ExitRateLimited {
		t.Fatalf("err %v exit %d", err, output.ExitOf(err))
	}
}

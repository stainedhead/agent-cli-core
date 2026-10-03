package redact_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/stainedhead/agent-cli-core/internal/redact"
)

const jwt = "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dBjftJeZ4CVPmB92K27uhbUJU1p1r_wW1gFWFOEjXk"

func TestStringTable(t *testing.T) {
	r := redact.New()
	cases := []struct {
		name, in, want string
	}{
		{"empty", "", ""},
		{"plain", "resource not found", "resource not found"},
		{"uuid kept", "request 123e4567-e89b-12d3-a456-426614174000 failed", "request 123e4567-e89b-12d3-a456-426614174000 failed"},
		{"bearer", "got Authorization: Bearer abc.def-123 here", "got Authorization: [redacted] here"},
		{"bearer lower", "bearer SECRETVALUE1", "[redacted]"},
		{"basic", "Authorization: Basic dXNlcjpwYXNz", "Authorization: [redacted]"},
		{"jwt", "token was " + jwt + " ok", "token was [redacted] ok"},
		{"query", "GET /x?access_token=abc123&page=2", "GET /x?access_token=[redacted]&page=2"},
		{"query secret", "client_secret=s3cr3t;next", "client_secret=[redacted];next"},
		{"json", `{"refresh_token":"abc","name":"x"}`, `{"refresh_token":"[redacted]","name":"x"}`},
		{"json spaced", `{"password": "hunter2"}`, `{"password": "[redacted]"}`},
		{"opaque", "id " + strings.Repeat("aB3", 20) + " end", "id [redacted] end"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := r.String(c.in); got != c.want {
				t.Fatalf("got %q want %q", got, c.want)
			}
		})
	}
}

func TestRegisteredSecrets(t *testing.T) {
	r := redact.New("hunter2xyz", "", "ab")
	got := r.String("pw=hunter2xyz and ab stays; HUNTER2XYZ not")
	if strings.Contains(got, "hunter2xyz") {
		t.Fatalf("secret leaked: %q", got)
	}
	if !strings.Contains(got, " ab stays") {
		t.Fatalf("short value must be ignored: %q", got)
	}
}

func TestRegisteredSecretsLongestFirst(t *testing.T) {
	r := redact.New("secret-abc", "secret-abcdef")
	if got := r.String("x secret-abcdef y"); got != "x [redacted] y" {
		t.Fatalf("got %q", got)
	}
}

func TestHeader(t *testing.T) {
	h := http.Header{}
	h.Set("Authorization", "Bearer abc")
	h.Set("Cookie", "sid=1")
	h.Set("X-Api-Key", "k")
	h.Set("X-Auth-Token", "t")
	h.Set("Content-Type", "application/json")
	h.Add("X-Note", "Bearer zzz123")
	out := redact.New().Header(h)
	for _, k := range []string{"Authorization", "Cookie", "X-Api-Key", "X-Auth-Token"} {
		if v := out.Get(k); v != "[redacted]" {
			t.Errorf("%s = %q", k, v)
		}
	}
	if out.Get("Content-Type") != "application/json" {
		t.Error("content-type changed")
	}
	if out.Get("X-Note") != "[redacted]" {
		t.Errorf("value scrub missing: %q", out.Get("X-Note"))
	}
	if h.Get("Authorization") != "Bearer abc" {
		t.Error("input header mutated")
	}
	if redact.New().Header(nil) != nil {
		t.Error("nil header must stay nil")
	}
}

func TestBodyOmittedByDefault(t *testing.T) {
	r := redact.New()
	if got := r.Body([]byte("secret payload")); got != "[body omitted: 14 bytes]" {
		t.Fatalf("got %q", got)
	}
	if got := r.Body(nil); got != "[body omitted: 0 bytes]" {
		t.Fatalf("got %q", got)
	}
}

func TestErrorf(t *testing.T) {
	r := redact.New("hunter2xyz")
	err := r.Error(fmt.Errorf("call failed: Bearer abc123 hunter2xyz"))
	if strings.Contains(err.Error(), "abc123") || strings.Contains(err.Error(), "hunter2xyz") {
		t.Fatalf("leak: %v", err)
	}
	if r.Error(nil) != nil {
		t.Fatal("nil error must stay nil")
	}
}

func FuzzStringNeverLeaksRegistered(f *testing.F) {
	f.Add("tok-12345678", "prefix %s suffix")
	f.Add("Zm9vYmFy-abc", `{"access_token":"%s"}`)
	f.Add("sécret-ünï", "%s%s")
	f.Fuzz(func(t *testing.T, secret, format string) {
		if len(secret) < 6 || strings.Contains(secret, "[redacted]") ||
			strings.Contains("[redacted]", secret) {
			t.Skip()
		}
		r := redact.New(secret)
		in := strings.ReplaceAll(format, "%s", secret) + secret
		if got := r.String(in); strings.Contains(got, secret) {
			t.Fatalf("leaked %q in %q", secret, got)
		}
	})
}

func ExampleRedactor_String() {
	r := redact.New()
	fmt.Println(r.String("failed: Authorization: Bearer abc.def and token=xyz"))
	// Output: failed: Authorization: [redacted] and token=[redacted]
}

func TestAuthSchemeProseNotOverMatched(t *testing.T) {
	r := redact.New()
	keep := []string{
		"the basic usage of the tool",
		"Basic information about the host",
		"a bearer of bad news",
		"basic authentication is not supported",
		"Bearer of Tidings",
		"use basic, then bearer",
	}
	for _, in := range keep {
		if got := r.String(in); got != in {
			t.Errorf("prose over-redacted: %q -> %q", in, got)
		}
	}
}

func TestAuthSchemeCredentialsStillRedacted(t *testing.T) {
	r := redact.New()
	cases := []struct{ in, want string }{
		{"Authorization: Bearer abc", "Authorization: [redacted]"},
		{"authorization=basic zz", "authorization=[redacted]"},
		{"bearer SECRETVALUE1", "[redacted]"},
		{"Basic dXNlcjpwYXNz", "[redacted]"},
		{"sent bearer abc.def-123 here", "sent [redacted] here"},
	}
	for _, c := range cases {
		if got := r.String(c.in); got != c.want {
			t.Errorf("%q -> %q want %q", c.in, got, c.want)
		}
	}
}

func TestOpaqueThresholdAndSHA1(t *testing.T) {
	r := redact.New()
	sha1 := strings.Repeat("0123456789abcdef", 3)[:40]
	if got := r.String("commit " + sha1 + " ok"); got != "commit "+sha1+" ok" {
		t.Errorf("git SHA-1 must be preserved: %q", got)
	}
	sha256 := strings.Repeat("a1", 32)
	if got := r.String("x " + sha256); got != "x [redacted]" {
		t.Errorf("64-hex run must be redacted: %q", got)
	}
	run41 := strings.Repeat("aB3", 14)[:41]
	if got := r.String(run41); got != "[redacted]" {
		t.Errorf("41-char run: %q", got)
	}
	mixed40 := strings.Repeat("aB3", 14)[:40]
	if got := r.String(mixed40); got != "[redacted]" {
		t.Errorf("40-char non-hex run must be redacted: %q", got)
	}
	if redact.OpaqueRunMin != 40 {
		t.Errorf("OpaqueRunMin = %d", redact.OpaqueRunMin)
	}
}

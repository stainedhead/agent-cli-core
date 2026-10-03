package integration_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stainedhead/agent-cli-core/auth"
	"github.com/stainedhead/agent-cli-core/examples/sampletool"
	"github.com/stainedhead/agent-cli-core/httpx"
	"github.com/stainedhead/agent-cli-core/internal/clock"
)

// canaryDaemon issues a known token so the test can look for it. refreshed
// tokens differ so both generations are tracked.
type canaryDaemon struct {
	tokens []string // issued so far
	base   string
	errMsg string // when set, Fetch fails with this message
}

func (d *canaryDaemon) issue() auth.Token {
	v := fmt.Sprintf("%s.%d", d.base, len(d.tokens))
	d.tokens = append(d.tokens, v)
	return auth.NewToken(v)
}

func (d *canaryDaemon) Fetch(context.Context, string) (auth.Token, error) {
	if d.errMsg != "" {
		return auth.Token{}, errors.New(d.errMsg)
	}
	return d.issue(), nil
}

func (d *canaryDaemon) Refresh(context.Context, string) (auth.Token, error) {
	if d.errMsg != "" {
		return auth.Token{}, errors.New(d.errMsg)
	}
	return d.issue(), nil
}

// canaryToken builds a token from fuzz input. The prefix keeps it unique so a
// substring match cannot be a coincidence with ordinary output; the form
// selects opaque, JWT-shaped or long-opaque tokens.
func canaryToken(form uint8, raw string) string {
	var b strings.Builder
	for _, r := range raw {
		if r > 0x20 && r < 0x7f && r != '"' && r != '\\' && r != ',' && r != ';' {
			b.WriteRune(r)
		}
		if b.Len() >= 40 {
			break
		}
	}
	body := b.String()
	for len(body) < 12 {
		body += "x7"
	}
	// Pattern-detectable forms use only the base64url alphabet, as real JWTs
	// and API keys do.
	b64 := strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
			return r
		}
		return '_'
	}, body)
	body = b64
	switch form % 3 {
	case 1:
		return "eyJcny" + strings.NewReplacer(".", "_").Replace(body) + ".payloadcny" + body + ".sigcny" + body
	case 2:
		return "cnyopaque" + strings.NewReplacer(".", "_").Replace(body) + strings.Repeat("Q", 30)
	}
	return "cny-" + raw0(raw)
}

// raw0 keeps arbitrary printable characters for the opaque short form.
func raw0(raw string) string {
	var b strings.Builder
	for _, r := range raw {
		if r > 0x20 && r < 0x7f && r != '"' && r != '\\' && r != ',' && r != ';' {
			b.WriteRune(r)
		}
		if b.Len() >= 40 {
			break
		}
	}
	for b.Len() < 12 {
		b.WriteString("x7")
	}
	return b.String()
}

// hostile returns a fake upstream that misbehaves in a way selected by mode
// and tries to get the token it is shown echoed back through every channel
// the library surfaces: status text, headers, body and Retry-After.
func hostile(mode uint8) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tok := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		switch mode % 8 {
		case 0: // well-behaved
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(w, `{"id":"A1","title":"t","description":"d","author":"a"}`)
		case 1: // 403 echoing the token as a vendor code
			w.Header().Set("X-Vendor-Code", tok)
			w.WriteHeader(http.StatusForbidden)
			_, _ = fmt.Fprint(w, tok)
		case 2: // always 401, token everywhere
			w.Header().Set("WWW-Authenticate", `Bearer error="`+tok+`"`)
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = fmt.Fprint(w, tok)
		case 3: // 500 with body
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = fmt.Fprint(w, tok)
		case 4: // 429 with junk Retry-After
			w.Header().Set("Retry-After", tok)
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = fmt.Fprint(w, tok)
		case 5: // 503 persistent
			w.Header().Set("X-Request-Id", tok)
			w.WriteHeader(http.StatusServiceUnavailable)
		case 6: // 404 with body
			w.WriteHeader(http.StatusNotFound)
			_, _ = fmt.Fprint(w, tok)
		case 7: // 409 with header
			w.Header().Set("Location", "/x?token="+tok)
			w.WriteHeader(http.StatusConflict)
		}
	})
}

// checkNoLeak fails when any held token appears in any captured surface.
func checkNoLeak(t *testing.T, tokens []string, surfaces map[string]string) {
	t.Helper()
	for name, text := range surfaces {
		for _, tok := range tokens {
			if tok != "" && strings.Contains(text, tok) {
				t.Fatalf("token %q leaked into %s:\n%s", tok, name, text)
			}
		}
	}
}

// runOnce drives one command against the hostile upstream and returns every
// surface a token could leak into.
func runOnce(t *testing.T, form, mode uint8, raw string, daemonErr bool, cmd []string) {
	t.Helper()
	d := &canaryDaemon{base: canaryToken(form, raw)}
	if daemonErr {
		// Only meaningful for pattern-detectable forms: a daemon that puts a
		// token it did not hand to the library into its own error text is
		// outside what the library can know.
		d.errMsg = "daemon failure for " + d.base
	}
	r := newRig(t, d, hostile(mode), func(c *sampletool.Config) {
		c.HTTP.VendorCode = func(h http.Header) string { return h.Get("X-Vendor-Code") }
	})
	r.out.Reset()
	code := r.tool.Run(context.Background(), cmd)
	_ = code

	tokens := append([]string(nil), d.tokens...)
	if daemonErr {
		tokens = append(tokens, d.base)
	}
	surfaces := map[string]string{
		"envelope": r.out.String(),
		"audit":    r.audit.String(),
		"trace":    r.trace.String(),
	}
	checkNoLeak(t, tokens, surfaces)
	// The envelope must stay one valid JSON document.
	var env map[string]any
	if err := json.Unmarshal(r.out.Bytes(), &env); err != nil {
		t.Fatalf("invalid envelope: %v\n%s", err, r.out.String())
	}
}

func FuzzNoTokenLeak(f *testing.F) {
	f.Add(uint8(0), uint8(1), "abc123def456", false, uint8(0))
	f.Add(uint8(1), uint8(2), "tok", false, uint8(1))
	f.Add(uint8(2), uint8(3), "Zm9v", true, uint8(0))
	f.Add(uint8(0), uint8(4), "a/b+c=", false, uint8(0))
	f.Add(uint8(1), uint8(5), "-_-_-_-_-", true, uint8(1))
	f.Add(uint8(0), uint8(6), "k", false, uint8(0))
	f.Add(uint8(2), uint8(7), "secret", false, uint8(1))
	f.Fuzz(func(t *testing.T, form, mode uint8, raw string, daemonErr bool, which uint8) {
		// Daemon-error text can be scrubbed only when it is pattern-shaped.
		if daemonErr && form%3 == 0 {
			daemonErr = false
		}
		cmd := []string{"get", "A1"}
		if which%2 == 1 {
			cmd = []string{"create", "title", "--priority", "2"}
		}
		runOnce(t, form, mode, raw, daemonErr, cmd)
	})
}

// TestNoTokenLeakMatrix runs every hostile mode for every token form so the
// property is checked on each plain `go test`, not only when fuzzing.
func TestNoTokenLeakMatrix(t *testing.T) {
	for mode := uint8(0); mode < 8; mode++ {
		for form := uint8(0); form < 3; form++ {
			for _, cmd := range [][]string{{"get", "A1"}, {"create", "t", "--priority", "2"}} {
				name := fmt.Sprintf("mode%d/form%d/%s", mode, form, cmd[0])
				t.Run(name, func(t *testing.T) {
					runOnce(t, form, mode, "matrix", false, cmd)
					if form != 0 {
						runOnce(t, form, mode, "matrix", true, cmd)
					}
				})
			}
		}
	}
}

// TestTokenNeverFormatted covers the in-process surfaces: a Token under every
// verb, JSON and text marshaling, and the errors and traces of a direct
// httpx client.
func TestTokenNeverFormatted(t *testing.T) {
	const v = "cny-direct-0123456789"
	tok := auth.NewToken(v)
	for _, verb := range []string{"%v", "%+v", "%#v", "%s", "%q", "%x", "%X", "%d", "%T"} {
		if s := fmt.Sprintf(verb, tok); strings.Contains(s, v) || strings.Contains(s, fmt.Sprintf("%x", v)) {
			t.Errorf("%s leaks: %s", verb, s)
		}
	}
	b, _ := json.Marshal(struct{ T auth.Token }{tok})
	if strings.Contains(string(b), v) {
		t.Errorf("json leaks: %s", b)
	}
	b, _ = json.Marshal(map[string]any{"t": &tok, "p": []any{tok}})
	if strings.Contains(string(b), v) {
		t.Errorf("json leaks: %s", b)
	}
}

func TestDirectClientErrorsAndTraceCarryNoToken(t *testing.T) {
	for mode := uint8(1); mode < 8; mode++ {
		d := &canaryDaemon{base: "cny-direct-abcdef123456"}
		src, _ := auth.NewDaemonTokenSource(d, "p")
		var trace strings.Builder
		clk := clock.NewFake(clockStart())
		clk.SetAutoAdvance(true)
		srv := httptest.NewServer(hostile(mode))
		c := httpx.NewClient(httpx.Config{
			Clock: clk, Refresher: auth.NewAuthorizer(src), Trace: &trace,
			VendorCode: func(h http.Header) string { return h.Get("X-Vendor-Code") },
		})
		resp, err := c.Get(srv.URL + "/x")
		if resp != nil {
			_ = resp.Body.Close()
		}
		srv.Close()
		surfaces := map[string]string{"trace": trace.String()}
		if err != nil {
			surfaces["error"] = err.Error()
			surfaces["error %+v"] = fmt.Sprintf("%+v", err)
			surfaces["error %#v"] = fmt.Sprintf("%#v", err)
		}
		checkNoLeak(t, d.tokens, surfaces)
	}
}

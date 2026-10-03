package auth_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/stainedhead/agent-cli-core/auth"
	"github.com/stainedhead/agent-cli-core/output"
)

func TestSentinelsAreAuthCategory(t *testing.T) {
	for _, err := range []error{auth.ErrReauthRequired, auth.ErrRevoked, &auth.UnreachableError{Socket: "/s"},
		&auth.ActionRequiredError{Provider: "p", Err: auth.ErrRevoked}, &auth.TokenError{Provider: "p", Op: "fetch", Err: errors.New("x")}} {
		if output.CategoryOf(err) != output.CategoryAuth || output.ExitOf(err) != output.ExitAuth {
			t.Errorf("%T: category %v exit %v", err, output.CategoryOf(err), output.ExitOf(err))
		}
		if output.ExitOf(fmt.Errorf("wrapped: %w", err)) != 3 {
			t.Errorf("%T wrapped is not exit 3", err)
		}
	}
}

func TestUnreachableNamesSocket(t *testing.T) {
	err := &auth.UnreachableError{Socket: "/run/x.sock", Err: errors.New("dial: no such file")}
	msg := err.Error()
	if !strings.Contains(msg, "/run/x.sock") || !strings.Contains(msg, "may not be running") {
		t.Fatalf("msg %q", msg)
	}
	if !strings.Contains(err.Hint(), "/run/x.sock") {
		t.Fatalf("hint %q", err.Hint())
	}
	if !errors.Is(err, err.Err) {
		t.Fatal("Unwrap")
	}
	if (&auth.UnreachableError{}).Error() == "" {
		t.Fatal("empty message")
	}
	env := output.FromError(err)
	if env.Error.Code != output.CategoryAuth || !strings.Contains(env.Error.Message, "/run/x.sock") {
		t.Fatalf("%+v", env.Error)
	}
}

func TestActionRequired(t *testing.T) {
	re := &auth.ActionRequiredError{Provider: "prov", Err: auth.ErrReauthRequired, Remediation: "run `tool enroll prov`"}
	if !errors.Is(re, auth.ErrReauthRequired) || errors.Is(re, auth.ErrRevoked) {
		t.Fatal("Is")
	}
	if !strings.Contains(re.Error(), "prov") || !strings.Contains(re.Hint(), "run `tool enroll prov`") ||
		!strings.Contains(re.Hint(), "human") {
		t.Fatalf("%q / %q", re.Error(), re.Hint())
	}
	bare := &auth.ActionRequiredError{Provider: "prov", Err: auth.ErrRevoked}
	if !strings.Contains(bare.Hint(), "human") || strings.Contains(bare.Hint(), "  ") {
		t.Fatalf("hint %q", bare.Hint())
	}
	if env := output.FromError(re); env.Error.Hint != re.Hint() {
		t.Fatalf("hint not surfaced: %+v", env.Error)
	}
	if (&auth.ActionRequiredError{}).Error() == "" {
		t.Fatal("empty")
	}
}

func TestTokenError(t *testing.T) {
	inner := errors.New("boom")
	e := &auth.TokenError{Provider: "p", Op: "fetch", Err: inner}
	if !errors.Is(e, inner) || !strings.Contains(e.Error(), "p") || !strings.Contains(e.Error(), "fetch") || e.Hint() == "" {
		t.Fatalf("%q", e.Error())
	}
	// Secrets inside underlying messages are scrubbed.
	leak := &auth.TokenError{Provider: "p", Op: "fetch", Err: errors.New("bad Bearer abcdef0123456789xyz")}
	if strings.Contains(leak.Error(), "abcdef0123456789xyz") {
		t.Fatalf("leak: %s", leak.Error())
	}
	if (&auth.TokenError{Provider: "p", Op: "fetch"}).Error() == "" {
		t.Fatal("nil Err")
	}
}

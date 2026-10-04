package auth

import (
	"errors"
	"fmt"
	"strings"

	"github.com/stainedhead/agent-cli-core/internal/redact"
	"github.com/stainedhead/agent-cli-core/output"
)

// sentinel is a fixed auth failure in category auth (exit code 3).
type sentinel struct{ msg string }

func (s *sentinel) Error() string             { return s.msg }
func (s *sentinel) Category() output.Category { return output.CategoryAuth }

// The failures a DaemonClient reports. Test for them with errors.Is.
var (
	// ErrReauthRequired means the daemon cannot obtain a token without a
	// human action (re-enrollment).
	ErrReauthRequired error = &sentinel{"reauth_required: the credential needs a human action"}
	// ErrRevoked means the credential was revoked and needs a human action.
	ErrRevoked error = &sentinel{"revoked: the credential was revoked"}
	// ErrRefreshUnsupported means a TokenSource cannot force a refresh (it
	// does not implement Refresher).
	ErrRefreshUnsupported error = &sentinel{"refresh unsupported: the token source cannot force a refresh"}
)

// UnreachableError means the credential daemon could not be reached on
// Socket. Nothing else is tried: there are no fallback credentials. Its
// category is auth, so the exit code is 3.
type UnreachableError struct {
	// Socket is the socket path that was tried.
	Socket string
	// Err is the underlying dial error, if any.
	Err error
}

// Error names the socket and says the service may not be running.
func (e *UnreachableError) Error() string {
	msg := fmt.Sprintf("credential daemon unreachable at socket %q: the agent-okta-d service may not be running", e.Socket)
	if e.Err != nil {
		msg += ": " + scrub(e.Err.Error())
	}
	return msg
}

// Hint tells the caller what to check.
func (e *UnreachableError) Hint() string {
	return fmt.Sprintf("Check that the credential daemon is running and listening on %q. No fallback credentials are used.", e.Socket)
}

// Unwrap returns the underlying error.
func (e *UnreachableError) Unwrap() error { return e.Err }

// Category returns output.CategoryAuth.
func (*UnreachableError) Category() output.Category { return output.CategoryAuth }

// ActionRequiredError reports reauth_required or revoked for a provider. Err
// is ErrReauthRequired or ErrRevoked. The message is generic; Remediation is
// the exact, tool-supplied instruction appended to the hint.
type ActionRequiredError struct {
	Provider    string
	Err         error
	Remediation string
}

// Error describes the failure without any remediation.
func (e *ActionRequiredError) Error() string {
	reason := "reauth_required"
	if errors.Is(e.Err, ErrRevoked) {
		reason = "revoked"
	}
	return fmt.Sprintf("provider %q: %s: a human action is needed", e.Provider, reason)
}

// Hint is the generic human-action message followed by Remediation.
func (e *ActionRequiredError) Hint() string {
	h := "A human action is needed: the credential must be re-enrolled."
	if r := strings.TrimSpace(e.Remediation); r != "" {
		h += " " + r
	}
	return h
}

// Unwrap returns ErrReauthRequired or ErrRevoked.
func (e *ActionRequiredError) Unwrap() error { return e.Err }

// Category returns output.CategoryAuth.
func (*ActionRequiredError) Category() output.Category { return output.CategoryAuth }

// TokenError is any other failure to obtain a token. Its message is scrubbed
// of secrets. Category is auth.
type TokenError struct {
	Provider string
	// Op is "fetch" or "refresh".
	Op  string
	Err error
}

// Error returns the provider, the operation and the scrubbed cause.
func (e *TokenError) Error() string {
	msg := fmt.Sprintf("provider %q: could not %s a token", e.Provider, e.Op)
	if e.Err != nil {
		msg += ": " + scrub(e.Err.Error())
	}
	return msg
}

// Hint says no fallback is tried.
func (*TokenError) Hint() string {
	return "No fallback credentials are used. Check the credential daemon and retry."
}

// Unwrap returns the underlying error.
func (e *TokenError) Unwrap() error { return e.Err }

// Category returns output.CategoryAuth.
func (*TokenError) Category() output.Category { return output.CategoryAuth }

func scrub(s string) string { return redact.New().String(s) }

// categorizedError passes an error that already has a category through
// DaemonTokenSource. The category, the hint and errors.As/Is on the cause all
// work as on the original; the message and hint are scrubbed of secrets, the
// same guarantee TokenError gives.
type categorizedError struct{ err error }

func (e *categorizedError) Error() string { return scrub(e.err.Error()) }

func (e *categorizedError) Unwrap() error { return e.err }

func (e *categorizedError) Category() output.Category { return output.CategoryOf(e.err) }

func (e *categorizedError) Hint() string {
	var h output.Hinter
	if errors.As(e.err, &h) {
		return scrub(h.Hint())
	}
	return ""
}

package httpx

import (
	"errors"
	"fmt"
	"net/url"
	"strconv"

	"github.com/stainedhead/agent-cli-core/internal/redact"
	"github.com/stainedhead/agent-cli-core/output"
)

// maxVendorCode bounds the length of ForbiddenError.VendorCode so a hostile
// or buggy server cannot inflate an error message.
const maxVendorCode = 64

// RateLimitedError reports that a request was rate limited or hit a transient
// failure (429, 502, 503, 504 or a network error) and was not satisfied
// within the retry bounds, or was not eligible for retry. It maps to exit
// code 8. It never carries a response body.
type RateLimitedError struct {
	// Status is the last HTTP status, or 0 when the last failure was a
	// transport error.
	Status int
	// Attempts is how many times the request was sent.
	Attempts int
	// Err is the underlying transport error, if any. Its message is never
	// reproduced verbatim by Error: the text is scrubbed first.
	Err error

	// errText is the scrubbed message of Err, set by the Transport. When
	// empty, Error scrubs Err.Error() with the built-in patterns.
	errText string
}

// Error describes the failure without any response content.
func (e *RateLimitedError) Error() string {
	what := "transport error"
	if e.Status != 0 {
		what = "HTTP " + strconv.Itoa(e.Status)
	}
	msg := fmt.Sprintf("rate limited or transient failure persisted (%s) after %d attempt(s)", what, e.Attempts)
	if e.errText != "" {
		msg += ": " + e.errText
	} else if e.Err != nil {
		msg += ": " + redact.New().String(e.Err.Error())
	}
	return msg
}

// Unwrap returns the underlying transport error, if any.
func (e *RateLimitedError) Unwrap() error { return e.Err }

// Category implements output.CategoryError.
func (e *RateLimitedError) Category() output.Category { return output.CategoryRateLimited }

// Hint implements output.Hinter.
func (e *RateLimitedError) Hint() string {
	return "wait and retry later; do not loop on this command"
}

// AuthError reports an authentication failure: a 401 that a single token
// refresh did not cure (or could not be attempted), or a failed refresh. It
// maps to exit code 3.
type AuthError struct {
	// Err is the refresh failure, if the refresh itself failed.
	Err error
}

// Error describes the failure without any response content.
func (e *AuthError) Error() string {
	if e.Err != nil {
		return "authentication failed: token refresh failed: " + e.Err.Error()
	}
	return "authentication failed: the server rejected the credentials (HTTP 401)"
}

// Unwrap returns the refresh failure, if any.
func (e *AuthError) Unwrap() error { return e.Err }

// Category implements output.CategoryError.
func (e *AuthError) Category() output.Category { return output.CategoryAuth }

// Hint implements output.Hinter.
func (e *AuthError) Hint() string {
	return "sign in again with the tool's authentication command, then retry"
}

// ForbiddenError reports a 403. It carries the vendor error code when the
// tool's Config.VendorCode extractor found one, and never the response body.
// It maps to exit code 4.
type ForbiddenError struct {
	// VendorCode is the vendor-specific error code, possibly empty.
	VendorCode string
}

// Error describes the refusal without any response content.
func (e *ForbiddenError) Error() string {
	if e.VendorCode != "" {
		return "forbidden: the server refused the action (HTTP 403, code " + e.VendorCode + ")"
	}
	return "forbidden: the server refused the action (HTTP 403)"
}

// Category implements output.CategoryError.
func (e *ForbiddenError) Category() output.Category { return output.CategoryForbidden }

// Hint implements output.Hinter.
func (e *ForbiddenError) Hint() string {
	return "the account lacks permission for this action; retrying will not help"
}

// sendError carries a scrubbed message for a transport error while keeping the
// original in the chain for errors.Is and errors.As.
type sendError struct {
	msg string
	err error
}

func (e *sendError) Error() string { return e.msg }
func (e *sendError) Unwrap() error { return e.err }

// scrubSendText renders a transport error as text that is safe to show. The
// query string and userinfo of any URL in a *url.Error are dropped (the host
// and path stay), then the configured redactor and every credential this call
// attached are applied.
func (t *Transport) scrubSendText(err error, held []string) string {
	var ue *url.Error
	if errors.As(err, &ue) {
		inner := ue.Err
		if inner == nil {
			inner = errors.New("error")
		}
		return t.scrub(ue.Op+" "+stripURLSecrets(ue.URL)+": "+inner.Error(), held)
	}
	return t.scrub(err.Error(), held)
}

// stripURLSecrets removes userinfo, query and fragment from raw. If raw does
// not parse it is returned unchanged for the redactor to handle.
func stripURLSecrets(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	u.User, u.RawQuery, u.Fragment, u.ForceQuery = nil, "", "", false
	return u.String()
}

// scrubbedSendError wraps err so its message is scrubbed but the chain stays
// inspectable.
func (t *Transport) scrubbedSendError(err error, held []string) error {
	return &sendError{msg: t.scrubSendText(err, held), err: err}
}

// rateLimitedSend builds the RateLimitedError for an exhausted send failure.
func (t *Transport) rateLimitedSend(attempts int, err error, held []string) *RateLimitedError {
	return &RateLimitedError{Attempts: attempts, Err: err, errText: t.scrubSendText(err, held)}
}

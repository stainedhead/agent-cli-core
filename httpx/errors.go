package httpx

import (
	"fmt"
	"strconv"

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
	// Err is the underlying transport error, if any.
	Err error
}

// Error describes the failure without any response content.
func (e *RateLimitedError) Error() string {
	what := "transport error"
	if e.Status != 0 {
		what = "HTTP " + strconv.Itoa(e.Status)
	}
	msg := fmt.Sprintf("rate limited or transient failure persisted (%s) after %d attempt(s)", what, e.Attempts)
	if e.Err != nil {
		msg += ": " + e.Err.Error()
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

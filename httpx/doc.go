// Package httpx provides a retrying HTTP transport with Retry-After handling,
// jitter, idempotency rules, a single token refresh on 401, and redacted
// tracing.
//
// # Frozen contract: TokenRefresher
//
// httpx and auth never import each other. They meet through this interface,
// which an auth type satisfies structurally:
//
//	type TokenRefresher interface {
//	    // Authorize sets the Authorization header on req itself, so the token
//	    // never leaves the implementation except into that header.
//	    Authorize(ctx context.Context, req *http.Request) error
//	    // Refresh forces one token refresh.
//	    Refresh(ctx context.Context) error
//	}
//
// The signature is frozen: changing it is a breaking change.
//
// # Overview
//
// NewClient and NewTransport return an http.Client / http.RoundTripper that
// retries transient failures (429, 502, 503, 504, network errors) with
// jittered exponential backoff, honors Retry-After for 429 and 503 up to
// Config.MaxWait, retries only idempotent requests or those marked with
// MarkSafeToRetry (replaying bodies through GetBody), performs one token
// refresh and resend on 401 from the same attempt budget, and converts
// terminal failures into typed errors that map to exit codes through
// output.CategoryError: *RateLimitedError (8), *AuthError (3) and
// *ForbiddenError (4). Error values never contain response bodies. Tracing
// is off unless Config.Trace is set and is always redacted.
package httpx

import (
	"context"
	"net/http"
)

// TokenRefresher authorizes outgoing requests and can force one token
// refresh. It is the only coupling between httpx and the auth package.
type TokenRefresher interface {
	// Authorize sets the Authorization header on req itself.
	Authorize(ctx context.Context, req *http.Request) error
	// Refresh forces one token refresh.
	Refresh(ctx context.Context) error
}

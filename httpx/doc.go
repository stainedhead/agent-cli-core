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
// This package is a stub until its workstream lands; TokenRefresher is
// declared below so consumers can already depend on it.
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

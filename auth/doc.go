// Package auth obtains short-lived access tokens for a CLI built on
// agent-cli-core.
//
// It defines its own small DaemonClient and TokenSource interfaces, a
// redacting Token type that never prints its value, and typed errors that map
// to exit code 3 through output.CategoryError. It never imports httpx: the two
// packages meet only through the structural httpx.TokenRefresher interface,
//
//	interface {
//	    Authorize(ctx context.Context, req *http.Request) error
//	    Refresh(ctx context.Context) error
//	}
//
// which an auth type satisfies by having exactly those methods. Authorize sets
// the Authorization header itself, so a token never leaves auth except into
// that header.
//
// This package is a stub until its workstream lands.
package auth

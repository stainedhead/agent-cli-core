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
// which an *Authorizer satisfies by having exactly those methods. Authorize
// sets the Authorization header itself, so a token never leaves auth except
// into that header. No exported function returns a token as text.
//
// # Redaction limits
//
// Token redacts under every fmt verb (including inside unexported fields),
// JSON, text marshaling and log/slog. It cannot defend against code that uses
// reflect or unsafe in-process, or against process memory dumps and core
// files; the value lives in ordinary heap memory for the life of the Token.
//
// # Wiring
//
// A tool builds a DaemonClient, wraps it with NewDaemonTokenSource for one
// named provider (the library hard-codes none and falls back to nothing), and
// hands NewAuthorizer(source) to the HTTP layer. Pass WithRemediation to add
// the tool's own re-enrollment instruction to the generic reauth message.
//
// # Deferred: the daemon adapter and human mode
//
// The adapter over the credential daemon's pkg/client is not part of this
// module yet, because that module has no tagged release and its Go surface is
// unconfirmed. When it exists it is the only importer of pkg/client and
// implements DaemonClient, mapping the daemon's reauth, revoked and
// socket-unreachable signals to ErrReauthRequired, ErrRevoked and
// *UnreachableError; nothing else here changes. Human-mode token sources
// (browser PKCE login, OS keychain) are also out of scope: a tool that needs
// one supplies its own TokenSource (and, for forced refresh, Refresher).
package auth

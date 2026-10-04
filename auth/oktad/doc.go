// Package oktad adapts the credential daemon's Go client
// (github.com/stainedhead/agent-okta-d/pkg/client) to auth.DaemonClient. It
// is the only package of this module that imports that client; auth does not
// import it.
//
//	c := oktad.New() // socket from AGENT_OKTA_D_SOCKET or the platform default
//	defer c.Close()
//	src, err := auth.NewDaemonTokenSource(c, "graph")
//
// Fetch and Refresh hand the daemon's access token to auth.NewToken at one
// place; the token is never formatted, logged or put in an error.
//
// # Error mapping
//
// Failures are classified in this order (errors.Is and errors.As, so wrapped
// errors classify the same way):
//
//	cause                                            result                    category      exit
//	caller's context cancelled or past its deadline  wrapped context error     general       1
//	daemon socket unreachable or timed out           *auth.UnreachableError    auth          3
//	reauth_required (401)                            auth.ErrReauthRequired    auth          3
//	revoked (403)                                    auth.ErrRevoked           auth          3
//	degraded (503), or any answer with a retry hint  *TransientError           rate_limited  8
//	not_configured (404), unauthorized (403)         *AccessError              auth          3
//	invalid response, empty provider, other          plain wrapped error       general       1
//
// A cancelled or expired caller context is reported as such and never as an
// unreachable daemon. Exit 3 rather than 4 for not_configured and unauthorized:
// exit 4 means a remote server refused a request, whereas these are problems
// with the local credential setup that a human fixes.
//
// # Retry
//
// A *TransientError is the retryable-transient form of exit 8. Its
// RetryAfter method returns the daemon's hint and its Hint text names the
// wait, so the failure envelope's error.hint carries it. The envelope has no
// separate retry_after field; callers that need the number use
// errors.As(err, &te) and te.RetryAfter(), or the structural interface
// interface{ RetryAfter() time.Duration }.
package oktad

# auth/oktad: the credential daemon adapter

`auth/oktad` is a ready-made `auth.DaemonClient` for the `agent-okta-d` credential daemon. It talks to the daemon's local unix socket through the daemon's own Go client and hands the token to `auth` without ever printing it. It is new in v0.2.0 (unreleased; the tag is pending) and requires `github.com/stainedhead/agent-okta-d` v0.1.0, which your module then also depends on.

## Wire it up

```go
import (
	"github.com/stainedhead/agent-cli-core/auth"
	"github.com/stainedhead/agent-cli-core/auth/oktad"
	"github.com/stainedhead/agent-cli-core/httpx"
)

c := oktad.New() // socket from AGENT_OKTA_D_SOCKET, else the platform default
defer c.Close()

src, err := auth.NewDaemonTokenSource(c, "my-provider",
	auth.WithRemediation("run `mytool enroll my-provider`"))
if err != nil { /* provider name was empty */ }
client := httpx.NewClient(httpx.Config{Refresher: auth.NewAuthorizer(src)})
```

The provider name is yours; the adapter has no default and uses no fallback credentials. An empty provider name is an error and no socket call is made.

## Options

| Option | Meaning |
|---|---|
| `oktad.WithSocketPath(path)` | Daemon socket. Overrides the `AGENT_OKTA_D_SOCKET` environment variable and the platform default. An empty path is ignored |
| `oktad.WithTimeout(d)` | Per-request timeout. Zero or less is ignored and the daemon client's default applies |

Without `WithSocketPath` the socket is `AGENT_OKTA_D_SOCKET` if set, otherwise the daemon client's platform default (`/var/run/agentd/agentd.sock` on macOS, `/run/agentd/agentd.sock` on Linux). `c.SocketPath()` returns the path in use. `c.Close()` releases idle connections, is idempotent, and leaves the client usable. The `Client` is safe for concurrent use; the zero value is not usable (its methods return an error).

`c.Fetch(ctx, provider)` returns the current token and `c.Refresh(ctx, provider)` asks the daemon for a new one; `auth.DaemonTokenSource` calls them for you.

## Errors and exit codes

Failures land in the existing categories; there is no new exit code.

| Situation | What you get | Exit |
|---|---|---|
| Your context was cancelled or its deadline passed | the context error, wrapped (never reported as an unreachable daemon) | 1 |
| Daemon socket unreachable or timed out | `*auth.UnreachableError` naming the socket | 3 |
| Daemon says re-enrollment is needed (`reauth_required`) | `errors.Is(err, auth.ErrReauthRequired)`; through `DaemonTokenSource` it is an `*auth.ActionRequiredError` with your remediation text | 3 |
| Credential revoked | `errors.Is(err, auth.ErrRevoked)`, same | 3 |
| Daemon degraded, or any answer with a retry hint | `*oktad.TransientError` | 8 |
| Provider not configured in the daemon, or this caller not authorized | `*oktad.AccessError` | 3 |
| Invalid response, other errors | plain wrapped error | 1 |

Not-configured and unauthorized are exit 3, not 4: they are local setup problems that a human fixes, whereas exit 4 means a remote server refused a request.

### Retry hints

`*oktad.TransientError` is the retryable form of exit 8. Its error hint (the `hint` field of the failure envelope) names the wait in whole seconds, rounded up. The envelope has no separate retry field, so to get the number use:

```go
var te *oktad.TransientError
if errors.As(err, &te) {
	wait := te.RetryAfter() // zero if the daemon gave no hint
	_ = wait
}
```

or the structural interface `interface{ RetryAfter() time.Duration }`. The daemon client's own errors stay reachable with `errors.Is`/`errors.As` through the cause chain.

## Testing

You do not need a running daemon to test your CLI: use `auth/authtest` (see [auth](auth.md)). Tests of the adapter itself use the fake daemon server shipped in the `agent-okta-d` module. This guide describes behavior checked against that fake; it has not been verified against a live daemon.

## Troubleshooting

- Exit 3, "unreachable at socket ...": the daemon is not running, or the socket path is wrong. Check `AGENT_OKTA_D_SOCKET` or pass `WithSocketPath`.
- Exit 3, "provider ... not configured in the credential daemon": the provider name does not match one configured and enabled in the daemon.
- Exit 3, "not authorized to use the credential daemon": ask the daemon's administrator to authorize this agent.
- Exit 8 with "Retry after N seconds": the daemon is degraded; wait and retry.

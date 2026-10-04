# auth

Token acquisition for your CLI. You give it a way to reach the credential daemon and a provider name; it gives your HTTP layer something that authorizes requests. Tokens never print.

## Wire it up

```go
daemon := oktad.New()                         // implements auth.DaemonClient; see oktad.md
defer daemon.Close()
src, err := auth.NewDaemonTokenSource(daemon, "my-provider",
    auth.WithRemediation("run `mytool enroll my-provider`"))
authz := auth.NewAuthorizer(src)              // pass to the HTTP layer
```

- `auth/oktad` is the adapter for the real credential daemon; see [oktad](oktad.md). Any other type with `Fetch` and `Refresh` also works.
- The provider name is yours; the library has no default and no fallback credentials.
- `authz` has `Authorize(ctx, *http.Request) error` and `Refresh(ctx) error`, the shape the `httpx` package expects, so it plugs in with no glue. `Authorize` sets the `Authorization` header itself; nothing in the library returns or prints a token, and there is no `print-token` helper.
- `auth.Token` prints as `[redacted]` under `%v`, `%+v`, `%#v`, `%s`, JSON, text marshaling and `log/slog`, including when a Token sits in an unexported struct field. Limits: in-process `reflect`/`unsafe` access and memory dumps can still read the value.

## Errors and exit codes

All map to exit code 3 (category `auth`) through `output`:

| Error | Meaning | Message |
|---|---|---|
| `*auth.UnreachableError` | daemon socket not reachable | names the socket and says the service may not be running |
| `*auth.ActionRequiredError` (`errors.Is` `auth.ErrReauthRequired` / `auth.ErrRevoked`) | a human must re-enroll | generic text, plus your `WithRemediation` text in the hint |
| `*auth.TokenError` | any other failure to get a token | provider, operation, scrubbed cause |

Nothing else is tried after a failure.

The adapter `auth/oktad` adds two more error types, `*oktad.TransientError` (exit 8, with a retry hint) and `*oktad.AccessError` (exit 3); see [oktad](oktad.md).

## Testing your CLI

`auth/authtest` gives a fake daemon and fake resource server:

```go
f := authtest.New(authtest.UnauthorizedThenSuccess)
srv := httptest.NewServer(f.Handler())
```

Scenarios: `Valid`, `ExpiredNeedsRefresh`, `ReauthRequired`, `Revoked`, `Unreachable` (use `WithSocket`), `UnauthorizedThenSuccess`, `UnauthorizedTwice`. No real network or credentials are involved.

## Not included yet

- Human-mode login (browser PKCE, OS keychain). Supply your own `auth.TokenSource`; add `Refresh(ctx) (auth.Token, error)` if you want forced refresh.

## Troubleshooting

- Exit 3 and "unreachable at socket ...": start the daemon or fix the socket path (see [oktad](oktad.md)).
- Exit 3 and "human action is needed": run the remediation command shown in the hint.
- `ErrRefreshUnsupported`: your custom source needs a `Refresh` method.

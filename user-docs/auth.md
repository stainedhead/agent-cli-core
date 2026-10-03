# auth

Token acquisition for your CLI. You give it a way to reach the credential daemon and a provider name; it gives your HTTP layer something that authorizes requests. Tokens never print.

## Wire it up

```go
daemon := myAdapter{}                         // implements auth.DaemonClient
src, err := auth.NewDaemonTokenSource(daemon, "my-provider",
    auth.WithRemediation("run `mytool enroll my-provider`"))
authz := auth.NewAuthorizer(src)              // pass to the HTTP layer
```

- The provider name is yours; the library has no default and no fallback credentials.
- `authz` has `Authorize(ctx, *http.Request) error` and `Refresh(ctx) error`, the shape the `httpx` package expects, so it plugs in with no glue. `Authorize` sets the `Authorization` header itself; nothing in the library returns or prints a token, and there is no `print-token` helper.
- `auth.Token` prints as `[redacted]` under `%v`, `%+v`, `%#v`, `%s`, JSON, text marshaling and `log/slog`.

## Errors and exit codes

All map to exit code 3 (category `auth`) through `output`:

| Error | Meaning | Message |
|---|---|---|
| `*auth.UnreachableError` | daemon socket not reachable | names the socket and says the service may not be running |
| `*auth.ActionRequiredError` (`errors.Is` `auth.ErrReauthRequired` / `auth.ErrRevoked`) | a human must re-enroll | generic text, plus your `WithRemediation` text in the hint |
| `*auth.TokenError` | any other failure to get a token | provider, operation, scrubbed cause |

Nothing else is tried after a failure.

## Testing your CLI

`auth/authtest` gives a fake daemon and fake resource server:

```go
f := authtest.New(authtest.UnauthorizedThenSuccess)
srv := httptest.NewServer(f.Handler())
```

Scenarios: `Valid`, `ExpiredNeedsRefresh`, `ReauthRequired`, `Revoked`, `Unreachable` (use `WithSocket`), `UnauthorizedThenSuccess`, `UnauthorizedTwice`. No real network or credentials are involved.

## Not included yet

- The adapter over the credential daemon's own Go client. It is deferred until that module has a tagged release. When it exists it implements `auth.DaemonClient`; until then write a small adapter of your own or use the fake in tests.
- Human-mode login (browser PKCE, OS keychain). Supply your own `auth.TokenSource`; add `Refresh(ctx) (auth.Token, error)` if you want forced refresh.

## Troubleshooting

- Exit 3 and "unreachable at socket ...": start the daemon or fix the socket path.
- Exit 3 and "human action is needed": run the remediation command shown in the hint.
- `ErrRefreshUnsupported`: your custom source needs a `Refresh` method.

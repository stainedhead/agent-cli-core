# ADR fragment (WS-A): adapter error mapping for auth/oktad

Status: proposed (D-A1, D-A2, D-A3, D-A4 of specs/261003-core-v0-2-daemon-adapter/spec.md).

## Context
`auth/oktad` implements `auth.DaemonClient` over `agent-okta-d` v0.1.0 `pkg/client`. Its errors must land in the core's existing categories and exit codes (`output.CategoryError`), with no new category and no new exit code. Exit 4 (`forbidden`) means a remote server refused a request; exit 8 (`rate_limited`) means "rate limiting or a transient failure" (`output/exit.go`).

## Decision
Classification is evaluated top to bottom with `errors.Is` / `errors.As`:

| # | Condition (agent-okta-d) | Result | Category | Exit |
|---|---|---|---|---|
| 1 | caller's `ctx.Err() != nil` | `fmt.Errorf("oktad: %w", ctx.Err())` (never unreachable) | general | 1 |
| 2 | `ErrDaemonUnavailable` | `*auth.UnreachableError{Socket, Err}` | auth | 3 |
| 3 | `ErrReauthRequired` (401) | `auth.ErrReauthRequired`, cause also wrapped; `DaemonTokenSource` turns it into `*auth.ActionRequiredError` | auth | 3 |
| 4 | `ErrRevoked` (403 revoked) | `auth.ErrRevoked`, same as row 3 | auth | 3 |
| 5 | `ErrDegraded` (503), or any `*APIError` with a retry hint | `*oktad.TransientError` | rate_limited | 8 |
| 6 | `ErrNotConfigured` (404) | `*oktad.AccessError` | auth | 3 |
| 7 | `ErrUnauthorized` (403) | `*oktad.AccessError` | auth | 3 |
| 8 | `ErrInvalidResponse`, other `*APIError` without hint, anything else | `fmt.Errorf("oktad: %w", err)` | general | 1 |
| - | empty provider name, zero-value `Client` | local plain error, no socket call | general | 1 |

Reasons:
- Exit 8 for degraded: the daemon is up but cannot serve now and says when to retry, which is the "transient failure" meaning of exit 8. `*TransientError` is adapter-local so `auth` need not import `httpx`.
- Exit 3 (not 4) for not-configured and unauthorized: both are local credential-setup problems a human fixes, like reauth and revoked; exit 4 is reserved for a server 403 on the vendor API. A bare 403 stays fail-closed (`ErrUnauthorized`).
- Context errors are reported as such: a cancelled caller is not an unreachable daemon.

## Retry-after exposure
`TransientError` has `RetryAfter() time.Duration` (the daemon's hint, zero if none) and a `Hint()` that names the wait in whole seconds (rounded up), so `output.FromError` puts it in `error.hint`. `output.Error` has no `retry_after` field in v0.1.0, and adding one is outside this workstream; consumers read the number with `errors.As` or the structural `interface{ RetryAfter() time.Duration }`. The cause chain keeps `client.ErrDegraded` and `client.RetryAfter(err)` working.

## Deviation from the data dictionary
Spec/data-dictionary list `TransientError{RetryAfter time.Duration; Err error}` together with a `RetryAfter()` accessor; Go cannot have both a field and a method of that name. The field is named `Wait` and the method is `RetryAfter()`. Orchestrator: update data-dictionary.md accordingly.

## New exported API (WS-A)
`oktad.Client`, `oktad.New`, `oktad.Option`, `oktad.WithSocketPath`, `oktad.WithTimeout`, `(*Client).Fetch/Refresh/SocketPath/Close`, `oktad.TransientError` (+`RetryAfter`, `Hint`, `Category`, `Unwrap`, `Error`), `oktad.AccessError` (+`Hint`, `Category`, `Unwrap`, `Error`).

## Architecture test
`internal/archtest` gains `Rules.VendorExempt` (internal, test support): `auth/oktad` is exempt from the vendor-name rule (directory name, identifiers and the `agent-okta-d` import path), is allowed to import `auth` and `output`, and is the only package allowed `github.com/stainedhead/agent-okta-d/pkg/client`. `auth` still cannot import it.

## go.mod
`require github.com/stainedhead/agent-okta-d v0.1.0`, resolved from the Go proxy, no `replace`. Consumers of v0.2.0 gain the module in their graph.

# PRD: agent-cli-core v0.2.0 - daemon adapter and shared-core additions

**Created:** 2026-10-03
**Jira:** N/A
**Status:** Draft
**Feature name:** core-v0-2-daemon-adapter
**Branch:** feat/core-v0.2

## Problem Statement

`agent-cli-core` v0.1.0 defines `auth.DaemonClient` but ships no real implementation: snow, outlook and teams each carry a stub that returns `*auth.UnreachableError` (exit 3), so none can obtain a token from `agent-okta-d`. `agent-okta-d` v0.1.0 now publishes `pkg/client` and a fake daemon (`clienttest`), so the adapter is unblocked (milestone M0a).

In addition, the three CLIs independently worked around the same v0.1.0 gaps (continuation token, bounding objects, importable clock, nested docgen commands, audit extension fields, standalone trusted-file check, vendor error code from the response body). Requests made by at least two of the three CLIs are in scope; the rest are deferred with reasons.

Sources (read-only): `snow-cli/docs/core-change-requests.md` (CR-xx), `outlook-cli/docs/requested-core-changes.md` (outlook #n), `teams-cli/docs/requested-core-changes.md` (teams #n).

The release is ADDITIVE ONLY against the v0.1.0 tag: a consumer pinned to v0.1.0 must still compile and behave the same.

## Goals

- G1: Ship `auth/oktad`, a real `auth.DaemonClient` over `agent-okta-d` `pkg/client`, with a documented error mapping onto the existing core categories and exit codes.
- G2: Implement requests R1..R7 (below) as small additive APIs, each with Go doc comments, tests and a runnable `Example`.
- G3: Prove backward compatibility (API diff vs v0.1.0 shows additions only; downstream CI job green) and record what is deferred and why.

## Non-Goals

- Tagging or releasing v0.2.0 (no tag is created here).
- Breaking changes, renames or behaviour changes to any v0.1.0 exported identifier.
- Deferred requests: snow CR-01 shared write counter; CR-02 required-field rule; CR-03 standalone offset base (unless trivially covered by R2); CR-05 DeniedError as CategoryError; CR-06 settable Redactor; CR-08 5xx classification; CR-09 selftest exit codes/per-row results; CR-11 and outlook 4 / teams 1 cross-process rate-limit state (larger design: injectable counter store; flagged); typed policy rules (core is generic by design); Untrusted JSON shape (outlook 3 / teams 6: wire-format change, golden risk; documented). Recorded in `docs/deferred.md`.
- Human-mode PKCE/keychain TokenSource, signature checks, new CI secrets.

## Functional Requirements

**FR-001 (daemon adapter, M0a):** New package `auth/oktad` implements `auth.DaemonClient` (`Fetch` and `Refresh(ctx, provider) (auth.Token, error)`) over `github.com/stainedhead/agent-okta-d` v0.1.0 `pkg/client`. A compile-time assertion `var _ auth.DaemonClient = (*Client)(nil)` exists. `go.mod` requires the tagged version; no `replace`, no pseudo-version.

**FR-002 (construction):** `oktad.New(opts ...Option)` with a socket-path option (default `client.DefaultSocketPath()`, which honours `AGENT_OKTA_D_SOCKET`) and a timeout option (default `client.DefaultTimeout`).

**FR-003 (token mapping):** `Credential.AccessToken` (`client.Secret`) becomes `auth.NewToken(secret.Reveal())`. The raw value is never logged and never appears in any error, `fmt` output or log of any adapter type.

**FR-004 (error mapping):**

| client error | core result | exit |
|---|---|---|
| `ErrDaemonUnavailable` | `*auth.UnreachableError` carrying `client.SocketPath()` | 3 |
| `ErrReauthRequired` | `auth.ErrReauthRequired` | 3 |
| `ErrRevoked` | `auth.ErrRevoked` | 3 |
| `ErrDegraded`, or `*APIError` with `RetryAfter > 0` | retryable transient error, category rate_limited, exposes the retry-after duration | 8 |
| `ErrNotConfigured`, `ErrUnauthorized` | auth-category error | 3 (vs 4 decided in ADR) |
| `ErrInvalidResponse` | general error | 1 |
| context cancel/deadline | the context error, wrapped, not mapped to unreachable | [TBD: category in ADR] |

Choice of types for the transient and not-configured/unauthorized rows uses existing core categories (no new categories or exit codes) and is recorded in a new ADR.

**FR-005 (R1, output):** `Meta.NextPageToken string` with JSON key `next_page_token`, `omitempty`. (outlook 2/13/18, teams 5)

**FR-006 (R2, output):** Bounds can name an array field inside object data (for example `Bounds.ArrayField`) so `{"items":[...]}` honours `--max-bytes`, trimming that array and setting `Meta.Truncated` and `Meta.NextOffset`. An offset-base for the caller is added only if trivial. Behaviour is unchanged when unset. (snow CR-04/CR-12, outlook 13, teams 6)

**FR-007 (R3, clock):** An exported importable clock: `Clock` interface, `System`, and a manual/fake clock for tests. `audit.WithClock`, `policy` and `httpx` accept it; `internal/clock` stays compatible (existing callers and the internal type still work). (outlook 6/14, teams 7)

**FR-008 (R4, docgen):** `CommandTree` supports nested `Subcommands`, rendered hierarchically and deterministically; flat `Commands` keep working and render as before. (outlook 9, teams 9)

**FR-009 (R5, audit):** `Record.Extra map[string]string` (omitempty), plus `RuleID` and `TargetRef` if clean. Emitted as real JSON keys/columns, redacted and bounded like other fields, never bodies. Old records still parse; schema version handling documented. (outlook 8/17, teams 3, snow CR-07/CR-10)

**FR-010 (R6, policy):** Exported standalone trusted-file check (for example `policy.CheckTrustedFile`) with ownership, not group/world-writable, symlink owner checks, `O_NOFOLLOW` plus `fstat` semantics and an explicit trusted-uid parameter; never the effective uid. `policy.Load` keeps v0.1.0 behaviour. Unix only; fails closed elsewhere. (outlook 5/15, teams 2)

**FR-011 (R7, httpx):** `Config.VendorCodeFromBody` hook receiving a bounded body prefix, and/or error types exposing status and a bounded body prefix. Header-based `VendorCode` keeps working and takes documented precedence. Body prefix is redacted and never copied into audit. (outlook 1/19, teams 4)

**FR-012 (docs):** Update `docs/technical-details.md`, user-docs, new ADRs (adapter error mapping, one per new API), this PRD (milestone M0a done; fix text saying the adapter is deferred, including AGENTS.md, README and INTENT), `CHANGELOG.md` (v0.2.0 Unreleased), `docs/deferred.md`, `docs/api-compat-v0.2.md`. Verification of outlook 10/11/12 and teams 10/11/12 recorded if cheap. Done after implementation (dev-flow step 6).

## Per-requirement testable criteria (added in PRD review)

- R1: marshalling an envelope with `NextPageToken` set emits `meta.next_page_token`; unset emits byte-identical v0.1.0 golden output.
- R2: with `Bounds` naming `items`, an object whose JSON exceeds the byte limit is trimmed to fit, valid UTF-8/JSON, `meta.truncated=true` and `meta.next_offset` = number of retained items; a missing or non-array field returns a documented error (not a panic); unset keeps v0.1.0 behaviour (`ErrBoundTooSmall`).
- R3: a manual clock injected into audit, policy rate limiting and httpx retry drives time with no real sleeping; a v0.1.0-style `clock.Clock` value (internal) still satisfies the option parameter types.
- R4: a two-level tree renders children under parents in deterministic order and the output is byte-identical across runs; a flat-only tree renders byte-identical to v0.1.0.
- R5: a record with `Extra`, `RuleID`, `TargetRef` emits those keys, secrets in values are redacted, value count and size are bounded (limits [TBD] in spec); a record without them is byte-identical to v0.1.0.
- R6: table tests (temp files) for wrong owner, group/world-writable, symlink, symlinked ancestor, trusted-uid match/mismatch, TOCTOU via fstat on the opened descriptor; effective uid is never consulted; `policy.Load` tests unchanged.
- R7: a hook returning a vendor code from a body prefix larger than the bound sees only the bound; 403 error exposes the code; header-only `VendorCode` configs unchanged.

## Non-Functional Requirements

- **Compatibility:** apidiff vs v0.1.0 reports only additions; existing tests unchanged and green; CI `downstream` job (snow/outlook/teams against the PR core) passes.
- **Security:** token never in errors, logs or `fmt` of any type (fuzz/noleak style test extended to `oktad`); no fallback credentials; no new CI secrets (GITHUB_TOKEN only); no bodies in audit or errors.
- **Reliability / portability:** adapter tests use the real client against `clienttest` over a real unix socket under a SHORT temp dir (`os.MkdirTemp("", "ocd")`, macOS 104-byte limit); no macOS-only assumptions; no timing-dependent assertions; `go test -race -count=3 ./...` passes on Linux CI.
- **Quality:** gofmt, go vet, golangci-lint clean; at least 90% coverage on new code; Go doc comments and runnable `Example` on each new API; dependency-rule test (no vendor names) still passes.
- **Observability:** adapter errors carry the socket path (not secrets) and retry-after where known; the adapter emits no log output of its own; audit extension fields make rule id and target visible per record. Performance: no extra goroutines or background work in the adapter; one request per Fetch/Refresh.

## Acceptance Criteria

- [ ] AC1: `auth/oktad` exists, satisfies `auth.DaemonClient` (compile-time assertion), and every mapping row in FR-004 is tested through `clienttest`, including token redaction (token never appears in errors, logs or `fmt` of anything), context cancellation, unreachable socket and retry-after honoured.
- [ ] AC2: R1..R7 are each implemented with tests and an `Example`, none changing existing behaviour or signatures.
- [ ] AC3: Backward compat: apidiff of exported API vs the v0.1.0 tag reports only additions (`go run golang.org/x/exp/cmd/apidiff@latest` or gorelease; otherwise a documented manual symbol diff in `docs/api-compat-v0.2.md`); CI `downstream` job passes; existing tests unchanged and green.
- [ ] AC4: gofmt, go vet, golangci-lint run, `go test -race -count=3 ./...` clean; at least 90% coverage on new code; no new CI secrets.
- [ ] AC5: `docs/deferred.md` lists all deferred requests with reasons; api-compat result recorded; docs updated as in FR-012.

## Requirement-to-CLI Traceability

| Req | snow-cli | outlook-cli | teams-cli |
|---|---|---|---|
| R1 Meta.NextPageToken | - | 2, 13, 18 | 5 |
| R2 bound object array | CR-04, CR-12 (CR-03) | 13 | 6 |
| R3 public clock | - | 6, 14 | 7 |
| R4 nested docgen | - | 9 | 9 |
| R5 audit extension fields | CR-07, CR-10 | 8, 17 | 3 |
| R6 trusted-file check | - | 5, 15 | 2 |
| R7 vendor code from body | - | 1, 16, 19 | 4 |
| Adapter (FR-001..004) | - | 7 | 8 |

## Dependencies and Risks

| Item | Type | Notes |
|---|---|---|
| `agent-okta-d` v0.1.0 tag and `pkg/client` | Dependency | Must be resolvable via go proxy or CI; [TBD] confirm tag published and its Go version vs core `go 1.27` |
| `clienttest` fake daemon | Dependency | Tests only; verify it is importable outside the module (not under `internal/`) |
| Downstream CI job | Dependency | Consumers must compile against the PR core |
| Existing dependency-rule / archtest | Risk | Importing `agent-okta-d` may trip the "no vendor name" test; scope it to `auth/oktad` and document |
| `internal/clock` vs public clock | Risk | Type mismatch could break `audit.WithClock` callers; use interface compatibility, verify with apidiff |
| `audit.Record` new fields | Risk | JSON/column order and golden files; fields must be omitempty so existing goldens are unchanged |
| `Meta` new field | Risk | Envelope golden tests; omitempty keeps them byte-identical |
| Overlap of R3 across audit/policy/httpx | Risk | Keep R3, R5, R6, R7 in one workstream to avoid conflicts |
| Unix socket path length | Risk | Short temp dirs |
| Trusted-file check on non-unix | Risk | Fail closed; [TBD] build tags |
| Exit code for not-configured/unauthorized (3 vs 4) | Risk | Decided in ADR |
| Cross-process rate limit state deferred | Risk | Consumers keep workarounds; flagged in `docs/deferred.md` |

## Open Questions

Recommended defaults (to be confirmed in the spec and ADRs, not yet decisions): not-configured/unauthorized -> auth category (exit 3, hint names the human action); transient -> adapter-local error type implementing `output.CategoryError` with category rate_limited (avoids an `auth` -> `httpx` dependency); cancellation -> wrapped context error, category general; public clock in a new top-level `clock` package with `internal/clock` kept as a thin alias or compatible interface; `RuleID` and `TargetRef` as typed omitempty fields.

- [TBD] Category for `ErrNotConfigured`/`ErrUnauthorized`: auth (3) vs forbidden (4) per core PRD.
- [TBD] Transient error type for `ErrDegraded`/retry-after: new type in `auth/oktad` implementing `output.CategoryError` (rate_limited) vs reuse `httpx.RateLimitedError`; avoid an `auth` -> `httpx` import cycle.
- [TBD] Name and package of the public clock (`clock` package vs existing public package).
- [TBD] Whether `Bounds` offset-base is trivial enough to include in R2 (otherwise defer CR-03).
- [TBD] Whether `RuleID` and `TargetRef` are added as typed fields or only via `Extra`.
- [TBD] Context cancellation mapping.

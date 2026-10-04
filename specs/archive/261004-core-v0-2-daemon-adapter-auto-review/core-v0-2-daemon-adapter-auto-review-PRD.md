# PRD: agent-cli-core v0.2.0 code-review fixes

**Created:** 2026-10-04
**Jira:** N/A
**Status:** Reviewed (step 8)
**Feature name:** core-v0-2-daemon-adapter-auto-review
**Branch:** feat/core-v0.2
**Reviewed spec:** specs/261003-core-v0-2-daemon-adapter (archived in step 9)

## Problem Statement

A step-7 review of the diff `main...feat/core-v0.2` (spec FR-001..FR-012) found the implementation sound: no P0 findings, and no change to any v0.1.0 exported signature or default behaviour. This PRD lists the small number of defects and hardening items that remain. Two are security-adjacent (a documentation overclaim about `CheckTrustedFile`, and audit `Extra` keys that bypass redaction); the rest are robustness and test-hygiene items.

## Goals

- G1: Close the two P1 findings (documentation overclaim, audit key redaction) so the release makes no false security statement.
- G2: Close the P2 robustness and test-hygiene items without any change to a v0.1.0 exported identifier.

## Non-Goals

- New features, API additions beyond those named below, tagging or releasing, and the items already in `docs/deferred.md`.

## Dependencies

- `agent-okta-d` v0.1.0 `pkg/client` and `clienttest` (read-only) for R-P2-1 and R-P2-2.
- A Linux non-root environment (CI matrix or container) to confirm R-P2-4; Docker was unavailable during review.
- `apidiff` (golang.org/x/exp) for the compatibility re-check in the NFRs.

## Open questions

- Q1 (R-P1-2): reject keys whose redacted form differs (proposed, consistent with D-C1) or silently drop them? Owner: maintainer; default is reject.
- Q2 (R-P1-1): is a descriptor-returning `OpenTrustedFile` wanted in v0.3 or never? Owner: maintainer; default is a deferred entry.
- Q3 (R-P2-1): does the client return a context error when the caller context is already cancelled and the socket is dead? Verify in the fix; if not, keep the current precedence and document it.

## Review method and evidence

Read in full: `auth/oktad/oktad.go`, `policy/trust*.go`, `audit/audit.go` diff, `httpx` transport/config diff, `output` (write/envelope/render) diff, `docgen` diff, `clock`, `internal/clock`, `internal/archtest` diff, `go.mod`, `docs/api-compat-v0.2.md`, the ADR text. Compared the error mapping to `agent-okta-d/pkg/client/errors.go` and `client.go` (read-only).

Run (macOS arm64, go1.27.1): `go build`, `go vet`, `golangci-lint run` (0 issues), `gofmt -l` (clean), `go test -race -count=1 ./...` (all pass), `GOOS=linux go vet ./...` and `GOOS=windows go vet ./policy ./auth/...` (clean). Coverage: audit 96.1, auth/oktad 98.3, clock 100, policy 98.3, httpx 96.1, output 94.8, docgen 94.4, archtest 97.2 percent. `grep` shows `Reveal()` is called in exactly one non-test place (`auth/oktad/oktad.go`). Existing test and golden files are unmodified except an additive change to `internal/archtest/archtest_test.go`.

Not verified (be honest): Docker was not running, so the suite was not executed on Linux as a non-root user; `apidiff` is not installed, so `docs/api-compat-v0.2.md` was reviewed, not re-derived.

## Verified as correct (no action)

- Backward compatibility: all additions are new identifiers or `omitempty` fields. `audit.Record.Extra` is a pointer so `Record` stays comparable. `audit.WithClock` keeps a nil-safe, alias-identical parameter. `ArrayField == ""` leaves `Render` on the v0.1.0 path.
- Token handling: `oktad` errors wrap only client errors (`APIError.Error()` has status, code and retry text, no body or token); `Secret` is revealed once; `TransientError`/`AccessError` messages contain no credential. httpx body hook result is trimmed, control-stripped, scrubbed against held `Authorization` values, then cut to 64 bytes (scrub before cut, so a token cannot straddle the cut). No body prefix is stored on any error type.
- Error mapping matches `pkg/client`: `ErrDaemonUnavailable` also covers client-side request timeout, `ErrInvalidResponse` covers an empty `access_token`, and a bare 403 is `ErrUnauthorized`; all route as specified (exit 3 / 8 / 1).
- `CheckTrustedFile` walk logic: symlink hops are bounded (40), every directory on the resolved route including `/` is checked, symlink owners are checked, `O_NOFOLLOW|O_NONBLOCK` plus `fstat` plus `os.SameFile` on the final file, FIFOs rejected before open, effective non-root uid refused as trusted owner, non-unix fails closed.
- Architecture: `auth/oktad` imports only `auth`, `output` and the client; `auth` does not import it; `clock` is a leaf; `policy -> output` is a new but inward-pointing edge, recorded in archtest; the vendor-name exemption is limited to the `auth/oktad` subtree and import allow-lists still apply there.
- Test determinism: `clienttest` sockets are created under a short `os.MkdirTemp("", "aod")`; no sleeps beyond the item in R-P2-3; no network.

## Requirements

### P0

None.

### P1

**R-P1-1: Correct the TOCTOU claim for `CheckTrustedFile`.**
`user-docs/policy.md` (line 89) says the file "is opened without following links and re-checked on the open descriptor, so a swap between check and use fails". The function opens, fstats and closes the descriptor inside the check and returns only an error; the caller then re-opens the path. Only a swap between the walk's `lstat` and the check's own `open` is detected. Safety after the check rests on every ancestor being owned by a trusted uid and not group/world writable, which is a real guarantee but a different one.
Acceptance criteria:
- `user-docs/policy.md`, `docs/technical-details.md` and the `CheckTrustedFile` godoc state precisely what is guaranteed (no untrusted user could have written the file or any directory on its resolved path at check time, and cannot change it afterwards unless a trusted user or root does) and that the function returns no descriptor.
- The statement "a swap between check and use fails" is removed or rewritten; `grep -rn "check and use" user-docs docs policy` finds no overclaim.
- `docs/deferred.md` gains an entry for a descriptor-returning variant (for example `OpenTrustedFile`) that would remove the re-open window entirely, with the reason it was not done in v0.2.
- The same pass notes that POSIX ACLs and extended attributes are not examined.

**R-P1-2: Do not let `audit.Record.Extra` keys bypass redaction.**
`sanitize` redacts values and capitalised fields, but keys are only syntax-checked (`[a-z0-9_.-]{1,32}`). A lowercase hex string of 32 characters, or a literal registered with `audit.WithSecrets`, is a valid key and is written verbatim. A caller bug (using an identifier or secret as a key) therefore leaks, contradicting the "nothing secret reaches the audit line" property.
Acceptance criteria:
- `Log` runs each key through the logger's redactor; if the redacted form differs from the key, the record is rejected with `ErrInvalidExtra` (consistent with decision D-C1) and nothing is written.
- Test: a key equal to a registered `WithSecrets` value, and a key matching a token pattern within the allowed charset, are rejected; the error text does not contain the key.
- Existing valid keys and the golden audit output are unchanged.

### P2

**R-P2-1: Do not let a late context cancellation overwrite a daemon verdict.**
`mapError` tests `ctx.Err() != nil` first, so a reply such as `ErrRevoked` that arrived just before the caller cancelled is reported as a context error (exit 1) instead of exit 3. The spec wording is "ctx.Err() != nil and the error is a context error".
Acceptance criteria:
- The first case becomes `ctx.Err() != nil && (errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded))` (the dead-socket-with-cancelled-context test must still return a context error; verify that the client returns a context error in that case, otherwise keep the precedence and document it).
- Tests: existing cancellation tests pass; a test with a revoked daemon reply and a context that is cancelled by the test after the reply (via a hook in the fake or a wrapper) still yields `auth.ErrRevoked`, or the doc states the precedence explicitly.

**R-P2-2: Defence in depth for an empty token.**
`get` relies on the client rejecting an empty `access_token` (`ErrInvalidResponse`). Spec section 8a requires "never an empty token".
Acceptance criteria:
- After `Reveal()`, an empty value returns a general (exit 1) error `oktad: daemon returned an empty credential`; unit test via a substituted call function.

**R-P2-3: Remove the wall-clock assertion in `clock_test.go`.**
`TestFakeSleepWakesOnAdvance` waits 20 ms on `time.After` to prove the sleeper has not woken. It cannot flake red, but it is a timing-based negative assertion and the only one in the new tests.
Acceptance criteria:
- Replace it with a check that does not depend on real time (for example `f.Waiters() == 1` after the first `Advance`, then the second `Advance` drains it); no `time.After`/`time.Sleep` remains in the new test files.

**R-P2-4: Guarantee `CheckTrustedFile` real-filesystem tests are meaningful on Linux CI.**
`realDir` creates its scratch dir under `$HOME` and silently `t.Skip`s when the home chain is not clean. On a CI runner whose home or its parents are group-writable the whole real-filesystem suite (`TestRealFilesystem`, `TestOSTrustFSOpenStat`) skips and the O_NOFOLLOW/SameFile code is only covered by the fake. (Not reproducible here; Linux run not possible.)
Acceptance criteria:
- A CI log line or `t.Log` states when the real-FS tests are skipped; and `TestOSTrustFSOpenStat` does not depend on a clean home chain (it only needs a writable directory, since `openStat` does not walk ancestors) so O_NOFOLLOW and SameFile are exercised everywhere.
- Test runs non-root on Linux (verify in the CI matrix or in a container).

**R-P2-5: Tighten compatibility wording.**
`docs/api-compat-v0.2.md` opens with "No source-incompatible change", while ADR text and `technical-details.md` correctly say that adding fields breaks unkeyed struct literals of `Meta`, `Bounds`, `Command` and `Record`.
Acceptance criteria:
- The api-compat result line says "additions only; the only source-level caveat is unkeyed composite literals of the four extended structs (none in the repo or in snow/outlook/teams at the time of writing)" and states how that was checked (grep of the three consumers), or drops the claim if it was not checked.
- `CHANGELOG.md` v0.2.0 carries the same caveat.

**R-P2-6: Make docgen sibling ordering match its uniqueness rule.**
Validation compares trimmed names; `writeTree` sorts by the untrimmed `Name` with the unstable `sort.Slice`. Names that differ only by surrounding space can render in an input-dependent order.
Acceptance criteria:
- Sort by `strings.TrimSpace(Name)` (use `sort.SliceStable`); test with `" b"` and `"a"` siblings that the output is the same for both input orders.

## Implementation guidance

- TDD: write the failing test for each requirement first (red), then the fix (green), then refactor.
- Review: one focused code review per fix (`/review-code` on the fix commit) before the next starts; P1 items before P2.
- Parallelism: the requirements touch disjoint areas (docs/policy: R-P1-1, R-P2-4, R-P2-5; audit: R-P1-2; oktad: R-P2-1, R-P2-2; clock and docgen tests: R-P2-3, R-P2-6). They may be given to agent teammates, each in its own git worktree under `.worktrees/`, merged back into `feat/core-v0.2` in any order. `docs/` edits for R-P1-1 and R-P2-5 are by one owner to avoid conflicts.
- No P0 items exist; P1 items are blockers for the PR because they affect a security statement and the redaction guarantee, P2 items are not blockers.

## Non-functional requirements

- No change to any v0.1.0 exported identifier; `apidiff` (or the manual symbol diff) still lists additions only.
- `gofmt -l .` empty, `go vet`, `golangci-lint run`, `go test -race -count=3 ./...` pass; coverage of changed packages stays at 90 percent or more.
- All fixes keep tests deterministic (no real-time sleeps, short socket paths, non-root).

## Out of scope

Tagging, the CI downstream job, and the deferred items already in `docs/deferred.md`.

## Observability and security NFRs

- Security: no change may print or store a token, key or file body in an error or log; new error text must be covered by a leak test that uses a sentinel value.
- Observability: rejection reasons from `Log` (`ErrInvalidExtra`) and `TrustError.Reason` stay actionable and stable enough to match with `errors.Is`.
- Reliability: no new test may depend on wall-clock time, a network, or a particular uid.

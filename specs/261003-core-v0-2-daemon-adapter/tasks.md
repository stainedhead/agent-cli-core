# core-v0-2-daemon-adapter - Tasks

Created: 2026-10-03 | Status: Planning

## Progress Summary
0/26 tasks complete. Workstreams are independent: each runs in its own worktree on a disjoint package set.

## WS-A - auth/oktad + go.mod/go.sum (+ ADR text)
- A1: Confirm `auth.DaemonClient` method set, `auth.NewToken`, `UnreachableError`/`ActionRequiredError`/`TokenError` field names; confirm `agent-okta-d` v0.1.0 resolves from the proxy. Deps: none. AC: notes in implementation-notes.
- A2: `go get github.com/stainedhead/agent-okta-d@v0.1.0`; go.mod/go.sum only; no replace; `go mod tidy` clean. Deps: A1.
- A3: archtest: allow the import for `auth/oktad` only; confirm vendor-name rule. Deps: A2.
- A4: Tests first: construction/options (socket, timeout, env default), token mapping + redaction. Deps: A2.
- A5: Tests first: mapping rows (unavailable, reauth, revoked, degraded, APIError retry-after, not-configured, unauthorized, invalid response), cancellation, retry-after carried, through clienttest. Deps: A4.
- A6: Implement Client, options, Fetch/Refresh, errors (TransientError), compile-time assertion; Example. Deps: A5.
- A7: ADR text for adapter error mapping (as `doc.go` + a text block handed to step 6). Deps: A6.
- A8: Coverage at least 90 percent, race count=3 on `./auth/...`. Deps: A6.

## WS-B - output (R1, R2) + docgen (R4)
- B1: Tests first R1: Meta.NextPageToken marshal + golden unchanged when unset. 
- B2: Implement R1 (+ text/table rendering rule); Example. Deps: B1.
- B3: Tests first R2: ArrayField trimming, Offset, UTF-8/JSON validity, missing/non-array field, too-small, unset == v0.1.0.
- B4: Implement R2; Example; document the offset semantics. Deps: B3.
- B5: Tests first R4: nested rendering, ordering, sibling uniqueness, flat golden unchanged, depth limit.
- B6: Implement R4 (`Command.Subcommands`); Example. Deps: B5.
- B7: Coverage at least 90 percent, race count=3 on `./output/... ./docgen/...`.

## WS-C - clock (R3), audit (R5), policy (R6), httpx (R7)
- C1: New `clock` package (promote from internal/clock) with tests moved/copied; internal/clock aliases; existing internal tests green. archtest entry. Example.
- C2: Verify audit/policy/httpx accept public clock (compile tests, Example with Fake). Deps: C1.
- C3: Resolve D-C4 (Record comparability) by grepping consumers; choose field shape. Deps: none.
- C4: Tests first R5; implement Record.Extra/RuleID/TargetRef, limits, redaction, deterministic order; Example. Deps: C2, C3.
- C5: Tests first R6 (temp files, uid cases, symlink, O_NOFOLLOW/fstat); implement `policy.CheckTrustedFile` (+unix build tags and fail-closed fallback); Example. Deps: none.
- C6: Tests first R7; implement `Config.VendorCodeFromBody` with bounded prefix, precedence with header hook; Example. Deps: C2.
- C7: Coverage at least 90 percent, race count=3 on `./clock/... ./audit/... ./policy/... ./httpx/... ./internal/...`.

## Integration and post-workstream (not in workstreams)
- I1: Merge WS branches; resolve archtest overlap; full gate run.
- I2: apidiff vs v0.1.0 / `docs/api-compat-v0.2.md`; confirm existing goldens/tests untouched.
- I3: Docs (technical-details, user-docs, README, AGENTS, INTENT, PRD M0a done), ADRs, CHANGELOG v0.2.0 Unreleased, `docs/deferred.md` (+ verify outlook 10/11/12, teams 10/11/12), CI `downstream` job.

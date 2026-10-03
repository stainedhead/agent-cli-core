# Tasks - agent-cli-core
Date: 2026-10-03 | Status: Implementation complete except I.4 (docs final pass)

## Progress Summary
38/39 tasks complete (recounted: 11 + 23 + 5; I.4 pending, optional I.6 deferred). Format: ID | owner | depends | est | files | acceptance. Every task is TDD (failing test first) and ends with its package quality gate. Workers touch only the listed files.

## Phase 1 - WS0 Foundation (owner: foundation worker)
- [x] **P1.1** Repo scaffolding. Depends: none. 1h. Files: `go.mod`, `go.sum`, `Makefile`, `.golangci.yml`, `doc.go` stubs for auth, auth/authtest, policy, audit, httpx, selftest, docgen (with frozen `httpx.TokenRefresher`), YAML dep. AC: `go build ./...` ok; three-target cross-build ok.
- [x] **P1.2** `internal/clock` (Clock, Fake). Files: `internal/clock/**`. AC: fake advances deterministically; Example.
- [x] **P1.3** `internal/redact`. Files: `internal/redact/**`. AC: scrubs Authorization/bearer/secret headers, token-looking strings, bodies by default; table + fuzz test.
- [x] **P1.4** `output` exit codes + Category/CategoryError/ExitFor. Files: `output/exit*.go`, goldens. AC: codes 0..9 golden (CORE-OUT-2/8).
- [x] **P1.5** `output` envelope types and writers. Files: `output/envelope*.go`. AC: success/error golden, vendor-neutral text (CORE-OUT-1).
- [x] **P1.6** `output` untrusted marking. AC: JSON `"untrusted": true`, text delimiters with author+timestamp (CORE-OUT-3).
- [x] **P1.7** `output` bounds/truncation. AC: default 32768, `meta.truncated`/`next_offset`, never splits UTF-8 or yields invalid JSON, property test (CORE-OUT-5).
- [x] **P1.8** `output` formats json/table/text + redaction of error message/hint (CORE-OUT-6/7).
- [x] **P1.9** `internal/archtest`: import-graph and no-vendor-name test. AC: fails on a cycle or vendor name in import path/identifier (FR-025).
- [x] **P1.10** Docs skeletons: `docs/technical-details.md` with `## API: <pkg>` sections, `user-docs/<pkg>.md` stubs; `output` section written. AC: skeletons present.
- [x] **P1.11** Output `Example*` tests; gate green; commit/push. Gate for Phase 2.

## Phase 2 - parallel (each after P1.11)
### WS-A auth (`auth/**`, `auth/authtest/**`)
- [x] **A.1** Types: redacting `Token`, `TokenSource`, `DaemonClient`, typed errors incl. `UnreachableError{Socket}` (CategoryError -> auth). AC: String/%v/%+v/GoString print `[redacted]` (CORE-AUTH-1/1a/8).
- [x] **A.2** `authtest.Fake` (valid, expired-needs-refresh, reauth_required, revoked, unreachable, 401-then-success, 401-twice). Depends A.1 (CORE-AUTH-1b).
- [x] **A.3** Daemon-backed `TokenSource` parameterised by provider; no hard-coded provider; no fallback (CORE-AUTH-2/3).
- [x] **A.4** `Authorizer` satisfying `httpx.TokenRefresher` shape; reauth_required message generic + tool-supplied remediation (CORE-AUTH-4/5). AC: messages name socket; exit 3 mapping.
- [x] **A.5** No-token-out tests; API check that nothing returns a token to a user surface (CORE-AUTH-9). Examples. `user-docs/auth.md`, API section; document deferred adapter and human-mode (CORE-AUTH-10/11).

### WS-B policy (`policy/**`)
- [x] **B.1** Strict YAML parser, unknown keys error, invalid -> fail closed (CORE-POL-8).
- [x] **B.2** Verb/resource allow/deny matching, rule ids, decision as data (POL-1/5).
- [x] **B.3** Field allowlists, value constraints, write modes (POL-2/3).
- [x] **B.4** Rate limits with fake clock; max results/bytes caps (POL-2/4).
- [x] **B.5** Warn/refuse if file writable by current user (POL-6, POSIX only; P1). Examples, `user-docs/policy.md`, API section; guardrail-not-control wording (POL-9).

### WS-C audit (`audit/**`)
- [x] **C.1** Versioned `Record` + JSONL writer, golden (AUD-1/5).
- [x] **C.2** No secrets/bodies enforced; redaction pass (AUD-2).
- [x] **C.3** Path from config; write-failure surfacing and blocking mode (AUD-3/4). Examples, `user-docs/audit.md`, API section.

### WS-D httpx (`httpx/**`)
- [x] **D.1** Retrying transport with jitter, injectable clock/sleep (HTTP-1/5).
- [x] **D.2** Retry-After handling for 429/503, bounded, MaxWait ceiling, rate-limited error -> exit 8 (HTTP-2).
- [x] **D.3** Idempotency: only idempotent or marked-safe requests retried (HTTP-4).
- [x] **D.4** 401 single refresh+retry via `TokenRefresher`, shared attempt budget; second 401 -> auth; 403 -> `ForbiddenError` with vendor code, no body (AUTH-4/6, HTTP-6).
- [x] **D.5** Redacted tracing, off by default (HTTP-3). Examples, `user-docs/httpx.md`, API section.

### WS-E selftest (`selftest/**`)
- [x] **E.1** Matrix runner, probes supplied by tool, per-row pass/fail (SELF-1/2).
- [x] **E.2** Envelope output, exit 1 on failure, read-only-rows mode (SELF-3/4). Document on-demand only (SELF-5). Examples, `user-docs/selftest.md`, API section.

### WS-F docgen (`docgen/**`)
- [x] **F.1** `CommandTree` + deterministic generator (stable ordering, no timestamps) (DOC-1/3).
- [x] **F.2** Always-included sections: untrusted rule, envelope, exit codes; references (not duplicates) shared conventions skill (DOC-2, SKILL-3).
- [x] **F.3** Golden SKILL.md in `docgen/testdata`; sample tree vendor-neutral (DOC-4 generic). Examples, `user-docs/docgen.md`, API section.

## Phase 3 - WS-I Integration (after all of Phase 2)
- [x] **I.1** `examples/` sample tool + `internal/integration` end-to-end test over fakes (policy -> auth -> httpx -> output -> audit; selftest; docgen).
- [x] **I.2** Cross-package fuzz/property test: no token value in any log, error, trace, audit line, envelope.
- [x] **I.3** CI: `.github/workflows/ci.yml` per BLD-1..6 (gofmt, tidy diff, vet, pinned golangci-lint, `-race`, govulncheck, three-target compile, pinned actions, least privilege). No consumer/downstream or release jobs.
- [ ] **I.4** Docs final pass: `docs/technical-details.md` (all packages API, versioning/API-1..9, apidiff documented not wired), ADR (D4, authtest placement, TokenRefresher, redact leaf, YAML choice, deferrals), product docs, README, `user-docs` index, AGENTS.md layout update (PRD now under specs).
- [x] **I.5** Re-run archtest and full gate on all three targets. Optional **I.6** (deferred, not started) conformance kit only if small.

## Completion notes (WS-I)
- I.1: `examples/sampletool` (policy -> auth -> httpx -> output -> audit, selftest matrix, docgen tree) and `internal/integration` end-to-end tests over the fake daemon, httptest and the fake clock. `*auth.Authorizer` satisfies `httpx.TokenRefresher` structurally; a compile-time assertion test guards it, and no bridge code was needed.
- I.2: `internal/integration/noleak_test.go` (fuzz target `FuzzNoTokenLeak`, hostile-upstream matrix, fmt/JSON checks). It found a real leak in `httpx` (a server echoing the credential into a non-sensitive header reached the trace and `ForbiddenError` vendor code); fixed in `httpx` with regression test `httpx/echo_test.go`.
- I.3: CI rewritten per BLD-1..6; downstream and secret-using steps removed. Actions are pinned to release tags; commit-SHA pinning needs network and is deferred.
- I.5: full gate run on the branch (see status.md).

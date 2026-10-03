# Tasks - agent-cli-core
Date: 2026-10-03 | Status: Planning

## Progress Summary
0/33 tasks complete. Format: ID | owner | depends | est | files | acceptance. Every task is TDD (failing test first) and ends with its package quality gate. Workers touch only the listed files.

## Phase 1 - WS0 Foundation (owner: foundation worker)
- **P1.1** Repo scaffolding. Depends: none. 1h. Files: `go.mod`, `go.sum`, `Makefile`, `.golangci.yml`, `doc.go` stubs for auth, auth/authtest, policy, audit, httpx, selftest, docgen (with frozen `httpx.TokenRefresher`), YAML dep. AC: `go build ./...` ok; three-target cross-build ok.
- **P1.2** `internal/clock` (Clock, Fake). Files: `internal/clock/**`. AC: fake advances deterministically; Example.
- **P1.3** `internal/redact`. Files: `internal/redact/**`. AC: scrubs Authorization/bearer/secret headers, token-looking strings, bodies by default; table + fuzz test.
- **P1.4** `output` exit codes + Category/CategoryError/ExitFor. Files: `output/exit*.go`, goldens. AC: codes 0..9 golden (CORE-OUT-2/8).
- **P1.5** `output` envelope types and writers. Files: `output/envelope*.go`. AC: success/error golden, vendor-neutral text (CORE-OUT-1).
- **P1.6** `output` untrusted marking. AC: JSON `"untrusted": true`, text delimiters with author+timestamp (CORE-OUT-3).
- **P1.7** `output` bounds/truncation. AC: default 32768, `meta.truncated`/`next_offset`, never splits UTF-8 or yields invalid JSON, property test (CORE-OUT-5).
- **P1.8** `output` formats json/table/text + redaction of error message/hint (CORE-OUT-6/7).
- **P1.9** `internal/archtest`: import-graph and no-vendor-name test. AC: fails on a cycle or vendor name in import path/identifier (FR-025).
- **P1.10** Docs skeletons: `docs/technical-details.md` with `## API: <pkg>` sections, `user-docs/<pkg>.md` stubs; `output` section written. AC: skeletons present.
- **P1.11** Output `Example*` tests; gate green; commit/push. Gate for Phase 2.

## Phase 2 - parallel (each after P1.11)
### WS-A auth (`auth/**`, `auth/authtest/**`)
- **A.1** Types: redacting `Token`, `TokenSource`, `DaemonClient`, typed errors incl. `UnreachableError{Socket}` (CategoryError -> auth). AC: String/%v/%+v/GoString print `[redacted]` (CORE-AUTH-1/1a/8).
- **A.2** `authtest.Fake` (valid, expired-needs-refresh, reauth_required, revoked, unreachable, 401-then-success, 401-twice). Depends A.1 (CORE-AUTH-1b).
- **A.3** Daemon-backed `TokenSource` parameterised by provider; no hard-coded provider; no fallback (CORE-AUTH-2/3).
- **A.4** `Authorizer` satisfying `httpx.TokenRefresher` shape; reauth_required message generic + tool-supplied remediation (CORE-AUTH-4/5). AC: messages name socket; exit 3 mapping.
- **A.5** No-token-out tests; API check that nothing returns a token to a user surface (CORE-AUTH-9). Examples. `user-docs/auth.md`, API section; document deferred adapter and human-mode (CORE-AUTH-10/11).

### WS-B policy (`policy/**`)
- **B.1** Strict YAML parser, unknown keys error, invalid -> fail closed (CORE-POL-8).
- **B.2** Verb/resource allow/deny matching, rule ids, decision as data (POL-1/5).
- **B.3** Field allowlists, value constraints, write modes (POL-2/3).
- **B.4** Rate limits with fake clock; max results/bytes caps (POL-2/4).
- **B.5** Warn/refuse if file writable by current user (POL-6, POSIX only; P1). Examples, `user-docs/policy.md`, API section; guardrail-not-control wording (POL-9).

### WS-C audit (`audit/**`)
- **C.1** Versioned `Record` + JSONL writer, golden (AUD-1/5).
- **C.2** No secrets/bodies enforced; redaction pass (AUD-2).
- **C.3** Path from config; write-failure surfacing and blocking mode (AUD-3/4). Examples, `user-docs/audit.md`, API section.

### WS-D httpx (`httpx/**`)
- **D.1** Retrying transport with jitter, injectable clock/sleep (HTTP-1/5).
- **D.2** Retry-After handling for 429/503, bounded, MaxWait ceiling, rate-limited error -> exit 8 (HTTP-2).
- **D.3** Idempotency: only idempotent or marked-safe requests retried (HTTP-4).
- **D.4** 401 single refresh+retry via `TokenRefresher`, shared attempt budget; second 401 -> auth; 403 -> `ForbiddenError` with vendor code, no body (AUTH-4/6, HTTP-6).
- **D.5** Redacted tracing, off by default (HTTP-3). Examples, `user-docs/httpx.md`, API section.

### WS-E selftest (`selftest/**`)
- **E.1** Matrix runner, probes supplied by tool, per-row pass/fail (SELF-1/2).
- **E.2** Envelope output, exit 1 on failure, read-only-rows mode (SELF-3/4). Document on-demand only (SELF-5). Examples, `user-docs/selftest.md`, API section.

### WS-F docgen (`docgen/**`)
- **F.1** `CommandTree` + deterministic generator (stable ordering, no timestamps) (DOC-1/3).
- **F.2** Always-included sections: untrusted rule, envelope, exit codes; references (not duplicates) shared conventions skill (DOC-2, SKILL-3).
- **F.3** Golden SKILL.md in `docgen/testdata`; sample tree vendor-neutral (DOC-4 generic). Examples, `user-docs/docgen.md`, API section.

## Phase 3 - WS-I Integration (after all of Phase 2)
- **I.1** `examples/` sample tool + `internal/integration` end-to-end test over fakes (policy -> auth -> httpx -> output -> audit; selftest; docgen).
- **I.2** Cross-package fuzz/property test: no token value in any log, error, trace, audit line, envelope.
- **I.3** CI: `.github/workflows/ci.yml` per BLD-1..6 (gofmt, tidy diff, vet, pinned golangci-lint, `-race`, govulncheck, three-target compile, pinned actions, least privilege). No consumer/downstream or release jobs.
- **I.4** Docs final pass: `docs/technical-details.md` (all packages API, versioning/API-1..9, apidiff documented not wired), ADR (D4, authtest placement, TokenRefresher, redact leaf, YAML choice, deferrals), product docs, README, `user-docs` index, AGENTS.md layout update (PRD now under specs).
- **I.5** Re-run archtest and full gate on all three targets. Optional **I.6** conformance kit only if small.

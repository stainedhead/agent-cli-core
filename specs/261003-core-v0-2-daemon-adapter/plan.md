# core-v0-2-daemon-adapter - Plan

Created: 2026-10-03 | Status: Planning

## Development Approach
TDD (failing test, pass, refactor). Three independent workstreams in separate git worktrees (branches off feat/core-v0.2, e.g. `feat/core-v0.2-ws-a`) touching disjoint packages (spec section 7), then merged into feat/core-v0.2. Docs and compat artefacts afterwards (step 6).

## Phase Breakdown
- Phase 1 (parallel): WS-A auth/oktad + go.mod; WS-B output (R1, R2) + docgen (R4); WS-C clock (R3), audit (R5), policy trusted-file (R6), httpx (R7).
- Phase 2 (serial, after merge): integration: `go test -race -count=3 ./...`, golangci-lint, coverage, apidiff, archtest.
- Phase 3: docs, user-docs, ADRs, CHANGELOG, deferred.md, api-compat-v0.2.md, AGENTS/README/INTENT text fixes, PRD M0a done, CI `downstream` job.

## Critical Path
Longest: WS-C (four packages). WS-A is gated by the availability of `agent-okta-d` v0.1.0 in the module proxy (check first; fallback [TBD] is to stop and escalate, not to add a replace).

## Testing Strategy
- WS-A: real `client` against `clienttest` fake on a unix socket in `os.MkdirTemp("", "ocd")`; table test per mapping row; no sleeps/timing asserts (retry-after asserted as carried value); cancellation via pre-cancelled context; unreachable via non-existent socket in temp dir; token redaction test with sentinel token across `fmt` verbs, `errors`, `slog`; compile-time assertion.
- WS-B/C: unit + golden (new goldens only; existing unchanged) + Examples.
- Whole: `go test -race -count=3 ./...`, `go vet`, gofmt, golangci-lint, `-cover` at least 90 percent on new code.

## Rollout Strategy
PR to main; no tag; maintainers tag v0.2.0 after downstream job green.

## Success Metrics
AC1..AC5 satisfied; apidiff additions only; downstream job green.

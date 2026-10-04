# core-v0-2-daemon-adapter-auto-review - Feature Specification

Created: 2026-10-04 | Status: Draft | Source PRD: `specs/261004-core-v0-2-daemon-adapter-auto-review/core-v0-2-daemon-adapter-auto-review-PRD.md`

## 1. Executive Summary
Fixes from the step-7 review of agent-cli-core v0.2.0: two P1 items (CheckTrustedFile documentation overclaim, audit Extra key redaction) and six P2 items. No P0 items. No v0.1.0 exported identifier may change.

## 2. Problem Statement
See PRD: the release would otherwise claim a TOCTOU guarantee it does not give, and audit Extra keys can carry a secret past the redactor.

## 3. Goals / Non-Goals
Goals G1, G2 and non-goals as in the PRD (no features, no tag).

## 4. Functional Requirements
- FR-001 (R-P1-1): Rewrite the CheckTrustedFile guarantee in user-docs/policy.md, docs/technical-details.md, the godoc and docs/ADR (no descriptor returned; ancestors-trusted guarantee; ACLs/xattrs not examined); add deferred entry for a descriptor-returning variant.
- FR-002 (R-P1-2): audit.Logger.Log runs Extra keys through the redactor; if the redacted form differs, reject with ErrInvalidExtra; error text omits the key. Tests for WithSecrets-equal key and token-pattern key.
- FR-003 (R-P2-1): oktad mapError uses the context-error case only when ctx.Err() != nil AND err is a context error (verify dead-socket+cancelled behaviour first; else document precedence). Test.
- FR-004 (R-P2-2): oktad get rejects an empty revealed token with a general error. Test.
- FR-005 (R-P2-3): replace time.After in clock_test.go with Waiters()-based check.
- FR-006 (R-P2-4): make TestOSTrustFSOpenStat independent of a clean home chain; log when real-FS tests skip; confirm non-root Linux run.
- FR-007 (R-P2-5): tighten wording in docs/api-compat-v0.2.md and CHANGELOG about unkeyed literals; state how checked.
- FR-008 (R-P2-6): docgen sorts siblings by trimmed name with sort.SliceStable; order-independence test.

## 5. Non-Functional Requirements
Security: no token/key/body in errors or logs, sentinel leak tests. Reliability: no wall-clock, network or uid dependence in tests. gofmt, vet, golangci-lint, go test -race -count=3 ./..., coverage 90 percent or more on changed packages.

## 6. System Architecture
Dependency direction unchanged. Touched: audit, auth/oktad, clock (test), policy (godoc, tests), docgen, docs, user-docs, CHANGELOG.

## 7. Scope of Changes
Modify: audit/audit.go and tests; auth/oktad/oktad.go and tests; clock/clock_test.go; policy/trust*.go docs and trust_real_internal_test.go; docgen/docgen.go and tests; user-docs/policy.md; docs/technical-details.md; docs/architectural-decision-record.md; docs/deferred.md; docs/api-compat-v0.2.md; CHANGELOG.md. No new dependencies. Workstreams (disjoint, parallelizable in worktrees): WS-1 audit (FR-002); WS-2 oktad (FR-003, FR-004); WS-3 docs and policy (FR-001, FR-006, FR-007); WS-4 clock and docgen (FR-005, FR-008).

## 8. Breaking Changes
None. FR-002 newly rejects records whose Extra key redacts differently; such keys are new in v0.2 so no v0.1.0 behaviour changes. Golden audit output unchanged.

## 9. Success and Acceptance Criteria
Each PRD requirement's acceptance criteria hold; quality gates in section 5 pass; apidiff or manual symbol diff still lists additions only.

## 10. Risks and Mitigation
Q3 (client behaviour with cancelled ctx and dead socket) may force keeping current precedence: then document it. Linux non-root run unavailable locally: use CI.

## 11. Timeline and Milestones
M1 WS-1..WS-4 in parallel (TDD, one code review per fix); M2 merge; M3 quality pass.

## 12. References
- PRD: `specs/261004-core-v0-2-daemon-adapter-auto-review/core-v0-2-daemon-adapter-auto-review-PRD.md`
- Archived spec: `specs/archive/261003-core-v0-2-daemon-adapter`

# Tasks: agent-cli-core Auto-Review Remediation
Date: 261003 | Status: Complete (pending final review)

## Progress Summary
17/17 tasks complete

## Workstreams and Exclusive File Ownership
Rule: a file is edited only by its owning stream. Other streams needing a change in an owned file request it from the owner or wait until the owner merges, then rebase. Each stream = own worktree + branch off feat/agent-cli-core.

| Stream | FRs | Branch (suggested) | Exclusively owns |
|---|---|---|---|
| A | FR-001 | fix/redirect-host-policy | httpx/transport.go, httpx/config*.go (if any), new httpx/hosterror.go, auth/source.go, tests httpx/transport_test.go, httpx/redirect_test.go, auth/source_test.go |
| B | FR-002 | fix/token-redaction | auth/token.go, auth/token_test.go, auth/doc.go |
| C | FR-005, FR-003, FR-004 | fix/redaction-output | internal/redact/*, httpx/errors.go, httpx/errors_test.go, output/envelope.go, output/write.go, their tests/goldens |
| D | FR-007 | fix/policy-hits | policy/engine.go, policy/engine_test.go |
| E | FR-006 | fix/hinter-errors-as | output/hint*_test.go; edit of findHinter in output/envelope.go ONLY after C merges |
| F | docs | docs/remediation | docs/*, skills/agent-cli-core.md, changelog, README |

Parallelism: A, B, C, D start concurrently (A/B touch disjoint files: auth/source.go vs auth/token.go; the PRD's "B after A" sequencing is only needed if B must read A's auth changes, otherwise run in parallel). Within C: FR-005 first, then FR-003/FR-004 (needs rebased A for errors.go consumers: A adds the new typed error in its own file, C owns errors.go). E after C. F last.

## Phase 1 - Stream A (FR-001)
### P1.1 Failing repro tests (cross-host redirect, http to non-loopback, same-host redirect) [x]
Deps: none | 2h | AC: tests fail on current code.
### P1.2 AllowedHosts config + forbidden-host typed error (auth/forbidden-host) [x]
Deps: P1.1 | 3h | AC: Authorize skipped, typed error surfaces in audit/trace.
### P1.3 CheckRedirect default + plain-http refusal (loopback allowed) [x]
Deps: P1.2 | 3h | AC: all P1.1 tests pass; coverage >= 90%.
### P1.4 Review-code pass and commit [x]
Deps: P1.3 | 1h | AC: reviewer (not author) approves.

## Phase 1 - Stream B (FR-002)
### P1.5 Failing test: unexported Token field under %v %+v %#v %s %x [x]
Deps: none | 1h
### P1.6 Re-implement Token storage behind pointer/closure; update doc limits [x]
Deps: P1.5 | 2h | AC: secret absent in all verbs; JSON/text/slog still redacted.
### P1.7 Review-code pass and commit [x]
Deps: P1.6 | 1h

## Phase 2 - Stream C (FR-005, FR-003, FR-004)
### P2.1 FR-005 table tests + auth-scheme/opaque fixes; decide OQ-2 [x]
Deps: none | 3h | AC: prose "basic"/"bearer" preserved; SHA-1 handling tested; threshold documented.
### P2.2 FR-003 scrub RateLimitedError.Err and send errors via Redactor; decide OQ-3 [x]
Deps: P2.1 | 3h | AC: registered secret absent from Error().
### P2.3 FR-004 FromErrorWithSecrets/FailureWithSecrets; Write rescrubs error envelope [x]
Deps: P2.1 | 3h | AC: `abc-123-xyz` absent on both paths.
### P2.4 Review-code pass and commits [x]
Deps: P2.2, P2.3 | 1h

## Phase 2 - Stream E (FR-006)
### P2.5 errors.As-based hint lookup with test for errors.Join(actionErr, audit.WriteError) [x]
Deps: P2.3 (envelope.go merged) | 2h | AC: hint found for joined error.

## Phase 3 - Stream D (FR-007)
### P3.1 Record hits only when PerHour > 0; tests for bounded growth [x]
Deps: none | 2h | AC: len(e.hits)==0 for per-run-only policy; review pass.

## Phase 4 - Stream F and integration
### P4.1 Update docs, skills/agent-cli-core.md, changelog (FR-001, FR-004, FR-005 behavior notes) [x]
Deps: A, C | 2h
### P4.2 Merge streams in order A,B,C,D,E,F with rebases; run gofmt/vet/lint/race/coverage [x]
Deps: all | 2h | AC: all gates green.
### P4.3 Final review-code and update status.md [x]
Deps: P4.2 | 1h

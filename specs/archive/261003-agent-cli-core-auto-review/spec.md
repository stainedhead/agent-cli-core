# Spec: agent-cli-core Auto-Review Remediation

Created: 261003 | Branch: feat/agent-cli-core | Source: automated review (dev-flow review-code) of branch vs main

## Executive Summary
Quality is high (clean architecture, 90-100% coverage, build/vet/race/lint pass), but credential handling at the edges is weak. One P0 (bearer token forwarded on cross-host redirects), one P1 (auth.Token leaks in unexported fields), five P2. This spec remediates all seven findings. Verdict from review: request changes; FR-001 and FR-002 block merge.

## Problem Statement
The library's core guarantee is safe agent credential handling. The guarantee holds for direct use but is broken by redirect re-authorization, reflection-based printing, unscrubbed error text, and a non-secret-aware envelope path. Redaction also over-matches prose, and the policy engine leaks memory.

## Goals / Non-Goals
Goals: close all credential-exposure paths; keep changes additive or default-tightening; keep gates green and coverage >= 90%.
Non-Goals: new features beyond the fixes; changes to CI pinning (tracked elsewhere); external service work.

## User Requirements: Functional Requirements
- FR-001 (P0) Redirect/host safety: `Config.AllowedHosts` (default: first request host); Authorize skipped and typed error (auth/forbidden-host) for other hosts; `NewClient` installs `CheckRedirect` refusing cross-host redirects by default; plain http to non-loopback refused unless explicitly enabled. Tests: cross-host redirect never carries Authorization; same-host redirect works. Evidence: auth/source.go:113-128, httpx/transport.go:68,73,147-162.
- FR-002 (P1) `auth.Token` must not store secret as plain reflect-readable string (use pointer to redacting type or closure). Test unexported field under %v %+v %#v %s %x. Docs state remaining limits (reflect, memory dumps). Evidence: auth/token.go:12-16,31-32.
- FR-003 (P2) Pass `RateLimitedError.Err` message and wrapped send errors through Config.Redactor and held credentials; strip/document query secrets in url.Error text. Test: transport error containing registered secret absent from Error(). Evidence: httpx/errors.go:35-36, transport.go:104.
- FR-004 (P2) Add `FromErrorWithSecrets`/`FailureWithSecrets` (additive); verify Write rescrubs error envelope. Test: non-pattern literal secret `abc-123-xyz` absent from both paths. Evidence: output/envelope.go:53-71, output/write.go:139.
- FR-005 (P2) Auth-scheme pattern requires Authorization/header context or minimum credential length/charset; document/configure opaque-run threshold; decide and test git SHA-1 handling. Table tests for prose with "basic"/"bearer". Evidence: internal/redact/redact.go:23,28.
- FR-006 (P2) Hint lookup uses errors.As semantics (traverses Unwrap() []error). Test with errors.Join(actionErr, audit.WriteError). Evidence: output/envelope.go:73-86.
- FR-007 (P2) Record policy hits only for keys with PerHour > 0; test len(e.hits)==0 for per-run-only policy and bounded under hourly limits. Evidence: policy/engine.go:69-80.

## Non-Functional Requirements
- Security: no token/registered secret/query secret in any error, envelope, trace, or print path.
- Compatibility: additive or default-tightening; behavior changes (FR-001 redirect refusal, FR-005 matching) in changelog and root skill `skills/agent-cli-core.md` per AGENTS.md.
- Performance: FR-007 bounded memory; FR-001 adds one host comparison per request.
- Quality gates: gofmt, go vet, golangci-lint, go test -race; per-package coverage >= 90%.
- Observability: FR-001 denials are typed errors visible in audit log and trace.

## System Architecture
Affected: auth, httpx, output, policy, internal/redact (leaf; must remain leaf per archtest). Components: host policy in httpx Config + CheckRedirect; Token secret holder type; redactor-aware error formatting; secrets-aware envelope constructors; hit-recording guard in policy engine.

## Scope of Changes
Modify: auth/source.go, auth/token.go, httpx/transport.go, httpx/errors.go, output/envelope.go, output/write.go, internal/redact/redact.go, policy/engine.go, plus tests, docs (docs/product-details.md, docs/technical-details.md, auth/doc.go), skills/agent-cli-core.md, changelog. Create: new typed-error file in httpx, new test files. Dependencies: internal only.

## Breaking Changes
- API: none removed; new Config.AllowedHosts, new FromErrorWithSecrets/FailureWithSecrets, new typed error.
- Behavior: cross-host redirects and non-loopback plain http now refused by default; redactor matches fewer prose words. Config/schema: additive.

## Success and Acceptance Criteria
All per-FR acceptance tests written first (TDD) and passing; gates green; coverage >= 90% per package; each FR in its own commit with review pass; docs/skill/changelog updated.

## Risks and Mitigation
- FR-001 default tightening breaks users with legitimate redirects: document AllowedHosts, changelog.
- Streams A and C both touch httpx/errors.go: single-owner rule (see tasks.md), rebase before merge.
- FR-005 under-redaction: table tests including positive cases.
- Open questions OQ-1..OQ-3 (see research.md) with default decisions assumed.

## Timeline and Milestones
M1 FR-001+FR-002 (merge blockers); M2 FR-005 then FR-003/004; M3 FR-006/007; M4 docs, gates, final review. [TBD calendar dates]

## References
- Source PRD: specs/261003-agent-cli-core-auto-review/agent-cli-core-auto-review-PRD.md
- Original spec: specs/archive/261003-agent-cli-core

# Plan: agent-cli-core Auto-Review Remediation
Date: 261003 | Status: Planning

## Development Approach
TDD per FR (review's throwaway repro tests as starting point); one commit per FR; review-code pass by a reviewer distinct from the author; parallel streams in short-lived worktrees off feat/agent-cli-core. Never push to main.
## Phase Breakdown
- Phase 1: Stream A (FR-001), Stream B (FR-002) [merge blockers]
- Phase 2: Stream C (FR-005 -> FR-003/FR-004), then Stream E (FR-006)
- Phase 3: Stream D (FR-007), parallel with Phase 1/2
- Phase 4: Stream F docs/skill/changelog, full gates, final review
## Critical Path
FR-001 -> FR-003 (httpx) ; FR-005 -> FR-003/004 ; FR-004 -> FR-006 (envelope.go).
## Testing Strategy
Behavioral tests, goldens where applicable, -race, coverage >= 90%.
## Rollout Strategy
Merge into feat/agent-cli-core in order A, B, C, D, E, F; note behavior changes in changelog.
## Success Metrics
All FR acceptance tests pass; 0 lint issues; no secret in any output path.

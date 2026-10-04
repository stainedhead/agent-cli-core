# core-v0-2-daemon-adapter-auto-review - Plan

Status: Planning. Created: 2026-10-04 | Source PRD: `specs/261004-core-v0-2-daemon-adapter-auto-review/core-v0-2-daemon-adapter-auto-review-PRD.md`

## Development Approach
TDD per fix; one review per fix; P1 before P2.
## Phase Breakdown
Phase 1 four workstreams in parallel worktrees; Phase 2 merge; Phase 3 quality pass.
## Critical Path
FR-002 and FR-001 (P1).
## Testing Strategy
Unit tests, sentinel leak tests, -race -count=3, non-root Linux.
## Rollout Strategy
Same PR as v0.2.0; no tag.
## Success Metrics
All PRD acceptance criteria met.

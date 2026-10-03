# Dev-Flow Process Analysis

**Feature:** agent-cli-core (Go library: output envelope, auth, policy, audit, httpx, selftest, docgen)
**Spec directory:** specs/archive/261003-agent-cli-core
**Report generated:** 2026-10-03

---

## 1. Executive Summary

agent-cli-core is a shared Go library for building agent-safe CLIs. It provides a structured output envelope with exit codes, redacting auth tokens, a YAML policy engine, a JSONL audit log, a retrying and redacting HTTP transport, a selftest matrix runner, and a SKILL.md doc generator. The run built all of this, wrote the docs, ran an automated review, and then fixed seven review findings (FR-001..FR-007), mostly security hardening around credential leaks and redirects.

**Total runtime:** first repo commit 2026-10-03T15:31:36-04:00 to the last commit before this report 17:13:34-04:00 is about 102 min. That includes PRD authoring. The dev-flow proper (spec creation 16:30:18 to step 11 complete 17:13:34) is about 43 min.
**Overall assessment:** Fast and mostly clean. Parallel workstreams and a review-then-fix loop worked. State tracking was disrupted by a restart and lost worker reports (see section 6).

---

## 2. Step-by-Step Timing

DEV-FLOW-STATUS.md records UTC; git is authoritative and matches it (UTC minus 4h).

| Step | Name | Start (UTC) | End (UTC) | Runtime (min) | Key Outputs |
|---|---|---|---|---|---|
| 0 | PRD validation (review-prd) | n/a | n/a | n/a | PRD tweaks (682408b); no timing recorded |
| 1 | Create Spec from PRD | 20:27:26 | 20:30:08 | 3 | spec dir (0bb05b1) |
| 2 | Review Spec | 20:30:08 | 20:30:38 | 1 | acceptance criteria, edge cases (c8ee177) |
| 3 | Implement Product | 20:32:01 | 20:53:37 | 22 | scaffold, 6 parallel workstreams merged, integration and CI |
| 4 | Documentation and User Docs | 20:53:37 | 20:59:39 | 6 | docs pass (8d62795, dd4b0e4, e921687) |
| 5 | Code and Design Review | 20:59:39 | 21:01:49 | 2 | auto-review PRD (db64e16) |
| 6 | Prepare Review PRD | 21:01:49 | 21:02:38 | 1 | PRD review (711efb8) |
| 7 | Archive Original Spec | 21:02:38 | 21:03:24 | 1 | 8533ede |
| 8 | Spec Review Fixes | 21:03:24 | 21:04:51 | 1 | remediation spec (03d5a9b) |
| 9 | Implement Review Fixes | 21:04:51 | 21:11:33 | 7 | FR-001..FR-007 fixes, 3 merges, docs |
| 10 | Archive Fixes Spec | 21:11:33 | 21:12:10 | 1 | 179dad6 |
| 11 | Final Quality Pass | 21:12:10 | 21:13:34 | 1 | "stable" (5a2bfa3) |
| 12 | Process Analysis Report | 21:13:34 | see git | n/a | this file |

**Notable observations:**
- Step 3 is about half of dev-flow runtime (22 min), with six workstreams (policy, httpx, audit, selftest, docgen, auth) landing within about 90 s of each other (16:43:48-16:45:47) and merged together at 16:46:54.
- Step 0 has no recorded timestamps. The spec directory's commit-time notion of "Process Start 00:00:00Z" in the status file is a placeholder, not a real time.
- Steps 5-8 and 10-11 took 1-2 min each, which suggests thin checkpoints. Several are mostly status bookkeeping commits.
- Step 9: the Token reflection-leak test was written failing first (TDD, commit e2e97dd), then fixed.

---

## 3. Commit and Push Summary

**Total commits:** 59 before this report (git log, branch feat/agent-cli-core). Of these, 12 are dev-flow status commits, 7 are merge commits, and the rest are feature, fix, test and docs commits. Selected key commits (full list: `git log --format="%h %aI %s"`):

| Commit | Timestamp | Message |
|---|---|---|
| 36aff84 | 2026-10-03T15:31:05-04:00 | Initial commit: scaffold agent-cli-core repository |
| 0bb05b1 | 2026-10-03T16:30:18-04:00 | Create spec 261003-agent-cli-core from PRD |
| 01cb357 | 2026-10-03T16:34:56-04:00 | Scaffold packages, YAML dependency, internal/clock and internal/redact (P1.1-P1.3) |
| f3c7c63 | 2026-10-03T16:39:31-04:00 | output: envelope, exit codes, untrusted marking, bounds, formats |
| 9fee0f9 | 2026-10-03T16:46:54-04:00 | Merge ws/docgen (plus 4 sibling workstream merges) |
| e5825bd | 2026-10-03T16:52:17-04:00 | feat(integration): sample tool, e2e and token-leak tests, CI |
| 8d62795 | 2026-10-03T16:57:20-04:00 | docs: final documentation pass for implemented library |
| db64e16 | 2026-10-03T17:01:07-04:00 | docs: step 5 automated code review PRD |
| 8533ede | 2026-10-03T17:02:53-04:00 | Archive spec 261003-agent-cli-core and update references |
| ea05c98 | 2026-10-03T17:06:48-04:00 | fix(httpx): refuse cross-host redirects and plain http (FR-001) |
| 6ceb9e7 | 2026-10-03T17:05:57-04:00 | fix(auth): hold Token value in a closure (FR-002) |
| 6522476 | 2026-10-03T17:10:40-04:00 | fix(httpx): trace refused redirects; docs; spec status |
| 5a2bfa3 | 2026-10-03T17:12:47-04:00 | stable |
| e407920 | 2026-10-03T17:13:34-04:00 | Dev-flow: step 11 complete, step 12 start |

No PRs created yet (step 14). Branch was in sync with origin before this report.

---

## 4. Spec vs. Implementation Comparison

The spec did not carry time estimates, so planned durations are N/A. Phases from status.md versus git:

| Phase | Planned (spec) | Actual (git log) | Difference | Notes |
|---|---|---|---|---|
| Phase 0: Spec and research | N/A | 16:30:18-16:32:01 (about 2 min) | n/a | spec creation and review |
| Phase 1: Foundation (WS0) | N/A | 16:34:56-16:40:10 (about 5 min) | n/a | scaffold, output, archtest, docs skeleton |
| Phase 2: Parallel packages | N/A | 16:43:48-16:46:54 (about 3 min of commits) | n/a | six workstreams merged |
| Phase 3: Integration (WS-I) | N/A | 16:52:17 (single commit) | n/a | I.6 conformance kit deferred |
| Phase 4: Docs and review | N/A | 16:57:20-17:01:07 | n/a | docs then auto-review |
| Unplanned: review remediation | none | 17:03-17:12 (about 9 min) | n/a | FR-001..FR-007 |

**Phases skipped:** I.6 (optional conformance kit) deferred. The spec status.md was not fully updated (still shows Phase 4 pending and 38/39 tasks), because it was archived before the final docs and review work.
**Phases added:** review remediation spec and its implementation (steps 5-11); the leak found by the end-to-end test and fixed within WS-I.

---

## 5. Token / Message Usage

Exact token counts unavailable, and none were recorded. Rough estimate: one orchestrator context, plus about 6-8 parallel workstream workers in step 3, and a small number of workers for steps 4, 5, 9. Per-step turn counts were not captured and are not estimated here.

---

## 6. Process Observations

**Run provenance (candid note):** this run was a restart; the first attempt was blocked. Steps 0-2 were done earlier, before the restart. Worker output reports were occasionally lost, so state was verified from git (commits, merges, DEV-FLOW-STATUS.md) rather than from reports. Timing here is therefore derived from git and the status file only.

### What worked well
- Parallel workstreams in separate branches merged cleanly in one pass.
- Test-first fixes (failing test commit e2e... before the FR-002 fix) and a cross-package token-leak test caught real leaks.
- Automated review produced concrete, numbered findings that were fixed in under 10 minutes.

### What caused delays or rework
- Restart after a blocked first attempt, and lost worker reports, meant verification from git and some duplicated checking.
- Plain-http refusal (FR-001) broke existing send-error scrub tests, requiring a follow-up test fix (fba0c4f).
- Archiving the spec before review fixes left spec status.md stale.
- Step 0 and the process start/end fields in the status file lack real timestamps.

### Recommendations for future runs
- Have workers write reports to files (or commit them) so they survive a lost message.
- Record a real Process Start and step 0 times.
- Update spec status.md before archiving.
- Run the review earlier so security findings (redirects, token printing) land before docs.

---

## 7. Manual vs. Automated Comparison

**Estimated manual duration:** about 4-6 working days for one senior Go developer (library with seven packages, tests, CI, docs, then a review and remediation round). This is a rough guess that excludes meetings and external review wait time.
**Actual automated runtime:** about 43 min dev-flow, about 102 min including repo scaffold and PRD authoring.
**Efficiency gain:** roughly 10-20x on implementation time; less on PRD authoring and human decisions, which are not counted in the automated number.

The comparison assumes equivalent scope and quality gates (gofmt, vet, race tests, lint, govulncheck). It is not a measured benchmark.

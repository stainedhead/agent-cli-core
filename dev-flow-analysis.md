# Dev-Flow Process Analysis

**Feature:** agent-cli-core v0.2.0 (auth/oktad daemon adapter, public clock, audit Extra/RuleID/TargetRef, output NextPageToken/ArrayField, nested docgen, policy.CheckTrustedFile, httpx VendorCodeFromBody) plus its review fixes
**Spec directory:** specs/archive/261004-core-v0-2-daemon-adapter-auto-review (original: specs/archive/261003-core-v0-2-daemon-adapter)
**Report generated:** 2026-10-04

This report replaces the v0.1 analysis; that report remains in git history (commit e9e7dd0).

---

## 1. Executive Summary

v0.2.0 adds a credential-daemon adapter over agent-okta-d v0.1.0, a public clock package and a set of additive APIs requested by snow-cli, outlook-cli and teams-cli. A review of the result produced two P1 and six P2 items (no P0), all fixed in the same branch. A downstream CI job now builds the three consumers against each PR's core.

**Total runtime:** first commit 2026-10-03T23:44:32-04:00 (PRD) to the step 14 commit at about 2026-10-04T00:15-04:00: about 31 min of commit-to-commit time. Wall time before the first commit (planning, discussion) is not captured by git.
**Overall assessment:** Fast and clean. Three parallel workstreams merged without conflicts and the review-then-fix loop closed every finding. Gaps: the downstream CI job was believed to exist but did not, and one review item (non-root Linux run) could only be deferred to CI.

---

## 2. Step-by-Step Timing

Git commit times (local, -04:00) are authoritative. Many steps were committed seconds apart, so the runtime column is the gap to the previous commit, not effort.

| Step | Name | Start | End | Runtime (min) | Key Outputs |
|---|---|---|---|---|---|
| 1 | Create PRD | n/a | 23:44:32 | n/a | 14fcab9 |
| 2 | Review PRD | 23:44:32 | 23:44:45 | <1 | 3e6c460 |
| 3 | Create Spec | 23:44:45 | 23:46:14 | 1.5 | 835370d |
| 4 | Review Spec | 23:46:14 | 23:46:34 | <1 | 39f2c70 |
| 5 | Implement (3 workstreams) | 23:46:34 | 23:55:36 | 9 | 73f2c52, 298c91f, c2e08b6, merges |
| 6 | Docs, ADRs, CHANGELOG, api-compat | 23:55:36 | 00:01:09 | 5.5 | 737b0ec, 895de47 |
| 7-8 | Code review, review PRD | 00:01:09 | 00:05:00 | 4 | 50d96ff, ef545ca |
| 9 | Archive original spec | 00:05:00 | 00:05:09 | <1 | 4101092 |
| 10 | Review fixes spec | 00:05:09 | 00:05:33 | <1 | 0a68cda |
| 11 | Implement review fixes | 00:05:33 | 00:09:53 | 4.3 | 894945c, 7274fd4, 6b6656c |
| 11b-13 | Downstream CI job, archive fixes spec, quality pass | 00:09:53 | 00:13:33 | 3.7 | 5fde5da |
| 14-15 | Analysis, archive check | 00:13:33 | about 00:15 | about 2 | this report |

**Notable observations:**
- Implementation (step 5) was the largest block, as expected with three parallel workstreams.
- Step 13 found no code defects: gofmt clean, vet clean, golangci-lint 0 issues, race x3 green, govulncheck clean, go mod tidy no diff.
- The `downstream` CI job was missing and had to be added late (step 11b).

---

## 3. Commit and Push Summary

**Total commits:** 19 on feat/core-v0.2 since the v0.1 merge (c511f00), including the final analysis commit.

| Commit | Timestamp | Message |
|---|---|---|
| 14fcab9 | 2026-10-03T23:44:32-04:00 | Dev-flow: PRD for core v0.2 daemon adapter |
| 3e6c460 | 2026-10-03T23:44:45-04:00 | Dev-flow: PRD review fixes (per-requirement criteria, NFRs, defaults) |
| 835370d | 2026-10-03T23:46:14-04:00 | Dev-flow: create spec for core v0.2 (three parallel workstreams) |
| 39f2c70 | 2026-10-03T23:46:34-04:00 | Dev-flow: spec review fixes (edge cases, owners, AC traceability) |
| 73f2c52 | 2026-10-03T23:50:37-04:00 | feat(output,docgen): Meta.NextPageToken, Bounds.ArrayField, nested docgen commands |
| 298c91f | 2026-10-03T23:50:58-04:00 | feat(auth/oktad): daemon adapter over agent-okta-d v0.1.0 with error mapping |
| c2e08b6 | 2026-10-03T23:53:48-04:00 | WS-C: public clock, audit Extra/RuleID/TargetRef, policy.CheckTrustedFile, httpx VendorCodeFromBody |
| 0e4049e | 2026-10-03T23:55:19-04:00 | Merge branch 'feat/core-v0.2-ws-b' into feat/core-v0.2 |
| 70df4b0 | 2026-10-03T23:55:36-04:00 | Merge WS-C into feat/core-v0.2 |
| 737b0ec | 2026-10-03T23:57:52-04:00 | docs: add api-compat-v0.2 and deferred requests |
| 895de47 | 2026-10-04T00:01:09-04:00 | docs: v0.2.0 documentation, ADR-16..18, user-docs for oktad, clock and new APIs, CHANGELOG |
| 50d96ff | 2026-10-04T00:04:43-04:00 | docs: step 7 code review PRD for core v0.2.0 |
| ef545ca | 2026-10-04T00:05:00-04:00 | docs: finalize review PRD (goals, dependencies, open questions, guidance) |
| 4101092 | 2026-10-04T00:05:09-04:00 | docs: archive core v0.2 spec; update dev-flow status |
| 0a68cda | 2026-10-04T00:05:33-04:00 | docs: create review-fixes spec from review PRD |
| 894945c | 2026-10-04T00:07:54-04:00 | fix(audit,oktad): reject redactable Extra keys; daemon verdict beats late cancel; reject empty token |
| 7274fd4 | 2026-10-04T00:08:44-04:00 | Review fixes: TOCTOU docs, clock test, real-FS tests, compat wording, docgen ordering |
| 6b6656c | 2026-10-04T00:09:53-04:00 | Merge branch 'feat/core-v0.2-f2' into feat/core-v0.2 |
| 5fde5da | 2026-10-04T00:13:33-04:00 | ci: downstream compatibility job; archive review-fixes spec; final quality pass |

No PR is opened by this step; the orchestrator opens it (step 16).

---

## 4. Spec vs. Implementation Comparison

| Phase | Planned (spec) | Actual (git log) | Difference | Notes |
|---|---|---|---|---|
| Review-fix tasks P1.1-P1.8 | about 6.5 h of estimates | about 4.3 min of commits (agent-executed) | not comparable | Estimates assume a human developer |
| Quality pass | gofmt, vet, lint, race x3, 90 percent coverage | all met; lowest package 90.9 percent (auth/authtest), new packages 94-100 percent | none | clock 100, auth/oktad 98.4, audit 96.1, docgen 94.6, output 94.8, policy 98.3, httpx 96.1 |

**Phases skipped:** none. R-P2-4's non-root Linux confirmation was not possible locally and is left to CI (marked partly done in the spec).
**Phases added:** CI `downstream` job (go.work based, no consumer go.mod edit), validated locally for all three consumers.

---

## 5. Token / Message Usage

Exact token counts unavailable. Estimate: the orchestrator plus one sub-agent per workstream (3) and one for the review fixes and one for the final pass; each step was a handful of tool turns.

---

## 6. Process Observations

### What worked well
- Disjoint workstreams merged cleanly; the archtest allowed-edge list caught dependency direction changes.
- The review found real issues (a secret usable as an audit Extra key, a TOCTOU documentation overclaim) before release.
- A go.work file lets CI test consumers against the PR core without editing their go.mod, so their guard tests still run.

### What caused delays or rework
- The lead believed a `downstream` job existed; it did not, discovered only at step 13. The earlier api-compat harness used a replace directive, which tripped two consumer guard tests.
- DEV-FLOW-STATUS.md still pointed at the pre-archive spec path until step 13.

### Recommendations for future runs
- Verify claimed CI jobs by reading the workflow at spec time.
- Run the downstream job on the PR; consumers should pin the new tag after release.
- Add a non-root Linux trust-test check to CI visibility (the skip log line makes a skipped run visible).

---

## 7. Manual vs. Automated Comparison

**Estimated manual duration:** about 2 to 3 working days for a senior developer (adapter, six API additions, docs and ADRs, review, fixes, CI), excluding meetings.
**Actual automated runtime:** about 31 min of commit time.
**Efficiency gain:** roughly 25 to 40 times on commit time; the estimate ignores human review of the PR and the planning time before the first commit.

# core-v0-2-daemon-adapter-auto-review - Status

Created: 2026-10-04 | Completed: 2026-10-04 | State: Complete

| Phase | Status |
|---|---|
| Phase 0 spec | Complete |
| Phase 1 WS-1 audit / WS-2 oktad / WS-3 docs+policy / WS-4 clock+docgen | Complete (commits 894945c, 7274fd4, merged 6b6656c) |
| Phase 2 integration | Complete |
| Phase 3 quality pass | Complete (see DEV-FLOW-STATUS.md step 13) |

Phase 0: [x] spec created  [x] research questions identified  [x] phase files initialized

## Review item verification (checked in code and tests, 2026-10-04)

| Item | Result | Evidence |
|---|---|---|
| R-P1-1 TOCTOU docs | Done | Guarantee rewritten in policy/trust.go godoc, user-docs/policy.md, docs/technical-details.md, ADR; deferred.md entry for `policy.OpenTrustedFile` |
| R-P1-2 audit Extra key redaction | Done | `Logger.checkExtra` rejects keys changed by the redactor with `ErrInvalidExtra`, error text omits the key; `TestExtraKeyRedactionRejected`, `TestExtraKeySyntaxErrorOmitsKey`, `TestExtraKeyUnaffectedByUnrelatedSecrets` |
| R-P2-1 oktad ctx precedence | Done | `mapError` uses the context case only when ctx is done AND err is a context error; `TestMapErrorDaemonVerdictBeatsLateCancel`. Note: the research question about a real client with a dead socket and cancelled ctx was not exercised against the daemon client; behaviour is covered with an injected error and documented in `doc.go` |
| R-P2-2 oktad empty token | Done | `get` rejects an empty revealed token (general error); `TestGetRejectsEmptyCredential` |
| R-P2-3 clock test | Done | wall-clock `time.After` replaced by a `Waiters()` check in `TestFakeSleepWakesOnAdvance` |
| R-P2-4 real-FS tests | Partly done | `TestOSTrustFSOpenStat` and `TestOSTrustFSOpenUnreadable` use a plain temp dir (no clean-home dependency); skips log "SKIPPED real-filesystem trust test". A non-root Linux run was not possible locally; it is left to the CI `verify` job |
| R-P2-5 compat wording | Done | docs/api-compat-v0.2.md and CHANGELOG reworded; keyed-literal check done by grep and by building all three consumers |
| R-P2-6 docgen ordering | Done | `sortByName` uses trimmed names with `sort.SliceStable`; `TestSiblingOrderUsesTrimmedNames` |

Also added in step 13: CI `downstream` job (build, vet, test of snow-cli, outlook-cli, teams-cli against this core via go.work).

Blockers: none.

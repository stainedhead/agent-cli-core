# Core change requests: implemented and deferred in v0.2.0

Sources: `snow-cli/docs/core-change-requests.md` (CR-xx), `outlook-cli/docs/requested-core-changes.md` (outlook n) and `teams-cli/docs/requested-core-changes.md` (teams n). Items 7/8 of outlook/teams (the daemon adapter) are implemented as package `auth/oktad` (see the adapter ADR). v0.2.0 is additive only: nothing here changes a v0.1.0 signature or default behaviour.

## Implemented

| Request | Asked by | Implemented as |
|---|---|---|
| Continuation token | outlook 2, 13, 18; teams 5 | `output.Meta.NextPageToken` (`json:"next_page_token,omitempty"`) |
| Bound an object holding an array; absolute offset for it | snow CR-04, CR-12; outlook 13; teams 6 | `output.Bounds.ArrayField` (+ `output.ErrArrayField`). Trims whole items, sets `Meta.Truncated`, `Meta.NextOffset` = `Offset` + items kept (an absolute item index). Also resolves snow CR-03 for object data |
| Importable clock | outlook 6, 14; teams 7 | package `clock` (`Clock`, `System`, `Fake`); `audit.WithClock` and `httpx.Config.Clock` accept it; `internal/clock` is an alias |
| Nested docgen commands | outlook 9; teams 9 | `docgen.Command.Subcommands`, `docgen.MaxDepth` |
| Audit extension fields, rule id, target ref | outlook 8, 17; teams 3; snow CR-07 (partly), CR-10 (target ref) | `audit.Record.Extra` (`*ExtraFields`, bounded), `Record.RuleID`, `Record.TargetRef` |
| Standalone trusted-file check | outlook 5, 15; teams 2 | `policy.CheckTrustedFile`, `policy.WithTrustedUIDs`, `policy.TrustError`, `policy.ErrNotTrusted` (a `TrustError` is an `output.CategoryError`, policy_denied). `policy.Load` is unchanged |
| Vendor error code from the body | outlook 1, 16, 19; teams 4 | `httpx.Config.VendorCodeFromBody`, `VendorBodyLimit`, `DefaultVendorBodyLimit`, `MaxVendorBodyLimit`. Header-based `VendorCode` still works |
| Real daemon adapter | outlook 7; teams 8 | package `auth/oktad` |

## Deferred

| Request | Asked by | Reason |
|---|---|---|
| Shared write counter across rules (`max_writes_per_run`) (snow CR-01) | snow | Needs a new policy concept (a verb-class counter) and the persistence design of the rate-limit item below. Workaround (per-rule `per_run`) is safe, only looser |
| Required-field rule on create (snow CR-02) | snow | Adds a new rule kind and schema to the generic policy engine; the CLI validates required flags before the policy check (exit 9). Better designed with the typed-rule discussion below |
| Standalone offset base `OffsetBase` (snow CR-03) | snow | Covered for object data by `Bounds.ArrayField` (`NextOffset` is `Offset` + kept, absolute). No separate option added; reopen if a use outside `ArrayField` appears |
| `policy.DeniedError` as `output.CategoryError` (snow CR-05) | snow | Changes how `output.FromError` classifies an existing error (general to policy_denied, exit 6): a behaviour change for v0.1.0 consumers, so not additive. Candidate for v0.3 / a minor with a changelog warning. Adapter wrapper keeps working |
| Settable `httpx.Config.Redactor` (snow CR-06) | snow | The field type is the internal `redact.Redactor`. Exposing a public redaction type or option is an API design of its own. The default redactor is always applied when nil |
| Configurable 5xx classification (snow CR-08) | snow | Today only 429, 502, 503, 504 are retried/mapped; widening or making this configurable changes retry semantics (risk of replaying non-idempotent calls) and needs its own design |
| Selftest exit codes and structured per-row results in the envelope (snow CR-09) | snow | A failing matrix is exit 1 with the detail in the message. Changing the failure envelope or exit code is a behaviour change. Note `selftest.Result` already carries per-row `Rows` for the success and `Write` paths; the failure envelope form carries no data |
| Cross-process rate-limit state / injectable counter store (snow CR-11) | snow CR-11, outlook 4, teams 1 | **Requested by all three CLIs.** Deferred as a larger design: the policy engine keeps rate-limit counters in memory per process, and every CLI run is a new process. The fix is an injectable counter store interface (file/flock, with fail-closed semantics and atomic read-modify-write) plus time-window handling and tests. Each CLI uses a local workaround (snow `auditx.StateLimiter`, the others the idempotency ledger). Highest priority for v0.3 |
| Typed policy rules (recipient, domain, mailbox, destination, sender, mention, content) | outlook 4, teams 1 | The core engine is generic (verb/resource/field) by design; typed domain rules belong in each CLI's `internal/domain`. Not a core defect |
| `output.Untrusted` JSON shape (`text`, `format`, `truncated` siblings; extra fields) | outlook 3, teams 6 | Wire-format change that would break goldens and consumers parsing `{"untrusted":true,"value":...}`. CLIs add sibling fields instead. Documented, not changed |
| First-class pending / `dry_run` / `applied_conflict` outcomes (snow CR-07, CR-10 outcomes) | snow | `Outcome` stays a free label by design. Target ref is implemented (`TargetRef`); the outcome enum is not |

## "To confirm" items, verified against the code

| Item | Question | Finding |
|---|---|---|
| outlook 10, teams 10 | Does httpx retry a 401 exactly once and never retry a non-idempotent POST? | `httpx/transport.go`: a 401 triggers one `Refresher.Refresh` and one resend (the `refreshed` flag); a second 401 returns `*AuthError`. The refresh is skipped (returns `*AuthError`) with no Refresher, when `MaxRetries` is exhausted or negative, or when the body is not replayable (`GetBody` nil). The 429/502/503/504 and transport-error retries apply only to idempotent methods or requests marked with `httpx.MarkSafeToRetry`, with a replayable body. **Nuance:** a POST with a replayable body does get the single 401 refresh-and-resend (the server rejected the credentials, so the request was not processed); only the error/5xx/429 retries are blocked for POST. A CLI that wants zero resend of a POST on 401 must not supply `GetBody` or must use no Refresher |
| outlook 11, teams 11 | Can a selftest probe express an expected denial? | Yes. `selftest.Row.Expect` is `allow` or `deny`, and a `Probe` returns an `Outcome`. A probe maps a 403 (or 404) to `selftest.Deny`; a row expecting `Deny` then passes (`StatusPass`). No core change needed |
| outlook 12, teams 12 | Does `audit.Record` have a free-form body field? | No body field. Free strings exist (`Resource`, `Outcome`, `PolicyDecision`, `TargetRef`, `RuleID`) and v0.2 adds `Extra`, but `Extra` is bounded (max `MaxExtraKeys` keys, key pattern `[a-z0-9_.-]{1,32}`, values cut at `MaxExtraValueLen`, invalid keys reject the record) and values pass through redaction. Keeping content out remains a caller convention that the bounds make hard to abuse, not a hard guarantee |

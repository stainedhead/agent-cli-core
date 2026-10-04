# core-v0-2-daemon-adapter - Feature Specification

Created: 2026-10-03 | Status: Draft | Source PRD: `specs/261003-core-v0-2-daemon-adapter/core-v0-2-daemon-adapter-PRD.md`

## 1. Executive Summary
agent-cli-core v0.2.0 adds (a) `auth/oktad`, the real `auth.DaemonClient` over `agent-okta-d` v0.1.0 `pkg/client`, and (b) seven small additive APIs (R1..R7) requested by at least two of snow, outlook and teams. Strictly additive versus the v0.1.0 tag.

## 2. Problem Statement
Consumers carry stub daemon clients (exit 3 always) and duplicated workarounds for continuation tokens, object bounding, clocks, nested docgen, audit extension fields, trusted-file checks and vendor error codes. See PRD.

## 3. Goals / Non-Goals
Goals G1-G3 and non-goals as in the PRD (no tag, no breaking change, deferred items in `docs/deferred.md`).

## 4. Functional Requirements
FR-001..FR-012 are exactly the PRD's. Implementation design per requirement:

### FR-001/002/003 auth/oktad (WS-A)
- Package `oktad`: `type Client struct{ c *client.Client; ... }`, `func New(opts ...Option) *Client`, `Option`, `WithSocketPath(string)`, `WithTimeout(time.Duration)`. Defaults: socket from `client.New` default (`DefaultSocketPath()` honouring `AGENT_OKTA_D_SOCKET`); timeout `client.DefaultTimeout`. Options are applied to `client.New(client.WithSocketPath, client.WithTimeout)` only when set.
- Methods `Fetch(ctx, provider)` -> `credential(ctx, provider, c.c.Credential)`; `Refresh(ctx, provider)` -> `c.c.Refresh`. Both return `auth.NewToken(cred.AccessToken.Reveal())` (preserving any expiry fields `auth.Token` supports; confirm against `auth/token.go` in implementation). `Reveal()` is called in exactly one place.
- `var _ auth.DaemonClient = (*Client)(nil)`. `Close()` delegates to `client.Close` if useful (not part of `DaemonClient`).
- Exact method set of `auth.DaemonClient` is confirmed from `auth/source.go` in task A1; this spec takes the PRD/change description's `Fetch`/`Refresh(ctx, provider) (auth.Token, error)` as authoritative.

### FR-004 Error mapping (WS-A, new ADR "adapter error mapping")
Evaluated in this order using `errors.Is` / `errors.As` (the client's `*APIError.Is` handles sentinel matching):
1. `ctx.Err() != nil` and the error is a context error -> returned wrapped (`fmt.Errorf("oktad: %w", err)`), category general (exit 1). Never mapped to unreachable. (Decision D-A1.)
2. `ErrDaemonUnavailable` -> `&auth.UnreachableError{Socket: c.c.SocketPath(), Err: err}` (exit 3). Field names confirmed against `auth/errors.go`.
3. `ErrReauthRequired` -> `auth.ErrReauthRequired` (wrapping the cause where the sentinel allows) (exit 3).
4. `ErrRevoked` -> `auth.ErrRevoked` (exit 3).
5. `ErrDegraded` or `*APIError` with `RetryAfter > 0` -> `*oktad.TransientError{RetryAfter, Err}` implementing `output.CategoryError` (rate_limited, exit 8), `Hint()` naming the wait, and `RetryAfter() time.Duration` accessor. `client.RetryAfter(err)` supplies the duration. (Decision D-A2: adapter-local type, so `auth` need not import `httpx`.)
6. `ErrNotConfigured`, `ErrUnauthorized` -> `*auth.ActionRequiredError`-style auth-category error (exit 3) with a hint (provider not configured / caller not authorized for this agent). (Decision D-A3: exit 3, not 4, because exit 4 means a server 403; this is a local credential problem; ADR records this against the core PRD.)
7. `ErrInvalidResponse` and anything else -> general error (exit 1) via a `*auth.TokenError` or plain wrapped error; message scrubbed.
The token never reaches an error: errors wrap client errors, which contain no token, and `Credential` values are never formatted.

### FR-005 R1 Meta.NextPageToken (WS-B)
`output.Meta` gains `NextPageToken string `json:"next_page_token,omitempty"``. Added after existing fields; marshalling order of existing fields unchanged. Rendered in text/table formats only where `next_offset` already is (follow the same rule). Redaction/bounds: token is opaque, passed as is; documented as caller-responsible for being non-secret.

### FR-006 R2 object bounding (WS-B)
`output.Bounds` gains `ArrayField string` (name of a top-level key of an object-shaped data value whose value is an array). When set and data marshals to an object containing that key as an array, `fit` trims trailing elements of that array until the full envelope fits, sets `Meta.Truncated=true` and `Meta.NextOffset` = Offset + number of items kept (consistent with existing array semantics, [TBD confirm in code: Offset applies by skipping the first Offset items of that array]). If the field is missing or not an array: return a documented error wrapped as usage category (exit 2) [decision D-B1] rather than silently ignoring. If even zero items does not fit: existing `ErrBoundTooSmall`. When `ArrayField == ""` behaviour is byte-identical to v0.1.0. Offset-base (snow CR-03) is satisfied by Offset semantic above; no separate `OffsetBase` (D-B2: deferred unless trivial in implementation).
Object data may be a struct or map; implementation marshals to `map[string]json.RawMessage`, trims the named array as `[]json.RawMessage`, re-marshals. Key order of the remaining fields becomes sorted if map is used: use an ordered approach (decode with `json.Decoder` tokens) or only re-encode the array field, to keep field order stable [TBD verify in tests].

### FR-007 R3 public clock (WS-C)
New top-level package `clock`: `Clock` interface (`Now()`, `Sleep(ctx, d)`), `System`, `Fake` with `NewFake(t)` and `Advance(d)` (a promotion of `internal/clock`). `internal/clock` becomes thin aliases: `type Clock = clock.Clock`, `type System = clock.System`, `type Fake = clock.Fake`, `var NewFake = clock.NewFake` (or function wrapper), so every existing internal import compiles unchanged and types are identical. Existing internal clock tests stay. `audit.WithClock(c clock.Clock)` signature text is unchanged in meaning (the parameter type is now the public alias); consumers can pass any implementer or `clock.Fake`. `policy.Clock` and `httpx.Clock` remain declared as is (interfaces satisfied by public types). Docs/examples show injection. Package `clock` has no deps and must pass archtest layering (add to the allowed-package list in WS-C).

### FR-008 R4 docgen nesting (WS-B)
`Command` gains `Subcommands []Command` (omitted/empty = leaf). Generation walks the tree, sorts siblings by name, renders children under the parent heading one level deeper, with full path name (`mail send`) in headings and the usage line. Validation: names unique among siblings; nested name rules same as top level. Flat `Commands` output unchanged (golden). [TBD: whether a parent with its own flags/args renders both.] Depth limit [TBD, suggest 4].

### FR-009 R5 audit (WS-C)
`Record` gains `Extra map[string]string `json:"extra,omitempty"``, `RuleID string `json:"rule_id,omitempty"``, `TargetRef string `json:"target_ref,omitempty"``. All pass through the existing redactor; `Extra` limits: at most 16 keys, key matching `[a-z0-9_.-]{1,32}` (invalid keys dropped or error, [TBD D-C1: error vs drop; prefer reject with error), value truncated to 256 bytes; keys emitted in sorted order for determinism. No body-like fields. `SchemaVersion` stays 1 because additions are omitempty and optional [TBD D-C2: confirm consumers tolerate unknown keys; document]. Any CSV/column writer emits the new fields only when present.

### FR-010 R6 trusted file (WS-C)
`policy.CheckTrustedFile(path string, opts ...TrustOption) error` (name final in implementation; ADR): `WithTrustedUIDs(uids ...uint32)` (default: {0}; never the effective uid; calling with no uid means root only), `WithAllowGroupWritable`-style loosening is NOT offered. Semantics: lstat each ancestor from `/` to the file: owner must be in trusted set; mode not group/world-writable (sticky-dir exception for ancestors owned by root documented, [TBD]); symlinks: target owner and the link's owner checked and resolved path re-verified; the file is opened `O_RDONLY|O_NOFOLLOW` and `fstat` on the descriptor is the check of record (owner, mode, regular file). Returns `*TrustError` (with `Reason`, `Path`) exported for matching; category policy_denied. Unix only via build tags; other platforms return a fail-closed error. `policy.Load` and `WritableMode` untouched. Tests need chown; run as non-root by checking against current uid as the trusted uid parameter, and root-owned cases skip when not root [TBD CI behaviour].

### FR-011 R7 httpx vendor code from body (WS-C)
`Config.VendorCodeFromBody func(status int, prefix []byte) string`. When a 403 (and, [TBD] 401/429) is received and the hook is set, the transport reads at most `BodyPrefixLimit` (const 4096, `Config` knob `VendorBodyLimit`) bytes of the body (`io.LimitReader`), calls the hook, closes the body, and puts the result (bounded to the existing 64 byte `maxVendorCode`, scrubbed) in `ForbiddenError.VendorCode`. Precedence: header `VendorCode` first when it returns non-empty, else body hook. New fields `Status` on `ForbiddenError`/`AuthError`/`RateLimitedError` are already or become exposed; a bounded redacted `BodyPrefix` is NOT added to error types (D-C3: avoids leaking bodies; hook only). Existing Config zero value unchanged.

### FR-012 Docs (post-workstreams, step 6)
As PRD; not part of the workstreams.

## 5. Non-Functional Requirements
As PRD. Plus: new exported identifiers have doc comments and Examples; `go test -race -count=3 ./...`; coverage at least 90 percent on new/changed code (`go test -cover` per package, new files measured with `-coverprofile` and `go tool cover -func`).

## 6. System Architecture
Dependency direction unchanged. `auth/oktad` is an edge adapter: imports `auth`, `output`, `agent-okta-d/pkg/client`; `auth` does not import `oktad`. New package `clock` is a leaf. Affected: `auth/oktad` (new), `clock` (new), `output`, `docgen`, `audit`, `policy`, `httpx`, `internal/clock`, `internal/archtest` (allowed-dependency lists), `go.mod`/`go.sum`.

## 7. Scope of Changes and Workstreams
Three independent workstreams touching disjoint packages; each can run in its own git worktree and merge in any order.

| WS | Packages / files owned (exclusive) | Requirements |
|---|---|---|
| WS-A | `auth/oktad/**`, `go.mod`, `go.sum`, ADR text (draft in the PR description or `auth/oktad/doc.go`; the ADR file edit itself is in step 6), the oktad entry in `internal/archtest` config | FR-001..004 |
| WS-B | `output/**`, `docgen/**` | FR-005, FR-006, FR-008 |
| WS-C | `clock/**` (new), `internal/clock/**`, `audit/**`, `policy/**`, `httpx/**`, the clock entry in `internal/archtest` config | FR-007, FR-009, FR-010, FR-011 |

Not in any workstream (step 6, after merge): `docs/**`, `user-docs/**`, `README.md`, `AGENTS.md`, `INTENT.md`, the PRD text, `CHANGELOG.md`, `docs/deferred.md`, `docs/api-compat-v0.2.md`, `.github/workflows/ci.yml` (downstream job).

Overlap hazards:
- R3 touches `audit`, `policy`, `httpx` options; R5, R6, R7 also edit `audit`, `policy`, `httpx`. All of these are WS-C; do not split R3 away from them.
- `go.mod`/`go.sum`: only WS-A adds `agent-okta-d`. WS-B and WS-C add no dependencies (stdlib only). If a workstream needs a dependency, stop and coordinate.
- `internal/archtest`: both WS-A (allow `agent-okta-d/pkg/client` for `auth/oktad` only; confirm the vendor-name rule does not flag `okta`) and WS-C (new `clock` package) edit its configuration. Different lines; textual conflict is trivial. Whoever merges second rebases.
- `output.Meta` golden files: only WS-B edits `output` goldens; WS-C and WS-A must not.
- `auth/oktad` must not import `httpx` (keeps WS-A independent of WS-C).
- Package docs (`doc.go`) of touched packages are edited by the owning workstream; `docs/technical-details.md` only in step 6.

## 8. Breaking Changes / Backward-compatibility check (explicit)
No breaking changes are permitted. Check procedure and per-item analysis:
1. Tooling: `go run golang.org/x/exp/cmd/apidiff@latest` exporting API of tag v0.1.0 (via `git worktree` or `apidiff -m` on module path at `v0.1.0`) against the branch for every public package; result must list only "Compatible changes". Otherwise (tool unavailable) manual symbol diff produced by `go doc -all` for each package at both refs recorded in `docs/api-compat-v0.2.md`. gorelease acceptable alternative.
2. Per change: R1 new struct field (omitempty; unkeyed `Meta{...}` literals would break: confirm no consumer or in-repo use of unkeyed literals; apidiff treats added fields as compatible unless struct is comparable-used, note it). R2 new `Bounds` field (same unkeyed-literal caveat). R4 new field on `Command`. R5 new fields on `Record`. R3: `audit.WithClock` parameter changes from `internal/clock.Clock` to `clock.Clock` via alias; type identity preserved (alias), so no signature change. R6/R7 new funcs/fields. Struct additions to types that consumers compare with `==` (map field in `Record` makes `Record` non-comparable!). **Risk RB-1:** adding `Extra map[string]string` to `audit.Record` makes `Record` non-comparable; any consumer doing `==` on a Record or using it as a map key stops compiling. Mitigation: grep the three consumers; apidiff flags this as incompatible; if flagged or used, switch to `Extra []KV`-free alternative (e.g. unexported map plus accessor, or `Extra` pointer to a struct) [decision D-C4, resolved during WS-C task C-R5-0].
3. Behaviour: all new fields omitempty/zero-valued -> v0.1.0 output byte-identical (existing goldens unchanged and must not be edited in this change). Verification: `git diff v0.1.0 -- '**/testdata/**' '**/*golden*'` shows no modification of existing golden files (new files only). Existing `_test.go` files are unmodified except compile-required import changes (none expected).
4. Dependencies: `go.mod` adds one require (agent-okta-d v0.1.0), no replace; `go` directive unchanged; the module graph for consumers gains `agent-okta-d` only when they import `auth/oktad`... note Go modules still add it to the consumer's graph (go.sum entries); consumers on v0.1.0 are unaffected until they upgrade. Document.
5. Consumers: CI `downstream` job builds and tests snow, outlook, teams against the PR core using a `replace` in a temporary go.work/go.mod (CI only, not committed); step 6.
6. Semver: all of the above is a minor bump (v0.2.0, pre-1.0).

## 8a. Edge Cases and Error Paths (added in spec review)
- oktad: empty provider string -> `*auth.TokenError`-style validation error before any socket call (exit 1) [or pass through to daemon whose error maps; choose the former, D-A4]; zero-value `Client` (not via `New`) must not panic: methods return a general error; concurrent `Fetch`/`Refresh` from multiple goroutines is safe (the underlying client is; covered by a `-race` test); `Close` is idempotent; a `Credential` with empty `AccessToken` (`IsZero`) -> `ErrInvalidResponse`-equivalent general error, never an empty token; expired credential returned by daemon is returned as is (the TokenSource decides); `*APIError` with unknown status/code -> general error; wrapped errors (`fmt.Errorf("%w")`) still classified via `errors.As/Is`; socket path longer than the OS limit -> unreachable error naming the path (test uses short dirs only).
- R1: empty token omitted; very long token is not truncated (opaque) but still counted in the byte budget.
- R2: `Offset` beyond array length -> empty page, not truncated; empty array; array field is `null`; data is not an object (array/string) while `ArrayField` set -> usage error (D-B1) [consistent: existing array/string bounding is not applied]; single item larger than budget -> `ErrBoundTooSmall`; multi-byte UTF-8 inside items preserved (items are whole JSON values, never cut); nested `ArrayField` paths are NOT supported (top-level key only, documented).
- R3: `nil` clock passed to `audit.WithClock` keeps current behaviour (system clock) rather than panicking [TBD verify in code]; `Fake.Sleep` honours context cancel; alias identity checked by a compile test using both import paths.
- R4: empty `Subcommands` equals leaf; duplicate sibling names -> validation error; same name under different parents allowed; cycles impossible (value types); depth limit 4 exceeded -> validation error.
- R5: nil `Extra`; key collisions with fixed JSON keys are impossible because `Extra` is nested under `extra`; value containing secrets (bearer-looking strings) is redacted; control characters/newlines in values escaped by JSON encoding (no log injection); more than 16 keys -> error (D-C1); concurrent `Log` calls remain serialised by the existing mutex.
- R6: path not existing, directory instead of file, empty path, relative path (resolved to absolute first), symlink loops, file replaced between check and open (fstat on descriptor is authoritative), non-unix build -> fail closed; permission-denied on an ancestor -> error (fail closed); sticky world-writable ancestor like `/tmp` owned by root is rejected unless it is the file's own directory exception is NOT granted (strict, D-C5).
- R7: hook panics are not recovered (documented; caller bug); body read error or timeout -> hook is not called, vendor code empty; body shorter than limit; non-UTF-8 bytes; empty body; hook returns very long or control-char string -> bounded to 64 bytes and scrubbed; body is drained/closed so connections are reused; hook applies to 403 only (D-C6: 401/429 keep their v0.1.0 typed errors without code); header `VendorCode` non-empty wins.

## 8b. Owners and resolution path for open items
All decisions D-* in section 12 are owned by the implementing workstream lead and recorded in `implementation-notes.md` when closed; D-C4 (Record comparability) must be closed first in WS-C (task C3) because it can change the R5 field shape; D-A3 (exit 3 vs 4) is confirmed by the maintainer at PR review. Unresolved [TBD] items must be closed or moved to `docs/deferred.md` before step 13.

## 8c. Acceptance criteria to FR traceability
AC1 -> FR-001..004 (A4-A8); AC2 -> FR-005..011 (B1-B6, C1-C6); AC3 -> section 8 and I2, CI downstream job; AC4 -> B7, C7, A8, I1; AC5 -> FR-012 and I3.

## 9. Success and Acceptance Criteria
AC1..AC5 from the PRD. Quality gates: gofmt, go vet, golangci-lint, `go test -race -count=3 ./...`, coverage at least 90 percent on new code, apidiff additions only.

## 10. Risks and Mitigation
See PRD; plus RB-1 above, `okta` vs archtest vendor-name rule, `clienttest` exposing a short socket, clock alias subtleties.

## 11. Timeline and Milestones
M1 WS-A, WS-B, WS-C in parallel; M2 merge + integrate; M3 docs/ADR/CHANGELOG/deferred/api-compat; M4 review and PR. No tag.

## 12. Decisions Log (open until reviewed)
D-A1 cancellation -> wrapped context error; D-A2 adapter-local TransientError (rate_limited, exit 8); D-A3 not-configured/unauthorized -> exit 3; D-B1 missing/non-array ArrayField -> usage error; D-B2 no separate OffsetBase; D-C1 invalid Extra keys rejected; D-C2 schema version stays 1; D-C3 no body prefix on error types; D-C4 resolve Record comparability; D-A4 empty provider rejected locally; D-C5 strict ancestor rule; D-C6 vendor-code body hook applies to 403 only.

## 13. References
- PRD: `specs/261003-core-v0-2-daemon-adapter/core-v0-2-daemon-adapter-PRD.md`
- `agent-okta-d/pkg/client` (read-only), `testdata/api.txt`, `clienttest`
- `snow-cli/docs/core-change-requests.md`, `outlook-cli/docs/requested-core-changes.md`, `teams-cli/docs/requested-core-changes.md`
- Prior spec: `specs/archive/261003-agent-cli-core/`

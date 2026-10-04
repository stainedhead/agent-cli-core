# Architectural decision record

One entry per decision: context, decision, consequences. Status is "accepted" unless noted.

## ADR-1: Its own repository, a library, no vendor code

Context: three CLIs (`snow`, `outlook`, `teams`) must behave identically to an LLM harness. Decided 2026-10-03.
Decision: shared behavior lives in `github.com/stainedhead/agent-cli-core`. It is a library (no binary, no container image). It contains no vendor clients and knows nothing of ServiceNow, Graph or Teams.
Consequences: the CLIs stay separate binaries in separate repositories that import this module. `internal/archtest` fails the build if a vendor name appears in an import path, identifier or file path.

## ADR-2: `auth` defines its own `DaemonClient`; `agent-okta-d` is not imported by `auth`

Status: amended in v0.2.0 (see ADR-16). `auth` still does not import the daemon client; the adapter now exists as `auth/oktad`.

Context: the daemon's `pkg/client` has no tagged release and its Go surface (types, how `reauth_required` is signalled) is unconfirmed. Importing it would block compilation and couple the public API to an unknown.
Decision: `auth` declares `TokenSource`, `Refresher` and `DaemonClient` (`Fetch`, `Refresh`) and the error vocabulary (`ErrReauthRequired`, `ErrRevoked`, `*UnreachableError`). A fake lives in `auth/authtest`. In v0.1.0 `go.mod` had no `agent-okta-d` requirement.
Consequences: the library compiled and was fully tested without the daemon module. The adapter over `pkg/client` is the single integration point: it implements `auth.DaemonClient`, is the only importer of `pkg/client`, and maps the daemon's signals to the sentinel errors. It shipped as `auth/oktad` in v0.2.0 (ADR-16); a consumer no longer needs to write its own.

## ADR-3: Fake daemon in `auth/authtest`

Context: tool authors need a fake credential daemon and resource server in their tests.
Decision: ship it as an exported sub-package `auth/authtest`, not in `internal/`, so other modules can import it.
Consequences: it is public API under semver. It is test support only and holds no real credentials.

## ADR-4: `auth` and `httpx` meet through a structural `TokenRefresher`

Context: a 401 must trigger exactly one refresh and one resend, and the retry budget is owned by `httpx`; `auth` must not import `httpx` and vice versa.
Decision: `httpx` declares `TokenRefresher { Authorize(ctx, *http.Request) error; Refresh(ctx) error }`. `*auth.Authorizer` has exactly those methods, so it satisfies the interface with no glue. `Authorize` sets the `Authorization` header itself, so a token never leaves `auth` except into that header. The signature is frozen; changing it is a breaking change.
Consequences: no import edge between the packages. `internal/integration` asserts the satisfaction at compile time. The `Authorizer` does not cache; the daemon owns caching.

## ADR-5: `internal/redact` is a leaf package

Context: the PRD places the redactor in `httpx`, but `output` and `audit` also need it, and `output` must not import `httpx`.
Decision: the redactor lives in `internal/redact`, with `internal/clock` as the other leaf. `output`, `httpx`, `auth` and `audit` import it; it imports nothing in the module.
Consequences: one scrubbing implementation for envelopes, traces, errors and audit lines. It is not public, so consumers pass literal secrets through `output.Options.Secrets` and `audit.WithSecrets`. `httpx.Config.Redactor` cannot be constructed outside the module and is effectively internal. `httpx` additionally scrubs the exact `Authorization` values it sent during a call, because a server that echoed the credential into another header leaked through pattern-only redaction (found by the leak fuzz test).

## ADR-6: Import graph enforced by a test

Decision: the allowed intra-module edges are fixed (v0.1.0 list; v0.2.0 additions are below; `output` to `redact`; `httpx` to `output`, `redact`, `clock`; `auth` to `output`, `redact`, `clock`; `policy` to nothing internal; `audit` to `redact`, `clock`; `selftest` and `docgen` to `output`) and checked by `internal/archtest` along with cycles and the third-party allow-list.
Consequences: in v0.1.0 `policy` could not import `output`, so its errors did not implement `output.CategoryError` and the caller mapped them. v0.2.0 adds the `policy -> output` edge for `policy.TrustError` only (ADR-18); the other policy errors still carry no category. `policy` declares its own one-method `Clock`; `audit` carries the policy decision as a plain string.

v0.2.0 graph additions: `clock` (public leaf), `internal/clock -> clock`, `audit -> clock`, `policy -> output`, `auth/oktad -> auth, output` (the only importer of `agent-okta-d/pkg/client`).

## ADR-7: YAML library `github.com/goccy/go-yaml`

Context: the policy must fail closed on unknown or duplicate keys.
Decision: use `goccy/go-yaml` (v1.19.2) with strict decoding. `gopkg.in/yaml.v3` also has `KnownFields(true)` but is archived upstream.
Consequences: one third-party dependency, used only by `policy`. Everything else is the standard library.

## ADR-8: Policy is a guardrail; `dry_run_only` fails safe

Decision: a policy decision is plain data and never claims the server will allow the action. `dry_run_only` decisions have `Allowed=false` and are recognized by `DryRunOnly()`, so a caller checking only `Allowed` never performs a write by mistake. Deny rules win regardless of order; unmatched requests are denied (`default-deny`). Only allowed decisions consume rate budget. The writable-file check warns by default and refuses on request.

## ADR-9: Audit has no body field and a per-Logger failure mode

Decision: `audit.Record` has no field that can hold a body or credential; text is redacted and capped at 512 bytes. The write-failure mode (`warn` or `block`) is a Logger setting because the library cannot classify verbs as reads or writes.
Consequences: the PRD's proposed "block write operations" default is achieved by the tool choosing a mode per call or using two Loggers.

## ADR-10: Retry semantics in `httpx`

Decision: `MaxRetries` is one budget for transient retries and the single 401 refresh. Statuses 429, 502, 503, 504 and network errors become `*RateLimitedError` (exit 8) when the budget is spent or the request is not eligible for retry. `Retry-After` replaces the backoff and is capped by `MaxWait`, without jitter. The vendor error code of a 403 comes only from response headers through `Config.VendorCode`, so the body is never read. Statuses such as 404, 409 and 500 pass through as responses.

## ADR-11: Exit codes are an API contract

Decision: the envelope shape, exit codes and category names are pinned by golden tests (API-8). Failure envelopes carry no data, so a failing selftest matrix is a `general` failure whose message names the failing rows.

## ADR-12: Platform targets

Decision: darwin/arm64, linux/amd64, linux/arm64; no native Windows. Windows users run the Linux build under WSL2, which is Linux. The writable-file check is build-tagged for Unix.

## ADR-13: Deferrals

Decided in the feature spec (2026-10-03):

- Human-mode PKCE login and OS keychain: belongs in `snow-cli`; the `TokenSource` interface only has to allow it.
- Conformance test kit: needs real consumers; revisit after the first release.
- `apidiff` wiring: documented as API-7, not enforced; no earlier release to compare with and blocking-versus-advisory is undecided.
- Release automation (tagging, GitHub Release, SBOM, provenance, signing) and the downstream compatibility build: no consumer code or tag exists, so CI is verify-only.
- The `pkg/client` adapter: delivered in v0.2.0 as `auth/oktad` (ADR-16). Superseded; no longer deferred.
- Policy file signature check (P2).

## ADR-14: Open items

- Who owns the policy schema across the tools (a generic schema ships; ownership is unassigned).
- First tagged `agent-okta-d` release containing `pkg/client`: resolved, `v0.1.0` is required by `go.mod` (ADR-16).
- Tagging `agent-cli-core` v0.2.0: pending; v0.2.0 is unreleased until the tag is cut.
- The shared-skill update in the root repository is a manual pull request per release.

## ADR-15: Hosts are allow-listed, redirects checked

Context: auto-review found a bearer token could follow a cross-host redirect or travel over plain http.
Decision: `httpx` pins hosts (`AllowedHosts`, default first request's host), refuses non-loopback plain http unless `AllowInsecureHTTP`, and installs `CheckRedirect` in `NewClient`; the transport also checks every request, so custom clients are covered. Redaction of bare `bearer`/`basic` became context-sensitive and a 40-hex git SHA-1 is preserved. `auth.Token` stores its value in a closure so reflection-based printing cannot reveal it.
Consequences: a legitimate cross-host redirect needs the target in `AllowedHosts`; short all-letter bare credentials outside an `authorization` context are no longer pattern-redacted (register them as literal secrets).

## ADR-16: `auth/oktad` adapter and its error mapping (v0.2.0)

Status: accepted (v0.2.0, unreleased). Supersedes the "not imported" part of ADR-2; decisions D-A1 to D-A4 of the v0.2.0 spec.

### Context
`auth/oktad` implements `auth.DaemonClient` over `agent-okta-d` v0.1.0 `pkg/client`. Its errors must land in the core's existing categories and exit codes (`output.CategoryError`), with no new category and no new exit code. Exit 4 (`forbidden`) means a remote server refused a request; exit 8 (`rate_limited`) means "rate limiting or a transient failure" (`output/exit.go`).

### Decision
Classification is evaluated top to bottom with `errors.Is` / `errors.As`:

| # | Condition (agent-okta-d) | Result | Category | Exit |
|---|---|---|---|---|
| 1 | caller's `ctx.Err() != nil` | `fmt.Errorf("oktad: %w", ctx.Err())` (never unreachable) | general | 1 |
| 2 | `ErrDaemonUnavailable` | `*auth.UnreachableError{Socket, Err}` | auth | 3 |
| 3 | `ErrReauthRequired` (401) | `auth.ErrReauthRequired`, cause also wrapped; `DaemonTokenSource` turns it into `*auth.ActionRequiredError` | auth | 3 |
| 4 | `ErrRevoked` (403 revoked) | `auth.ErrRevoked`, same as row 3 | auth | 3 |
| 5 | `ErrDegraded` (503), or any `*APIError` with a retry hint | `*oktad.TransientError` | rate_limited | 8 |
| 6 | `ErrNotConfigured` (404) | `*oktad.AccessError` | auth | 3 |
| 7 | `ErrUnauthorized` (403) | `*oktad.AccessError` | auth | 3 |
| 8 | `ErrInvalidResponse`, other `*APIError` without hint, anything else | `fmt.Errorf("oktad: %w", err)` | general | 1 |
| - | empty provider name, zero-value `Client` | local plain error, no socket call | general | 1 |

Reasons:
- Exit 8 for degraded: the daemon is up but cannot serve now and says when to retry, which is the "transient failure" meaning of exit 8. `*TransientError` is adapter-local so `auth` need not import `httpx`.
- Exit 3 (not 4) for not-configured and unauthorized: both are local credential-setup problems a human fixes, like reauth and revoked; exit 4 is reserved for a server 403 on the vendor API. A bare 403 stays fail-closed (`ErrUnauthorized`).
- Context errors are reported as such: a cancelled caller is not an unreachable daemon.

### Retry-after exposure
`TransientError` has `RetryAfter() time.Duration` (the daemon's hint, zero if none) and a `Hint()` that names the wait in whole seconds (rounded up), so `output.FromError` puts it in `error.hint`. `output.Error` has no `retry_after` field in v0.1.0, and adding one is outside this workstream; consumers read the number with `errors.As` or the structural `interface{ RetryAfter() time.Duration }`. The cause chain keeps `client.ErrDegraded` and `client.RetryAfter(err)` working.

### Deviation from the data dictionary
Spec/data-dictionary list `TransientError{RetryAfter time.Duration; Err error}` together with a `RetryAfter()` accessor; Go cannot have both a field and a method of that name. The field is named `Wait` and the method is `RetryAfter()`. The implementation follows this ADR.

### New exported API
`oktad.Client`, `oktad.New`, `oktad.Option`, `oktad.WithSocketPath`, `oktad.WithTimeout`, `(*Client).Fetch/Refresh/SocketPath/Close`, `oktad.TransientError` (+`RetryAfter`, `Hint`, `Category`, `Unwrap`, `Error`), `oktad.AccessError` (+`Hint`, `Category`, `Unwrap`, `Error`).

### Architecture test
`internal/archtest` gains `Rules.VendorExempt` (internal, test support): `auth/oktad` is exempt from the vendor-name rule (directory name, identifiers and the `agent-okta-d` import path), is allowed to import `auth` and `output`, and is the only package allowed `github.com/stainedhead/agent-okta-d/pkg/client`. `auth` still cannot import it.

### go.mod
`require github.com/stainedhead/agent-okta-d v0.1.0`, resolved from the Go proxy, no `replace`. Consumers of v0.2.0 gain the module in their graph.

## ADR-17: Output continuation token, object-array bounding, nested docgen (v0.2.0)

Status: accepted (v0.2.0, unreleased).
Requested by snow (CR-03, CR-04, CR-12), outlook (items 2, 13, 18, 9), teams (items 5, 6, 9).

### Context
Graph paging uses opaque tokens, and list commands want an object envelope (`{items, total, ...}`) that can still be bounded; v0.1.0 returned `ErrBoundTooSmall` for any object over the limit. Nested commands (`mail send`) were named with a space in a flat list.

### Decisions
1. R1: `Meta.NextPageToken string` (`next_page_token`, omitempty), last field of `Meta`, so existing marshalling is byte-identical. Opaque, passed through, counted in the byte budget, never cut; the footer of table/text shows `next_page_token=` (control characters escaped) after `request_id`. Remains set when output is truncated: the contract is "resume with `next_offset` first; use the token once `truncated` is false". Caller must supply a non-secret value.
2. R2: `Bounds.ArrayField string`. Data must be a JSON object holding that top-level key as an array (null counts as empty and stays null). The object is split with a token decoder, so key order and the bytes of all other fields are preserved (map and struct data alike). Offset skips that many items; `Meta.Count` is the items kept; `NextOffset = Offset + kept`, an absolute item index exactly like array data, which satisfies snow CR-03 without an `OffsetBase` option (D-B2). `Offset == len` gives an empty page; `Offset > len` returns `ErrOffsetOutOfRange` (same as array data; the spec's "empty page" wording for offset beyond the end was replaced by consistency with existing semantics). Missing key, non-array value, or non-object data returns new `ErrArrayField` (usage category, exit 2; D-B1). A single item too large for the budget returns `ErrBoundTooSmall`. Only top-level keys; no paths. Unset field is byte-identical to v0.1.0 (existing goldens unchanged). Failure envelopes ignore it.
3. R4: `Command.Subcommands []Command` and `docgen.MaxDepth = 4`. Siblings are sorted by name and rendered depth-first under the parent, heading level 3 + depth - 1, titled with the full path; a parent renders its own description/usage/examples/forbidden and then its children (answers the TBD). Validation (validation category): non-empty, no control characters, names unique among siblings after trimming, depth at most 4. Same name under different parents is allowed. Empty `Subcommands` equals a leaf; flat output is byte-identical (existing golden unchanged). `Usage`/`Examples` remain caller text and should carry the full invocation.

### Consequences
New exported symbols: `output.Meta.NextPageToken`, `output.Bounds.ArrayField`, `output.ErrArrayField`, `docgen.Command.Subcommands`, `docgen.MaxDepth`. Adding fields breaks unkeyed `Meta{}`/`Bounds{}`/`Command{}` literals (none in repo). Not delivered: snow CR-12 (honouring caller-set Truncated for object data without ArrayField) is subsumed by ArrayField; a separate OffsetBase is deferred.

## ADR-18: Public clock, audit extension fields, trusted-file check, httpx body vendor code (v0.2.0)

Status: accepted (v0.2.0, unreleased). Decisions D-C1 to D-C6 of the v0.2.0 spec, as implemented.

### R3: public clock (FR-007)
- New leaf package `clock` (Clock, System, Fake, NewFake) is the single implementation. `internal/clock` keeps its names as type aliases and a `NewFake` wrapper, so the types are identical and every internal import compiles unchanged.
- `audit` now imports the public `clock` directly so `audit.WithClock(c clock.Clock)` shows an importable type in docs. `policy.Clock` and `httpx.Clock` stay declared as they were; public types satisfy them structurally. Consumers (outlook 6/14, teams 7) can implement or reuse the clock.
- `audit.WithClock(nil)` keeps the system clock (previously a nil-pointer panic at the first Log).
- archtest: `clock` added as a leaf; `internal/clock` may import `clock`; `audit` may import `clock`.

### R5: audit Extra, RuleID, TargetRef (FR-009; D-C1, D-C2, D-C4)
- `Record` gains `RuleID`, `TargetRef` (omitempty strings) and `Extra *ExtraFields` (`type ExtraFields map[string]string`), written after `policy_decision` as `rule_id`, `target_ref`, `extra`.
- D-C4 (RB-1) resolved: a plain `map[string]string` field would make `Record` non-comparable, which breaks `==` for any caller and an existing v0.1.0 test already does that (`got != r`). The v0.2 release is additive-only, so `Extra` is a pointer to a named map type: `Record` stays comparable and callers write `Extra: &audit.ExtraFields{"k": "v"}`. A nil or empty Extra is omitted. Alternative rejected: a plain map field (incompatible per apidiff).
- D-C1: more than 16 keys (`MaxExtraKeys`) or a key outside `[a-z0-9_.-]{1,32}` is rejected: Log writes nothing and returns a `*WriteError` that matches both `ErrWrite` and the new `ErrInvalidExtra`. Values go through the redactor and are cut to 256 bytes (`MaxExtraValueLen`) on a rune boundary. `RuleID` and `TargetRef` use the existing redact+512-byte field cap. The caller's map is never mutated. Keys serialise sorted (encoding/json map order).
- D-C2: `SchemaVersion` stays 1 (additions are optional and omitempty; v0.1.0 output is byte-identical when unset). Readers must tolerate unknown keys.

### R6: standalone trusted-file check (FR-010; D-C5)
- `policy.CheckTrustedFile(path, ...TrustOption) error`, `policy.WithTrustedUIDs(uids ...uint32)`, `*policy.TrustError{Path, Reason, Err}`, `policy.ErrNotTrusted`. `TrustError` implements `output.CategoryError` (policy_denied, exit 6) and `Hinter`. This adds the `policy -> output` edge to the archtest allowed list (output does not import policy; no cycle).
- Trusted owners: root always, plus configured uids. The effective uid is never accepted unless it is root; passing it returns a `*TrustError` (the agent's own user can rewrite its own file). This satisfies outlook 15 and teams 2 ("root or a configured trusted uid, never the effective uid").
- Walk: the path is made absolute and walked component by component with `lstat`; symlinks are resolved by the check itself (owner of each link must be trusted, up to 40 hops, loops fail) and every directory on the real route, "/" included, must be owned by a trusted uid and not group/world writable (`mode&022`). D-C5 strict: a sticky world-writable ancestor such as `/tmp` is rejected.
- Final check of record: `open(O_RDONLY|O_NOFOLLOW|O_NONBLOCK)` then `fstat` on the descriptor (regular file, trusted owner, not group/world writable), plus `os.SameFile` against the earlier `lstat` so a swap between the walk and the check's own open fails. The function returns no descriptor; the caller re-opens the path, so the guarantee after the check is that every ancestor is trusted and not group/world writable (nobody untrusted can change the path unless a trusted user or root does). POSIX ACLs and extended attributes are not examined. A descriptor-returning variant is deferred (docs/deferred.md). Non-regular files are rejected before opening (no FIFO hang).
- Everything fails closed: missing file, EACCES on an ancestor, non-unix platform (`trust_other.go`, build tag `!unix`, always returns a `*TrustError`). `Load` and `WritableMode` are untouched.
- Testability without root: the filesystem is behind a small unexported interface with a fake (arbitrary owners/modes, covers every branch), and the effective-uid lookup is a test seam so a non-root test user can play "trusted owner" on real files under `$HOME` (macOS and Linux). Real-filesystem walk tests skip, with a "SKIPPED real-filesystem trust test" log line, if the home directory chain is not clean; the O_NOFOLLOW/SameFile open tests use a plain temp directory and always run. No test needs chown or root.

### R7: httpx vendor code from body (FR-011; D-C3, D-C6)
- `Config.VendorCodeFromBody func(status int, prefix []byte) string` and `Config.VendorBodyLimit int` (default `DefaultVendorBodyLimit` 4096, cap `MaxVendorBodyLimit` 64 KiB). Applies to 403 only (D-C6); 401 and 429 keep their v0.1.0 typed errors. Precedence: a non-empty header `VendorCode` wins and the body is not read; otherwise the body prefix is read with `io.LimitReader`, the hook is called (skipped on a read error), then the body is drained and closed so connections are reused.
- The result is trimmed, control characters removed, scrubbed (redactor plus held credentials) and cut to 64 bytes on a rune boundary. D-C3: no body prefix is stored on any error type; only the hook sees the body. Hook panics are not recovered (documented). Zero-value Config behaviour is unchanged.
- Covers outlook 1/16/19 and teams 4. The extra "expose Status on typed errors" idea in those requests is already met (RateLimitedError.Status exists; AuthError/Forbidden carry their status by type).

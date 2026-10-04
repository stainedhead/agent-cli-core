# Changelog

## v0.2.0 - 2026-10-04

### Added
- auth/oktad: new package, an `auth.DaemonClient` over the credential daemon's Go client (`New`, `WithSocketPath`, `WithTimeout`, `Fetch`, `Refresh`, `SocketPath`, `Close`). Failures map to existing categories: unreachable, reauth and revoked are exit 3; `*oktad.TransientError` (degraded or retry-hinted, `RetryAfter()`) is exit 8; `*oktad.AccessError` (not configured, unauthorized) is exit 3; a cancelled caller context is reported as such.
- output: `Meta.NextPageToken` (`next_page_token`), an opaque continuation token that is never cut and shown in table/text footers.
- output: `Bounds.ArrayField` and `ErrArrayField`, bounding a top-level array inside object data while keeping the other fields and their order.
- docgen: `Command.Subcommands` and `MaxDepth` (4), nested commands rendered depth-first under their parent.
- clock: public `Clock`, `System`, `Fake`, `NewFake`; `internal/clock` now aliases it, and `audit.WithClock` takes a `clock.Clock` (a nil clock keeps the system clock instead of panicking).
- audit: optional `Record.RuleID`, `Record.TargetRef` and `Record.Extra` (`*ExtraFields`; at most `MaxExtraKeys` keys of `[a-z0-9_.-]{1,32}`, values cut to `MaxExtraValueLen`), `ErrInvalidExtra`. Schema version stays 1; output without them is unchanged.
- policy: `CheckTrustedFile`, `WithTrustedUIDs`, `TrustError` (category `policy_denied`, exit 6) and `ErrNotTrusted`, a fail-closed standalone check of file and ancestor ownership and permissions. `policy` now imports `output` for `TrustError`.
- httpx: `Config.VendorCodeFromBody`, `Config.VendorBodyLimit`, `DefaultVendorBodyLimit`, `MaxVendorBodyLimit`, deriving a vendor code from a bounded prefix of a 403 body.
- user-docs: oktad adapter guide, clock guide, and sections for each new API; ADR-16 to ADR-18.

### Changed
- Source caveat: adding fields to `output.Meta`, `output.Bounds`, `docgen.Command` and `audit.Record` breaks unkeyed composite literals of those types; use keyed literals. None exist in this repo or in snow-cli, outlook-cli and teams-cli at the time of writing (checked by grep). Otherwise v0.2.0 is additions only.
- The adapter is no longer deferred (milestone M0a done). `go.mod` now requires `agent-okta-d` v0.1.0, so consumers gain it in their module graph.

### Dependencies
- Added `github.com/stainedhead/agent-okta-d` v0.1.0 (used only by `auth/oktad`).

## Unreleased - auto-review remediation

### Security
- httpx: requests and redirects are limited to `Config.AllowedHosts` (default: the first request's host); plain http to non-loopback hosts needs `Config.AllowInsecureHTTP`. Violations return `*httpx.ForbiddenHostError` (`auth/forbidden-host`, exit 4) before any credential is attached. `NewClient` installs a redirect check. Behavior change: a cross-host redirect that used to be followed is now refused; add the host to `AllowedHosts`.
- auth: `Token` keeps its value in a closure, so it no longer prints through fmt when held in an unexported struct field.
- httpx: send errors and `RateLimitedError.Err` are scrubbed (URL query, userinfo, configured redactor, held credentials).

### Added
- output: `FromErrorWithSecrets` and `FailureWithSecrets` remove literal secrets from error envelopes.
- httpx: `ForbiddenHostError`, `ForbiddenHostCode`, `Config.AllowedHosts`, `Config.AllowInsecureHTTP`.

### Changed
- redaction: bare "bearer"/"basic" in prose is kept unless a credential follows or an `authorization` key precedes it; a 40-hex git SHA-1 is preserved; the opaque-run threshold is exported as `OpaqueRunMin` (40). Short all-letter bare credentials outside an authorization context are no longer pattern-redacted; register them as literal secrets.
- output: the error hint is found with `errors.As`, so hints inside `errors.Join` are kept.

### Fixed
- policy: hit timestamps are recorded only for `per_hour` limits, so per-run-only policies no longer grow memory.

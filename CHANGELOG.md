# Changelog

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

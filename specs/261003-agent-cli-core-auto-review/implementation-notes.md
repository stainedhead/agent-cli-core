# Implementation Notes: agent-cli-core Auto-Review Remediation
Date: 261003

Purpose: record decisions, edge cases and deviations during implementation. Update after each task.

## Technical Decisions
### fixes
- OQ-2 (FR-005): git SHA-1 (exactly 40 lowercase hex) is preserved; threshold stays 40 and is exported as `redact.OpaqueRunMin`; longer hex and mixed-case 40+ runs are redacted. Literal registered secrets are still removed regardless.
- FR-005 auth scheme: after an `authorization` key any credential is redacted; a bare `bearer`/`basic` needs a digit/symbol word of 4+ chars or a letters-only word of 8+ chars with an inner uppercase letter. Trade-off: short all-letter bare credentials are no longer redacted outside an authorization context.
- OQ-3 (FR-003): query string, userinfo and fragment are always stripped from `*url.Error` text in transport errors (host and path stay); the configured redactor and held credentials are then applied. The raw error remains in the chain for errors.Is/As (message safe, value reachable programmatically). `RateLimitedError.Error()` also scrubs `Err` with built-in patterns when built outside the Transport. Errors returned by http.Client after the transport (its own *url.Error wrapper) are outside this scrub.
- FR-004: `FailureWithSecrets`/`FromErrorWithSecrets` added; `Failure`/`FromError` delegate. Write already rescrubbed with `Options.Secrets`; both paths are tested.
- FR-006: `findHinter` uses `errors.As`; the first Hinter in depth-first order wins.
- Deviation: Stream C touched two lines of `httpx/transport.go` (separate commit) to wire the scrubbed errors.
## Edge Cases & Solutions
## Deviations from Plan
## Lessons Learned

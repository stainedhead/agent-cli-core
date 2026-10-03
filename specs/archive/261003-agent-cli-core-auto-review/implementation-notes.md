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
- Integration: `errors_test.go` send-error tests use https URLs (plain http to a non-loopback host is now refused). A host refused in `CheckRedirect` is traced like one refused in `RoundTrip`.
## Edge Cases & Solutions
## Deviations from Plan
## Lessons Learned

## Root skill update needed
Compare of /Users/iggybdda/Code/stainedhead/agentic-teams/skills/agent-cli-core.md against the built library (not edited here):
- Status banner "planned, not built / no code and no release" is stale: the library exists (output, auth, policy, audit, httpx, selftest, docgen) and is verified by tests; drop the "planned" wording and the PRD-draft references.
- Section 1 envelope: matches (`ok`, `data`, `meta.truncated/next_offset/count/request_id`, `error.code/message/hint`). Add: `request_id` is omitted when empty, `hint` is omitted when empty, failure envelopes are never truncated and carry no `data`; error `code` values are the category names (ok, general, usage, auth, forbidden, not_found, policy_denied, conflict, rate_limited, validation).
- Section 2 exit codes 0-9: values match. Add exit 4 now also covers `auth/forbidden-host` (request or redirect to a host outside AllowedHosts, or plain http to a non-loopback host): final, do not retry or work around it. Exit 6 also covers policy rate-limit denials. Exit 8 includes 502 and 504 and network errors, not only 429/503.
- Section 5 paging: the offset is `Bounds.Offset` in the library (a byte/element index returned as `meta.next_offset`); the CLI flag name is tool-specific. An offset past the end is a usage error (exit 2). Failure envelopes are never truncated.
- Section 7 idempotency: the library has no idempotency-key helper. Retry of POST/PATCH is opt-in per request via `httpx.MarkSafeToRetry(req)`; reword to "writes are retried only when the tool marked them safe (for example, it sent an idempotency key the server honors)". Only idempotent methods (GET, HEAD, OPTIONS, TRACE, PUT, DELETE) retry by default; one shared retry budget (default 3) covers retries and the single 401 refresh.
- Section 4 credentials: add that the client refuses to send credentials to any host other than the allow-listed one(s) and refuses plain http (loopback excepted); error text and traces never contain query strings or userinfo.
- Section 6 policy: add that matching deny always wins, unmatched requests are denied (`default-deny`), and `dry_run_only` yields `Allowed=false` with a preview.
- Links: user-docs/ now has per-package guides; link to README.md and CHANGELOG.md.

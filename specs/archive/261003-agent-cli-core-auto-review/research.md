# Research: agent-cli-core Auto-Review Remediation
Date: 261003 | Source PRD: specs/261003-agent-cli-core-auto-review/agent-cli-core-auto-review-PRD.md

## Research Questions
1. OQ-1: Default host policy: first-request host (assumed) vs required explicit allow-list? Keep http to loopback allowed for tests (assumed yes)? How does net/http (current go version) treat Authorization on redirect and does CheckRedirect see the original request chain?
2. OQ-2: Preserve git SHA-1 (40 hex) in reOpaque, or raise threshold to 41+? Impact on audit Resource and error text.
3. OQ-3: Strip query from url.Error text always, or document only?
4. Which Token representation (pointer to secret type vs closure) keeps fmt, slog, JSON and text marshalling redacted, including `%x` and `%#v` on unexported fields, without breaking comparison/copy semantics?
5. Can redaction in httpx errors use Config.Redactor without importing output (archtest import graph)?

## Industry Standards
[TBD] (RFC 9110 sec 15.4 redirects; curl/Go behavior on credentials across redirects)
## Existing Implementations
[TBD]
## API Documentation
[TBD] (net/http Client.CheckRedirect, errors.As)
## Best Practices
[TBD]
## Open Questions
OQ-1, OQ-2, OQ-3 above.
## References
- Source PRD in this directory

# Research - agent-cli-core
Date: 2026-10-03 | Source PRD: agent-cli-core-PRD.md

## Research Questions
1. Which YAML library gives strict (unknown key is an error) parsing and is maintained? (CORE-POL-8; e.g. `gopkg.in/yaml.v3` with `KnownFields(true)` or a maintained fork.) Decided by WS-B; recorded in ADR.
2. How do `httpx` and `auth` share the single 401-refresh budget without importing each other? (CORE-HTTP-6, CORE-AUTH-4) Proposed: structural `httpx.TokenRefresher`.
3. What will `pkg/client` look like once tagged (errors, `reauth_required` signalling)? Gates M0a only (Q5, CORE-AUTH-10); unconfirmed.
4. Is a small conformance kit feasible (Q6)? Estimate once `output` golden fixtures exist.
5. Which API-compat tool (apidiff vs gorelease) and block vs advise (Q11, API-7)? Docs only now.
6. Does `golangci-lint` plus the `go 1.27` toolchain work in CI for all three targets?

## Industry Standards
[TBD] Retry-After semantics (RFC 9110); JSON Lines.
## Existing Implementations
[TBD]
## API Documentation
[TBD] Daemon socket API `GET /v1/credentials/{provider}`, `POST /v1/credentials/{provider}/refresh` (from the daemon PRD, not re-verified).
## Best Practices
[TBD]
## Open Questions
See spec.md section 12.
## References
- agent-cli-core-PRD.md

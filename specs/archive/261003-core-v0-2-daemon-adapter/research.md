# core-v0-2-daemon-adapter - Research

Created: 2026-10-03 | Source PRD: core-v0-2-daemon-adapter-PRD.md

## Research Questions
1. Does `agent-okta-d` v0.1.0 resolve via the module proxy, and does `clienttest` allow a short socket path? (answers A1/A2)
2. Does `internal/archtest` flag `okta` in an import path or identifier?
3. Do snow/outlook/teams use unkeyed literals or `==` on `audit.Record`, `output.Meta`, `output.Bounds` or `docgen.Command`? (RB-1)
4. What are the exact `output` fit semantics for `Offset` on arrays, to mirror them for `ArrayField`?
5. Are policy-file tests feasible for root-owned cases in CI without root?

## Industry Standards / Existing Implementations / API Documentation / Best Practices
Placeholders: `pkg/client/testdata/api.txt`; consumer request docs; apidiff docs.

## Open Questions
See spec section 12.

## References
PRD, request docs.

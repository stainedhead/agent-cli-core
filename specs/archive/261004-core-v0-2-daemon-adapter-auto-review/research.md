# core-v0-2-daemon-adapter-auto-review - Research

Created: 2026-10-04 | Source PRD: `specs/261004-core-v0-2-daemon-adapter-auto-review/core-v0-2-daemon-adapter-auto-review-PRD.md`

## Research Questions
1. Does pkg/client return a context error when the caller ctx is already cancelled and the socket is dead? (FR-003)
2. Which redactor entry point can test a bare key string without side effects? (FR-002)
3. Does the redactor change any 32-char lowercase hex string, or only registered secrets and known token patterns? (FR-002)
4. Is the Linux CI runner home chain clean enough for the real-FS trust tests? (FR-006)
5. Do snow/outlook/teams use unkeyed literals of Meta, Bounds, Command or Record? (FR-007)

## Industry Standards
## Existing Implementations
## API Documentation
## Best Practices
## Open Questions
Q1 reject vs drop; Q2 OpenTrustedFile; Q3 as question 1.
## References

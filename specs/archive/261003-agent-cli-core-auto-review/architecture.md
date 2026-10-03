# Architecture: agent-cli-core Auto-Review Remediation
Date: 261003 | Status: Draft

## Architecture Overview
Existing layering retained: internal/redact and internal/clock are leaves; output is the shared contract; archtest guards imports.
## Component Architecture
Host policy (httpx), secret holder (auth), redaction pipeline (internal/redact, httpx/errors, output), hit recorder (policy).
## Layer Responsibilities
[TBD]
## Data Flow
Request -> host check -> Authorize -> send -> redirect check (CheckRedirect) -> error scrub via Redactor -> envelope.
## Sequence Diagrams
[TBD]
## Integration Points
auth <-> httpx (Authorize per attempt); httpx <-> output (errors/categories); audit.Handle errors.Join.
## Architectural Decisions
- AD-1: Default allowed host = first request host; loopback http allowed (OQ-1).
- AD-2: Additive *WithSecrets APIs rather than signature changes.

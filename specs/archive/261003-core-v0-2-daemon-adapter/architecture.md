# core-v0-2-daemon-adapter - Architecture

Created: 2026-10-03 | Status: Draft

## Architecture Overview
Edge adapter `auth/oktad` implements the inward-defined `auth.DaemonClient`; `auth` has no dependency on it or on agent-okta-d. Public leaf package `clock` is consumed by audit/policy/httpx via their existing interfaces.

## Component Architecture / Layer Responsibilities
See spec sections 4 and 6.

## Data Flow
caller -> `auth` TokenSource -> `oktad.Client` -> `pkg/client` -> unix socket -> daemon; errors map back per spec FR-004.

## Sequence Diagrams
Fetch: ctx -> client.Credential -> Secret.Reveal -> auth.NewToken. Failure: classify -> core error.

## Integration Points
`agent-okta-d` v0.1.0; consumers snow/outlook/teams via CI downstream job.

## Architectural Decisions
D-A1..D-C4 in spec section 12; ADRs written in step 6.

# Architecture - agent-cli-core
Date: 2026-10-03 | Status: Draft

## Architecture Overview
Library only. Flow: `policy.Check -> auth.TokenSource.Token -> httpx.Do -> output.Envelope/Exit -> audit.Record`. The tool owns vendor requests and parsing.

## Component Architecture
Packages: output, auth (+authtest), policy, audit, httpx, selftest, docgen; internal/redact, internal/clock, internal/archtest.

## Layer Responsibilities
Leaf (internal/redact, internal/clock) -> contract (output) -> behavior (httpx, auth, policy, audit) -> tooling (selftest, docgen). Adapters (future `pkg/client` adapter) sit at the edge behind `auth.DaemonClient`.

## Import graph (acyclic; enforced by internal/archtest)
output -> redact; httpx -> output, redact, clock; auth -> output, redact, clock; policy -> yaml; audit -> redact, clock; selftest -> output; docgen -> output. `auth` and `httpx` never import each other; they meet through `httpx.TokenRefresher`. Errors reach exit codes through `output.CategoryError` (structural), not by importing the producing package.

## Data Flow / Sequence
401: httpx sends with Token -> 401 -> `Refresh` once -> resend once -> second 401 -> category auth (exit 3). 429/503: wait min(Retry-After, MaxWait) on the injected clock, bounded attempts, then category rate_limited (exit 8). One attempt budget is shared.

## Integration Points
Daemon via `auth.DaemonClient` (fake now, adapter later). Tools supply provider name, verbs/resources, command tree, selftest matrix, untrusted field list.

## Architectural Decisions
Recorded in `docs/architectural-decision-record.md`: D1-D5 from the PRD plus authtest placement, TokenRefresher decoupling, internal/redact leaf, YAML library choice, deferrals.

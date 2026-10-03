# Plan - agent-cli-core
Date: 2026-10-03 | Status: Planning

## Development Approach
TDD, clean architecture, fakes for everything external. Work is split into workstreams with exclusive file ownership so parallel workers never touch the same path. A worker edits ONLY paths in its own ownership row (plus its own section in shared docs, see Shared files). Anything needed from another workstream is requested via an interface already fixed by WS0, not by editing that package.

## Phase Breakdown

### Phase 1 - WS0 Foundation (sequential; must merge before Phase 2 starts)
Owner of: `output/**`, `internal/redact/**`, `internal/clock/**`, `internal/archtest/**`, `go.mod`/`go.sum` (initial), `.golangci.yml`, `Makefile`, and stubs of the shared docs (below).
Delivers: envelope, exit codes 0..9 + `Category`/`CategoryError`/`ExitFor`, untrusted marking, bounds, formats, golden tests, `redact`, `clock` (+Fake), archtest (import graph + no vendor names), YAML dependency added to `go.mod` (so policy never needs to), empty-package `doc.go` stubs for `auth`, `auth/authtest`, `policy`, `audit`, `httpx`, `selftest`, `docgen` (so `go build ./...` and the import graph exist), and `docs/technical-details.md` skeleton with one pre-created `## API: <pkg>` section per package plus `user-docs/<pkg>.md` stubs.
Exit: M0 + M1 acceptance; the contracts other workstreams consume are frozen.

### Phase 2 - parallel workstreams (start after Phase 1 merge; mutually independent)
| WS | Package | Exclusive ownership | May import | Depends on |
|---|---|---|---|---|
| WS-A | auth | `auth/**` (incl. `auth/authtest/**`), `user-docs/auth.md`, section `## API: auth` | output, internal/redact, internal/clock | WS0 |
| WS-B | policy | `policy/**`, `user-docs/policy.md`, section `## API: policy`. Sole editor of `go.sum` entries for YAML if `go mod tidy` demands (go.mod already updated by WS0) | stdlib, yaml | WS0 |
| WS-C | audit | `audit/**`, `user-docs/audit.md`, section `## API: audit` | internal/redact, internal/clock | WS0 |
| WS-D | httpx | `httpx/**`, `user-docs/httpx.md`, section `## API: httpx` | output, internal/redact, internal/clock | WS0 |
| WS-E | selftest | `selftest/**`, `user-docs/selftest.md`, section `## API: selftest` | output | WS0 |
| WS-F | docgen | `docgen/**` (incl. `docgen/testdata/**` goldens), `user-docs/docgen.md`, section `## API: docgen` | output | WS0 |

Rules for every Phase 2 worker: do not edit `go.mod`, `go.sum` (except WS-B above), `Makefile`, CI, ADR, README, AGENTS.md, other packages, or other workers' doc sections; each package includes `Example*` tests and a `doc.go`; run the full quality gate for its own package; commit only its own paths; rebase on the branch before push (distinct paths guarantee no conflicts). `auth` and `httpx` stay import-independent (see architecture.md).

Cross-package contract (frozen by WS0 in `httpx/doc.go` and `auth/doc.go`): `httpx.TokenRefresher` is `interface { Authorize(ctx, *http.Request) error; Refresh(ctx) error }`. `Authorize` sets the Authorization header itself, so the token never leaves `auth`'s redacting type except into the header. WS-A provides a type with exactly these methods (`auth.NewAuthorizer(TokenSource, DaemonClient-backed refresh)`); WS-D consumes the interface only. If the signatures drift, WS-I writes the bridge in `examples/`.

### Phase 3 - WS-I Integration (after all of Phase 2)
Owner of: `examples/**`, `.github/workflows/**`, `internal/integration/**` (cross-package tests incl. fuzz/property no-token-leak test), `docs/**` (final pass, ADR, product docs), `README.md`, `user-docs/README.md` and index, `go.mod`/`go.sum` tidy, `AGENTS.md` and `CLAUDE.md` layout lines, DEV-FLOW docs.
Delivers: sample tool end to end (`policy -> auth -> httpx -> output -> audit`, selftest matrix, docgen SKILL.md), CI per BLD-1..6 (gofmt, tidy diff, vet, pinned golangci-lint, `-race` tests, govulncheck, three-target compile), ADR entries, technical-details finalization, apidiff documented (not wired), optional I6 conformance kit if small.

## Critical Path
WS0 -> (max of A..F) -> WS-I. WS-D and WS-A are the longest Phase 2 items.

## Testing Strategy
Per PRD section 8: golden (output, audit, selftest, docgen), table-driven (policy), fake daemon/clock/HTTP server (auth, httpx), fuzz/property for no token in any output (WS-I), archtest for dependency rule and import graph (WS0, re-run by WS-I).

## Rollout Strategy
Library only: merge to branch; no release in this spec (CD, signing, SBOM deferred). First tag `0.1.0` is a later step.

## Success Metrics
All FR rows covered by tests; quality gates green; three-target compile passes; every package has Example tests; technical-details covers all exported API.

## Shared files (conflict avoidance)
- `docs/technical-details.md`: skeleton by WS0; each worker edits only its `## API: <pkg>` section; WS-I does the final pass.
- `docs/architectural-decision-record.md`: WS0 and WS-I only. Workers record decisions under their own `### <pkg>` heading in `implementation-notes.md`; WS-I promotes them to the ADR.
- `specs/261003-agent-cli-core/status.md` and `tasks.md`: the orchestrator updates; workers report completion in their final message.

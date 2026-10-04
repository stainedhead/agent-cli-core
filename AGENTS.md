# AGENTS.md

Rules for AI agents and human contributors working in this repository.

## Project summary

`agent-cli-core` is a Go library (module `github.com/stainedhead/agent-cli-core`, no binary) holding the behavior shared by the agent-facing CLIs `snow`, `outlook` and `teams`: token acquisition from the `agent-okta-d` daemon (`auth`, with `auth/oktad` wrapping the daemon's `pkg/client`), client-side policy (`policy`), the response envelope, exit codes and untrusted-content marking (`output`), the audit log (`audit`), HTTP retry and redaction (`httpx`), a self-test runner (`selftest`) and harness skill-document generation (`docgen`). It contains no vendor clients: it knows nothing of ServiceNow, Microsoft Graph or Teams.

Status: v0.1.0 content implemented on branch `feat/agent-cli-core`; v0.2.0 (daemon adapter and API additions) implemented on branch `feat/core-v0.2`. Nothing is tagged: v0.2.0 is unreleased, tag pending. The PRD (draft v0.1) is [agent-cli-core-PRD.md](specs/archive/261003-agent-cli-core/agent-cli-core-PRD.md); it lives in the spec directory (specs/archive/261003-agent-cli-core/). Evidence markers in the PRD (confirmed vs not confirmed) must be preserved when summarizing it.

Dependency chain: `agent-okta-d` (`pkg/client`) <- `agent-cli-core` <- `snow-cli`, `outlook-cli`, `teams-cli`. `auth` defines its own `DaemonClient` interface and does not import `agent-okta-d`; the adapter `auth/oktad` (v0.2.0, requires `agent-okta-d` v0.1.0) is the only package that imports `pkg/client` (see docs/architectural-decision-record.md, ADR-2 and ADR-16).

## Layout

Doc routing: a shift in goal, direction or scope goes in [INTENT.md](INTENT.md) (why and where the library fits); requirements go in the PRD; contributor rules go here.

- `INTENT.md` - purpose, wider context, goals, non-goals, scope boundary
- `specs/archive/261003-agent-cli-core/agent-cli-core-PRD.md` - product requirements (the how, archived); the rest of the feature spec (`spec.md`, `plan.md`, `tasks.md`, `implementation-notes.md`) is in the same directory
- `README.md` - overview for consumers; `user-docs/README.md` - index of the consumer guides

Go layout (PRD section 5). Public API lives in top-level packages; anything not meant for consumers goes under `internal/`.

- `auth/`, `policy/`, `output/`, `audit/`, `httpx/`, `selftest/`, `docgen/`, `clock/` - the public packages
- `auth/authtest/` - public fake daemon for tests of consuming CLIs
- `auth/oktad/` - adapter over the credential daemon's Go client; the only importer of `agent-okta-d/pkg/client`
- `internal/` - non-API helpers (`redact`, `clock` (aliases of the public `clock`), `archtest`, `integration`)
- `examples/sampletool/` - vendor-neutral sample CLI that compiles and is tested; not API
- `docs/` - product summary, product details, technical details, architectural decision record
- `specs/` - feature specs; `specs/archive/` for completed ones
- `user-docs/` - see rule below

## user-docs/ rule

`user-docs/` holds only files that help a user adopt, configure and use the library (here, a developer building a CLI on it): how to add the dependency, getting started, per-package usage, configuration reference, examples, troubleshooting. It is NOT for design, requirements, spec or process material, and it must not link into `specs/`.

## Standards

- Clean Architecture: dependencies point inward; this library has no dependency on any vendor API (ServiceNow, Graph, Teams). Adapters sit at the edges behind interfaces.
- TDD: write a failing test first, make it pass, then refactor. Use fakes (fake daemon, fake clock, fake HTTP server) and golden tests for the envelope and exit codes.
- Public API is semver-governed. Treat every exported identifier outside `internal/` as API.
- Security: never commit credentials, tokens or secrets. Never print or log tokens. The library offers no function that returns or prints a token to a user.

## Verification

Before committing, all of these must pass:

```
gofmt -l .        # must print nothing
go vet ./...
golangci-lint run
go test ./...
```

`make fmt`, `make vet`, `make lint` and `make test` wrap these. There is no `make build`: this is a library.

## Git

Use clear commit messages. Do not force-push. Do not commit build output or `.env` files. Dependencies are declared in `go.mod` at released semver tags (`agent-okta-d` is at v0.1.0): no pseudo-versions and no `replace` directives on `main` (PRD section 14).

## Agent skill

How agents use this tool is documented in the root repository's skill document, `skills/agent-cli-core.md`, in https://github.com/stainedhead/agentic-teams (see `skills/README.md`). That is its only home; do not copy it here. A change to the envelope, exit codes, untrusted-content marking, output bounds, policy semantics, retry or idempotency behavior is not finished until that skill is updated (see SKILL-1..7 in the PRD).

# agent-cli-core - Feature Specification

Created: 2026-10-03 | Status: Draft | Source PRD: `specs/261003-agent-cli-core/agent-cli-core-PRD.md`

Evidence markers in the PRD (confirmed vs not confirmed) are preserved here as "(unconfirmed)" where the PRD uses the warning marker.

## 1. Executive Summary
`agent-cli-core` is a Go library (module `github.com/stainedhead/agent-cli-core`, no binary) holding behavior shared by the agent-facing CLIs `snow`, `outlook`, `teams`: daemon-backed token acquisition (`auth`), client-side policy (`policy`), response envelope / exit codes / untrusted-content marking / bounded output (`output`), JSONL audit (`audit`), retrying HTTP + redaction (`httpx`), a self-test runner (`selftest`) and harness skill-doc generation (`docgen`). It has no vendor clients.

## 2. Problem Statement
Three CLIs must present identical envelopes, exit codes, token handling, policy and audit behavior to an LLM harness. Duplicating this per repo causes drift and token-leak risk. A single library, released at semver tags, removes that.

## 3. Goals / Non-Goals
Goals (PRD G1-G6): one audited token path that never prints/logs tokens; one envelope and exit-code set; untrusted-content marking; bounded output; policy + audit layers; adoption via a semver `go.mod` line.
Non-Goals: vendor clients or command surfaces; a security control (policy is a guardrail); a binary/CLI/container/daemon; any token-output helper; a general CLI framework.

### Scope decisions for this spec (from the user, 2026-10-03)
- IN: all P0 requirements for `output`, `auth`, `policy`, `audit`, `httpx`, `selftest`, `docgen`; cheap P1 items (CORE-OUT-6 formats, CORE-POL-6 warn-if-writable, CORE-AUD-4, CORE-SELF-3/4, CORE-DOC-4 generic shape); CI (BLD-1..6); `Example*` tests in every package; API documented in `docs/technical-details.md`; targets darwin/arm64, linux/amd64, linux/arm64.
- DEFERRED: human-mode PKCE / keychain `TokenSource` (Q1: lives in `snow-cli`; the interface only must allow it); conformance kit (Q6) unless small (stretch task I6); `apidiff` wiring beyond docs (API-7 documented, not enforced); CD/release pipeline, SBOM/signing and downstream compatibility build (BLD-7/8, REL-*; no consumer code and no tag exists yet); M0a adapter over `agent-okta-d` `pkg/client`; CORE-POL-7 signature check (P2).
- `auth` defines its own `DaemonClient` / `TokenSource` interfaces and a fake; `agent-okta-d` is NOT imported. ADR and docs state that an adapter over `pkg/client` comes after a tagged `agent-okta-d` release and must be addable without breaking `auth`'s API.

## 4. Functional Requirements
Requirement IDs are the PRD's (`CORE-*`).

| FR | PRD IDs | Summary | Pri |
|---|---|---|---|
| FR-001 | CORE-OUT-1 | Success/error envelope types and writers, stable | P0 |
| FR-002 | CORE-OUT-2 | Exit codes 0..9 constants + error-category to exit-code mapping | P0 |
| FR-003 | CORE-OUT-3/4 | Untrusted-content marking (JSON `"untrusted": true`; text delimiters with author+timestamp) | P0 |
| FR-004 | CORE-OUT-5 | Bounds: default max-bytes 32768, `meta.truncated`/`next_offset`, UTF-8 and JSON safe | P0 |
| FR-005 | CORE-OUT-6 | json/table/text formats | P1 |
| FR-006 | CORE-OUT-7, CORE-HTTP-3 | Shared redactor applied to error messages/hints and traces | P0 |
| FR-007 | CORE-OUT-8, API-8 | Golden tests pin envelope + exit codes | P0 |
| FR-008 | CORE-AUTH-1/1a/1b/2 | `TokenSource`, `DaemonClient`, typed errors, fake, daemon-backed TokenSource parameterised by provider | P0 |
| FR-009 | CORE-AUTH-3/5 | Unreachable daemon / `reauth_required` -> exit 3, message names socket / human action; no fallback creds | P0 |
| FR-010 | CORE-AUTH-4, CORE-HTTP-6 | On 401 exactly one forced refresh + one retry; second 401 -> exit 3; total retries bounded jointly with httpx | P0 |
| FR-011 | CORE-AUTH-6 | 403 -> exit 4, vendor error code, never the request body | P0 |
| FR-012 | CORE-AUTH-7, CORE-HTTP-1/2/4/5 | Retry 429/503 honoring Retry-After, bounded, jitter, idempotent-only, injectable clock/sleep, then exit 8 | P0 |
| FR-013 | CORE-AUTH-8/9, SEC-1 | Redacting token type (`[redacted]` for String/%v/%+v); no token-returning user-facing helper | P0 |
| FR-014 | CORE-AUTH-10/11 | Adapter over `pkg/client` and human-mode sources not in this package; tool may supply its own TokenSource | P1 / deferred |
| FR-015 | CORE-POL-1/2/3/4/5/8/9 | YAML policy: verb/resource allow/deny, field allowlists, constraints, rate limits, write modes, caps, decision as data, strict parse, fail closed | P0 |
| FR-016 | CORE-POL-6 | Warn/refuse when policy file writable by current user | P1 |
| FR-017 | CORE-POL-7 | Detached signature check | P2 / deferred |
| FR-018 | CORE-AUD-1/2/3/5 | JSONL audit record, versioned schema, no secrets/bodies, path from config | P0 |
| FR-019 | CORE-AUD-4 | Audit write failure surfaced; block-on-write configurable | P1 |
| FR-020 | CORE-SELF-1/2/5 | Matrix runner with tool-supplied probes, no server knowledge | P0 |
| FR-021 | CORE-SELF-3/4 | Envelope output, non-zero exit on failure, read-only-rows mode | P1 |
| FR-022 | CORE-DOC-1/2/3 | Deterministic SKILL.md from command tree; always includes untrusted rule, envelope, exit codes | P0 |
| FR-023 | CORE-DOC-4, SKILL-3 | One generic Markdown shape; reference (not duplicate) the shared conventions skill | P1 |
| FR-024 | API-1..9 | Semver, `internal/`, pre-1.0 policy, deprecation; documented | P0 (docs) |
| FR-025 | PRD section 8 | Dependency-rule test: no vendor name in any import path or identifier | P0 |

## 5. Non-Functional Requirements
- Go, standard library plus one YAML library (strict parsing). `go` directive stays as in `go.mod` (`go 1.27`).
- Compiles for darwin/arm64, linux/amd64, linux/arm64; native Windows is not a target.
- No `init()` side effects, no global mutable state, no import-time network; everything constructed explicitly with injected fakes (clock, HTTP, daemon).
- Small dependency footprint. Exported identifiers have doc comments. Every package has runnable `Example*` tests.
- Security SEC-1..9. A fuzz/property test asserts no token value appears in logs, errors or traces.
- TDD; PR CI has no credentials or real-system access.

## 6. System Architecture
Clean architecture; dependencies point inward. Allowed intra-module imports (acyclic by construction, enforced by `internal/archtest`):
```
internal/redact, internal/clock   (leaf)
output   -> internal/redact
httpx    -> output, internal/redact, internal/clock
auth     -> output, internal/redact, internal/clock   (NOT httpx)
policy   -> stdlib + yaml only; returns decisions as data
audit    -> internal/redact, internal/clock           (NOT policy; decision carried as plain string)
selftest -> output
docgen   -> output
```
401 refresh-and-retry (FR-010) vs retry bounds: `httpx` defines a `TokenRefresher` interface (`Authorize(ctx, *http.Request)`, `Refresh(ctx)`) that an `auth` type satisfies structurally, so `auth` and `httpx` never import each other. The shared total-attempt budget lives in `httpx`.

## 7. Scope of Changes
Create: `output/`, `auth/` (+ `auth/authtest/`), `policy/`, `audit/`, `httpx/`, `selftest/`, `docgen/`, `internal/redact/`, `internal/clock/`, `internal/archtest/`, `examples/` (cross-package sample tool), updated `.github/workflows/ci.yml`, updates to `docs/`, `user-docs/<pkg>.md` guides. Dependencies: one YAML library only.

## 8. Breaking Changes
None (first release `0.1.0`). Pre-1.0: breaks bump minor.

## 9. Success and Acceptance Criteria
Quality gates for every workstream: `gofmt -l .` empty; `go vet ./...`; `golangci-lint run`; `go test -race ./...`; `go mod tidy` no diff; `GOOS/GOARCH` cross-build of `./...` for the three targets.
Acceptance: PRD milestones M0..M4 plus a cross-package example in `examples/`; `docs/technical-details.md` documents each package's API; ADR records D4 and deferrals; golden tests pin envelope, exit codes, audit schema.

## 9a. Numbered Acceptance Criteria (each testable; maps to FRs)
- AC-1 (FR-001/007): success and error envelopes match golden files in json, table, text; message text vendor-neutral.
- AC-2 (FR-002/007): constants 0..9 match the PRD table; every Category maps to exactly one code; unknown category maps to 1.
- AC-3 (FR-003): a declared free-text field is emitted with `"untrusted": true` (JSON) or delimiters including author and timestamp (text); undeclared fields are untouched.
- AC-4 (FR-004): output over 32768 bytes sets `meta.truncated` and `next_offset`; result is valid JSON and valid UTF-8 for inputs with multi-byte characters at the cut point.
- AC-5 (FR-006/013, SEC-1): property/fuzz test finds no token value in any error, log, trace, audit line or envelope; `String`/`%v`/`%+v`/`%#v` on `auth.Token` print `[redacted]`.
- AC-6 (FR-008/009): against `authtest.Fake`, valid/refresh/revoked/reauth_required/unreachable all behave as specified; unreachable names the socket and maps to exit 3; no fallback is attempted.
- AC-7 (FR-010): 401 then success -> exactly one refresh and one retry; 401 twice -> exit 3 with no third request; total attempts never exceed the shared budget.
- AC-8 (FR-011): 403 -> exit 4, vendor code reported, request body absent from error and trace.
- AC-9 (FR-012): 429/503 with Retry-After (delta-seconds and HTTP-date) retried within MaxRetries and MaxWait on a fake clock, then exit 8; non-idempotent request is not retried unless marked safe; no real sleeping in tests.
- AC-10 (FR-015/016): table-driven policy decisions; unknown YAML key, empty file, and invalid value all fail closed; write modes allow/dry_run_only/deny honored; decision carries rule id and reason.
- AC-11 (FR-015): rate limits per hour/per run enforced on a fake clock, safe under concurrent `Check` calls (`-race`).
- AC-12 (FR-018/019): audit golden JSONL; no secret or body fields; schema_version present; write failure is returned, and block-on-failure is configurable.
- AC-13 (FR-020/021): selftest reports per-row pass/fail with fake probes; failing matrix -> exit 1; read-only mode skips non-read-only rows.
- AC-14 (FR-022/023): generated SKILL.md is byte-identical across runs and map orderings, contains untrusted rule, envelope and exit codes; matches golden.
- AC-15 (FR-025): archtest fails on an import cycle, a forbidden edge (e.g. `auth`->`httpx`) or a vendor name.
- AC-16 (NFR): gofmt, vet, golangci-lint, `go test -race ./...`, tidy diff and three-target cross-build pass; every package has an `Example*` test; `docs/technical-details.md` covers every exported package.

## 9b. Edge Cases and Error Paths
- External/daemon failures: socket missing, permission denied, timeout, malformed daemon reply, context cancellation mid-request (all map to a Category; none leak a token or fall back to other credentials).
- Concurrency: concurrent `Token` calls trigger a single in-flight refresh (single-flight); audit writes are serialized and each record is one atomic line; policy rate-limit counters are mutex-protected.
- Empty/null inputs: nil/empty envelope data, nil command tree, empty policy file, zero-row selftest matrix, empty `Retry-After`, negative or huge `Retry-After` (clamped by MaxWait), non-UTF-8 input to truncation.
- Permission boundaries: policy file writable by the agent user (warn or refuse, POSIX only); audit file unwritable; policy decisions never claim the server will allow the action.
- Request bodies: replayability on retry (body must be re-readable via `GetBody`; otherwise treated as non-retryable).
- Partial failures: audit write fails after the command succeeded (surface, do not hide the result).

## 9c. Out of Scope (explicit)
Vendor clients, CLI/binary, daemon, human-mode PKCE/keychain, `pkg/client` adapter, conformance kit (unless I.6 is small), apidiff enforcement, CD/release/SBOM/signing, downstream compatibility build, Windows, sample tool policies, policy signature check (CORE-POL-7), and the root-repo `skills/agent-cli-core.md` (maintained in `agentic-teams`; update is a manual PR, SKILL-4).

## 10. Risks and Mitigation
| Risk | Mitigation |
|---|---|
| `pkg/client` surface unknown (unconfirmed) | Own interface + fake; adapter later, sole importer (CORE-AUTH-10) |
| Scope creep into vendor logic | FR-025 test; review check |
| Import cycles | Fixed import graph (section 6); `internal/redact` is a leaf |
| Parallel workstreams colliding | Strict file ownership in `plan.md`; foundation first; `go.mod` rules |
| Redaction leaks | Cross-package fuzz/property test (integration) |

## 11. Timeline and Milestones
WS0 Foundation = M0 + M1; WS-A..F in parallel = M2..M4; WS-I integration = acceptance + CI. M0a, M5 (release), M6, M7 deferred. Unscheduled.

## 12. Decisions on PRD open questions
Q1 human mode: deferred, belongs in `snow-cli`. Q3/Q4: engine ships a generic schema, no tool sample policies; fixtures vendor-neutral. Q5: gates M0a only. Q6: deferred (stretch I6). Q11: documented only. CORE-AUTH-1b fake placement: `auth/authtest`. Redactor placement (PRD says "6.5 redactor", i.e. httpx): `internal/redact`, so `output` need not import `httpx`.

## 12a. Open items with owners
| Item | Owner | Resolution path |
|---|---|---|
| Q5 `pkg/client` tag / M0a adapter | agent-okta-d owner (TBD) | After a tagged release; separate spec |
| Q6 conformance kit | Enterprise Architecture (TBD) | Decide after 0.1.0 consumers exist |
| Q11 apidiff blocking vs advisory | Enterprise Architecture (TBD) | Decide before enforcing in CI |
| SKILL-1 root skill update | agentic-teams maintainers | Manual PR per release |

## 12b. Document-level notes
- AGENTS.md said the PRD stays at the repo root; create-spec moves it into the spec directory. Links in README, INTENT, AGENTS and docs were updated to the new path.
- DEP-1 forbids `replace` on main and no module in `go.mod` is ours today, so the dependency-token setup (DEP-2/3) is not needed until M0a.

## 13. References
- Source PRD: `specs/261003-agent-cli-core/agent-cli-core-PRD.md`
- `INTENT.md`, `AGENTS.md`, `docs/*`

# agent-cli-core — Product Requirements Document

| | |
|---|---|
| **Status** | Draft v0.1 |
| **Date** | 2026-10-03 |
| **Owner** | Enterprise Architecture (owner TBD) |
| **Companion docs** | `snow-cli-PRD.md` (§5 is the origin of this document), `outlook-cli-PRD.md` (§7, AUTH-1..4), `teams-cli-PRD.md` (§5, §6), `agent-okta-d-PRD.md` (§9 `pkg/client`, §11 daemon socket API) |
| **Module path** | `github.com/stainedhead/agent-cli-core` (Go library; no binary) |

**Evidence legend.** ✅ = confirmed against vendor documentation during research (2026-10-03). ⚠️ = not confirmed in vendor docs this session (community source, unwritten upstream detail or engineering judgment). Validate every ⚠️ item before depending on it. This document is about our own library, so most statements are design decisions rather than vendor facts; ✅ is used sparingly and only where a vendor or sibling document was actually checked.

---

## 1. Summary

`agent-cli-core` is the shared Go library from which the agent-facing CLIs `snow`, `outlook` and `teams` are built. It holds the behavior that must be identical across those tools so each tool stays a thin layer of vendor-specific commands:

1. **Token from the daemon.** A `TokenSource` abstraction whose agent implementation wraps `pkg/client` from `agent-okta-d`, with one forced refresh and retry on `401`, and a clear failure when the daemon reports `reauth_required`.
2. **One envelope, stable exit codes, untrusted-content marking, bounded output.** So an LLM harness sees the same shapes from every tool.
3. **Client-side policy, audit log, HTTP retry/redaction, a self-test runner and skill-document generation.**

The library contains **no vendor clients**. ServiceNow, Microsoft Graph and Teams code stays in the tool repositories.

Nothing is implemented and nothing is released. This PRD was seeded from `snow-cli-PRD.md` §5 and the CLI auth and behavior requirements in the `outlook-cli` and `teams-cli` PRDs, and makes the shared behavior generic.

## 2. Decisions already made

| # | Decision | Source |
|---|---|---|
| D1 | The shared CLI core is **its own repository**, `agent-cli-core`, not a directory inside `snow-cli`. | User decision, 2026-10-03; supersedes the "open question" in `snow-cli-PRD.md` §5 and §15.3 |
| D2 | It is a **Go library**: no `main` package, no binary, no container image. | Same |
| D3 | `snow`, `outlook` and `teams` stay **separate binaries** (so harness allow-lists and permission prompts can key on command name), each in its own repository, each importing this module. | `snow-cli-PRD.md` §5 |
| D4 | Its `auth` package **wraps `pkg/client`** from `agent-okta-d` (module `github.com/stainedhead/agent-okta-d`). | `agent-okta-d-PRD.md` §9 |
| D5 | It contains **no vendor clients**: nothing that knows ServiceNow, Graph or Teams. | This PRD (§5.2) |

Where the sibling PRDs still describe the core's location as open, this decision supersedes them; updating those documents is tracked in §13.

## 3. Goals and non-goals

**Goals**

- G1. Every agent-facing CLI gets its token from the daemon through one audited code path, and **never prints or logs a token**.
- G2. All tools return the same envelope and exit codes, so a harness skill can describe them once.
- G3. Free text written by other people is marked untrusted the same way everywhere (a mitigation for prompt injection, not a guarantee).
- G4. Output is bounded and safe to feed to an LLM.
- G5. A client-side policy engine and an audit log are available as guardrail and traceability layers.
- G6. A consumer can adopt the library with a `go.mod` line at a released semver tag, and a breaking change is visible in the version number.

**Non-goals**

- No vendor clients, no vendor API knowledge, no command surfaces (they live in `snow-cli`, `outlook-cli`, `teams-cli`).
- Not a security control. The vendor's server-side permissions (ServiceNow roles and ACLs, Entra/Exchange scopes) are the boundary; the policy engine is a guardrail and usability layer.
- No binary, no CLI, no container image, no daemon. The daemon is `agent-okta-d`.
- No token output of any kind: no function that returns a token to a user-facing surface, and no `token`/`print-token` helper.
- Not a general-purpose CLI framework. It provides the shared behavior listed in §6 only.

## 4. Consumers and dependency direction

```
agent-okta-d  (pkg/client)
      ^
      |  imports
agent-cli-core  (auth, policy, output, audit, httpx, selftest, docgen)
      ^
      |  imports
snow-cli  ·  outlook-cli  ·  teams-cli   (binaries: snow, outlook, teams)
```

| Module | Role | Depends on |
|---|---|---|
| `github.com/stainedhead/agent-okta-d` | Credential daemon; `pkg/client` is its Go client for the unix-socket API | (none of ours) |
| `github.com/stainedhead/agent-cli-core` | This library | `agent-okta-d` (`pkg/client` only) |
| `snow-cli`, `outlook-cli`, `teams-cli` | Binaries `snow`, `outlook`, `teams` | `agent-cli-core`; vendor clients of their own |

Rules:

- Dependencies point one way. `agent-okta-d` never imports `agent-cli-core`; the core never imports a CLI repository.
- The CLIs reach `pkg/client` **through** the core (`agent-okta-d-PRD.md` §9 describes `pkg/client` as consumed by the core's `auth` package). A CLI should not need to import `pkg/client` directly for the agent-mode path.
- **Dependency not yet satisfiable.** No release of any repository exists. `agent-okta-d` must publish a tagged release containing `pkg/client` (its PRD §17.5 expects even a `0.1.0` containing only `pkg/client`) before this module can compile against it. Recorded as open item Q5 and milestone M0.

## 5. Architecture

### 5.1 Package layout (planned)

Public packages are top-level; anything not meant for consumers is under `internal/` (§7).

```
agent-cli-core/
├─ auth/        TokenSource interface; daemon-backed implementation wrapping agent-okta-d pkg/client
├─ policy/      YAML policy engine (generic: verbs, resources, fields, limits, write modes)
├─ output/      envelope, truncation, untrusted-content marking, exit codes
├─ audit/       JSONL audit log
├─ httpx/       retrying HTTP transport, Retry-After handling, redacted tracing
├─ selftest/    expected-allow/deny matrix runner (the matrix is supplied by the tool)
├─ docgen/      harness skill document (SKILL.md) generation from a command tree
└─ internal/    non-API helpers
```

### 5.2 Dependency rule

The core knows nothing of ServiceNow, Microsoft Graph or Teams. Vendor behavior enters only through interfaces the core defines and the tool implements (for example, a `TokenSource` for a non-daemon source, a policy "resource" and "verb" vocabulary, a self-test matrix, a command-tree description for `docgen`). The core's only external service dependency is the daemon, through `pkg/client`. A review check: no import path in this module contains a vendor name, and no identifier encodes a vendor concept.

### 5.3 Typical use in a tool

```
tool command -> policy.Check -> auth.TokenSource.Token -> httpx.Do (vendor request)
             -> output.Envelope / output.Exit -> audit.Record
```

The tool owns the vendor request and response parsing; the core owns everything around it.

## 6. Package requirements

Priorities: P0 = needed for the first usable release, P1 = soon after, P2 = later. IDs use the prefix `CORE-`. Behavior is generic; vendor specifics stay in the tool PRDs.

### 6.1 `auth`

Origin: `snow-cli-PRD.md` §5 and AUTH-A1..A3; `outlook-cli-PRD.md` AUTH-1..4; `teams-cli-PRD.md` §5 (Token path) and "AUTH-1..4 are the same as in outlook-cli".

| ID | Requirement | Pri |
|---|---|---|
| CORE-AUTH-1 | Define a `TokenSource` interface so a tool can obtain a bearer token without knowing where it comes from. | P0 |
| CORE-AUTH-2 | Provide the daemon-backed `TokenSource`, wrapping `pkg/client`, parameterised by provider name (the tool supplies it, for example `servicenow` or `msgraph`). The core hard-codes no provider name. | P0 |
| CORE-AUTH-3 | No fallback credentials: if the daemon socket is unreachable the operation fails and nothing else is tried. The CLI exits with code `3` and a clear, actionable message that the daemon could not be reached; the message names the socket that was tried and says the `agent-okta-d` service may not be running. Decided: exit code `3` is approved (the sibling PRDs say "refuse to run" without naming a code). | P0 |
| CORE-AUTH-4 | On HTTP `401` from the vendor, force exactly one daemon refresh (the daemon exposes `POST /v1/credentials/{provider}/refresh`, `agent-okta-d-PRD.md` §11) and retry the request once. A second `401` exits `3`. | P0 |
| CORE-AUTH-5 | If the daemon reports `reauth_required`, exit `3` with a message that a human action is needed. The core supplies the generic message; the tool supplies the exact remediation command (for example, outlook/teams: a human runs `agent-okta-d enroll msgraph`). | P0 |
| CORE-AUTH-6 | On HTTP `403`, exit `4`, report the vendor error code, and **do not include the request body** in the error or the log. | P0 |
| CORE-AUTH-7 | Handled with `httpx` (6.5): `429` and `503` with `Retry-After` are retried a bounded number of times, then exit `8`. | P0 |
| CORE-AUTH-8 | Never print, log, trace, put in an error string, or expose in `ps` or child-process environment any token. Token values use a redacting type whose `String()` returns `[redacted]`. | P0 |
| CORE-AUTH-9 | No exported function returns or prints a token to a user-facing surface, and the core provides no `token`/`print-token` helper. | P0 |
| CORE-AUTH-10 | The exact surface of `pkg/client` (types, errors, how `reauth_required` is signalled) is not specified in `agent-okta-d-PRD.md`; `auth` adapts to it behind its own interface so a change in `pkg/client` is absorbed in one place. ⚠️ | P0 |
| CORE-AUTH-11 | Human-mode token sources (Okta PKCE login, OS keychain) are **not** in this package unless Q1 decides otherwise. The interface must allow a tool to supply its own `TokenSource` implementation. | P1 |

### 6.2 `policy`

Origin: `snow-cli-PRD.md` §5 and §9 (the policy YAML shown there is ServiceNow-shaped; the generic engine is extracted from it). The sibling PRDs (`outlook-cli-PRD.md` §9, `teams-cli-PRD.md` §7) each define their own policy section; a common schema is assumed possible but not verified ⚠️ (Q3).

| ID | Requirement | Pri |
|---|---|---|
| CORE-POL-1 | YAML policy engine with allow and deny rules per **verb** and **resource**, where verb and resource are opaque strings supplied by the tool. | P0 |
| CORE-POL-2 | Field allowlists, value constraints, and per-hour/per-run rate limits. | P0 |
| CORE-POL-3 | Write modes per rule: `allow` \| `dry_run_only` \| `deny`. | P0 |
| CORE-POL-4 | Maximum-results and maximum-bytes caps, feeding the output bounds (6.3). | P0 |
| CORE-POL-5 | A decision is returned as data (decision, rule id, reason) so the tool can map a denial to exit `6` and record it in the audit log as `policy_decision`. | P0 |
| CORE-POL-6 | The policy file location is supplied by the tool or configuration. The deployment convention is a root-owned directory the agent user can read but not write (`snow-cli-PRD.md` §5); the library does not enforce file permissions but **can warn or refuse** when the file is writable by the current user ⚠️ (proposed). | P1 |
| CORE-POL-7 | Optional detached-signature check of the policy file. | P2 |
| CORE-POL-8 | Strict parsing: unknown keys are errors; an invalid policy fails closed (the tool does not run). | P0 |
| CORE-POL-9 | The engine is a guardrail, not the control. Documentation and error text must not imply that passing the policy means the server will allow the action. | P0 |

### 6.3 `output`

Origin: `snow-cli-PRD.md` §5 (envelope, exit codes, untrusted content, output bounds).

**Envelope (success)**

```json
{ "ok": true,
  "data": { "...": "..." },
  "meta": { "truncated": false, "next_offset": null, "count": 12, "request_id": "..." } }
```

**Envelope (error)**

```json
{ "ok": false,
  "error": { "code": "policy_denied", "message": "closing incidents is not allowed for agents", "hint": "ask a human to resolve" } }
```

(The example message is taken from the `snow` PRD; the core's own tests must use vendor-neutral text.)

**Exit codes**

| Code | Meaning |
|---|---|
| `0` | ok |
| `1` | general error |
| `2` | usage |
| `3` | auth (including `reauth_required`, second `401`, daemon unreachable, with a clear message) |
| `4` | forbidden by server (`403` / ACL) |
| `5` | not found |
| `6` | denied by client policy |
| `7` | conflict / precondition |
| `8` | rate-limited / transient (`429`, `503` after bounded retries) |
| `9` | validation (for example a mandatory field is missing) |

| ID | Requirement | Pri |
|---|---|---|
| CORE-OUT-1 | Provide the envelope types and writers exactly as above, stable across tools. | P0 |
| CORE-OUT-2 | Provide the exit-code constants `0`..`9` with the meanings above, and a mapping from an error category to an exit code, so tools do not re-implement it. | P0 |
| CORE-OUT-3 | **Untrusted-content marking.** Free text written by other people (ticket descriptions, comments, work notes, email bodies, chat messages) can contain instructions aimed at the model. The core marks such fields `"untrusted": true` in JSON and, in text output, wraps them in explicit delimiters with the author and timestamp. The tool decides which fields are free text; the core does the marking. | P0 |
| CORE-OUT-4 | The generated skill document (6.7) tells the agent that marked content is data, never instructions. This is a mitigation, not a guarantee; server-side limits remain the real control. | P0 |
| CORE-OUT-5 | **Output bounds.** Default `--max-bytes 32768`. A truncated output sets `meta.truncated` and `next_offset`. Truncation never cuts inside a multi-byte character or produces invalid JSON. | P0 |
| CORE-OUT-6 | Formats `json` (default for agents), `table` and `text`; the default is chosen by the tool/mode. | P1 |
| CORE-OUT-7 | Error messages and hints are passed through the redactor (6.5) so they cannot contain tokens or request bodies. | P0 |
| CORE-OUT-8 | The envelope and exit-code contract is covered by golden tests (§8) and is part of the public API: changing it is a breaking change (§7). | P0 |

### 6.4 `audit`

Origin: `snow-cli-PRD.md` §5.

| ID | Requirement | Pri |
|---|---|---|
| CORE-AUD-1 | JSONL audit log, one record per command, with fields `ts, tool, agent_id, run_id, verb, resource, outcome, http_status, duration, policy_decision`. | P0 |
| CORE-AUD-2 | No secrets and, by default, no free-text bodies. | P0 |
| CORE-AUD-3 | Log path comes from configuration or policy (the `snow` PRD shows a path of the form `/var/log/agent-cli/<tool>.audit.jsonl` in its sample policy; that is a convention, not a library default). | P0 |
| CORE-AUD-4 | A failure to write the audit log is surfaced, and whether it blocks the command is configurable (proposed default: block write operations) ⚠️. | P1 |
| CORE-AUD-5 | The record schema is versioned and changes follow §7. The CI/CD section does not change this schema. | P0 |

### 6.5 `httpx`

Origin: `snow-cli-PRD.md` §5; AUTH-4 in `outlook-cli-PRD.md`.

| ID | Requirement | Pri |
|---|---|---|
| CORE-HTTP-1 | HTTP transport with bounded retries and jitter for transient failures. | P0 |
| CORE-HTTP-2 | `429`/`503` with `Retry-After` are honored; after bounded retries the caller receives an error that maps to exit `8`. The retry bound and the ceiling on any wait are configurable. | P0 |
| CORE-HTTP-3 | Request tracing with redaction of `Authorization` and other secret-bearing headers, and of request/response bodies by default. Tracing is off unless enabled by the tool (for example the `snow` PRD makes `--trace` human mode only). | P0 |
| CORE-HTTP-4 | Retries only idempotent requests unless the caller marks a request safe to retry (for example it carries an idempotency key). A non-idempotent request is never retried silently. | P0 |
| CORE-HTTP-5 | Clock and sleep are injectable so tests do not wait. | P0 |
| CORE-HTTP-6 | The one `401` refresh-and-retry of CORE-AUTH-4 is coordinated with the retry logic so a request is never retried more than the stated bounds in total. | P0 |

### 6.6 `selftest`

Origin: `snow-cli-PRD.md` §5 and §10 (`snow selftest`).

| ID | Requirement | Pri |
|---|---|---|
| CORE-SELF-1 | A runner that executes a matrix of expected allows and denies supplied by the tool and reports pass/fail per row. | P0 |
| CORE-SELF-2 | The runner knows nothing of the server being tested; the tool supplies the probe functions. | P0 |
| CORE-SELF-3 | Output uses the standard envelope; a failing matrix exits non-zero with a stable code (proposed: `1`) ⚠️. | P1 |
| CORE-SELF-4 | Self-test is destructive-capable only if the tool says so; the runner supports a "read-only rows only" mode. | P1 |
| CORE-SELF-5 | Self-tests run against a live server and therefore only on demand, never in PR CI (§14). | P0 |

### 6.7 `docgen`

Origin: `snow-cli-PRD.md` §5 and §15.5 (`SKILL.md` regenerated from the command tree).

| ID | Requirement | Pri |
|---|---|---|
| CORE-DOC-1 | Generate the harness skill document (`SKILL.md`) from a command-tree description supplied by the tool: usage, examples, forbidden actions. | P0 |
| CORE-DOC-2 | Always include the untrusted-content rule (CORE-OUT-4), the envelope and the exit codes. | P0 |
| CORE-DOC-3 | Output is deterministic (no timestamps, stable ordering) so a tool's CI can detect drift by regenerating and diffing. | P0 |
| CORE-DOC-4 | The format required by each harness (Hermes, or the CLI harness we provide) is not yet defined in this set ⚠️; `docgen` starts with one generic Markdown shape and adapts after harness integration is confirmed. | P1 |

## 7. Public API stability and versioning

| ID | Requirement |
|---|---|
| API-1 | The module follows **semantic versioning**. The git tag `vX.Y.Z` is the module version (see §14). |
| API-2 | Everything exported from a package outside `internal/` is public API. Non-API code lives under `internal/`, which the Go toolchain prevents other modules from importing. |
| API-3 | **Pre-1.0 policy.** While the version is `0.y.z`, a breaking change bumps the minor version (`0.y.z` -> `0.(y+1).0`) and is called out in the release notes. `1.0.0` is cut by an explicit decision, not automatically. |
| API-4 | **After 1.0.** Breaking changes require a new major version. |
| API-5 | **Deprecation.** An API is first marked `// Deprecated:` with its replacement, kept for at least one minor release (proposed) ⚠️, and removed only in a version that is allowed to break (API-3/API-4). |
| API-6 | **v2 and later.** Go requires a major version of 2 or higher to use a `/v2` (and so on) suffix on the module path and import paths. A v2 therefore needs a deliberate migration of every consumer's imports; it is avoided by keeping the public surface small and by using `internal/`. |
| API-7 | **Compatibility checks.** CI runs an API-compatibility check against the previous release (for example `apidiff` or `gorelease` from `golang.org/x/exp`/`golang.org/x/tools`) and fails a PR whose API change is larger than its release label allows ⚠️ (tool choice not yet verified). |
| API-8 | The envelope, exit codes and audit schema (CORE-OUT-1/2/8, CORE-AUD-5) are part of the API contract even though they are data formats; they are pinned by golden tests. |
| API-9 | A bad release is superseded, never deleted or re-tagged (§14, REL-14). |

## 8. Testing

Tests follow TDD (failing test first). PR CI uses no credentials and no network access to real systems.

| Area | Approach |
|---|---|
| `auth` | A **fake daemon** (a unix-socket HTTP server speaking the `agent-okta-d` local API: `GET /v1/credentials/{provider}`, `POST /v1/credentials/{provider}/refresh`) covering: valid token, expired token needing refresh, `reauth_required`, socket missing, `401` then success after refresh, `401` twice. |
| `httpx` | Fake HTTP server and fake clock: `429`/`503` with `Retry-After`, jitter bounds, retry limits, non-idempotent requests not retried, redaction in traces. |
| `output` | **Golden tests** for the success and error envelopes in each format, for truncation (`meta.truncated`, `next_offset`), for untrusted-content marking in JSON and text, and for every exit code `0`..`9` and its category mapping. |
| `policy` | Table-driven decisions; strict-parse failures; fail-closed on invalid policy; rate-limit windows with a fake clock. |
| `audit` | Golden JSONL records; assertion that no secret or body appears; failure-to-write behavior. |
| `selftest`, `docgen` | Fake probes; deterministic output compared to golden files. |
| Security tests | Fuzz/property test that no token value appears in any log line, error string or trace; `String()`/`%v`/`%+v` on token types print `[redacted]`. |
| Dependency rule | A test or lint rule that fails if a vendor name appears in an import path (§5.2). |

**Open idea: conformance test kit (Q6).** A package (for example `conformance/` or an `agent-cli-core/conformancetest` helper) that each consuming CLI runs against its own binary or command tree to check the shared contract: envelope shape, exit codes for canned error conditions, untrusted-content marking, no-token-output, output bounds. It would keep the three tools from drifting apart. Not decided; it would be public API and so subject to §7.

## 9. Security

| ID | Requirement |
|---|---|
| SEC-1 | Tokens are never printed, logged or traced, in any package, at any log level, and do not appear in `ps` or in the environment of child processes. |
| SEC-2 | There is no fallback credential path. If the daemon is not reachable, the operation fails. |
| SEC-3 | `403` handling never echoes the request body (CORE-AUTH-6). |
| SEC-4 | Untrusted content is marked on every free-text field the tool declares (CORE-OUT-3); this reduces but does not eliminate prompt-injection risk. |
| SEC-5 | The policy engine fails closed on invalid input and is documented as a guardrail, not the control (CORE-POL-8/9). Server-side permissions remain the boundary. |
| SEC-6 | The audit log contains no secrets and, by default, no free-text bodies (CORE-AUD-2). |
| SEC-7 | Supply chain: dependencies at released semver tags (DEP-1), `govulncheck` and a pinned linter in CI, pinned actions, signed and attested releases (§14). |
| SEC-8 | The library never reads the daemon's key material or any secret store; it speaks only to the daemon's unix socket through `pkg/client`. Peer-credential authentication is the daemon's job (`agent-okta-d-PRD.md` §11). |
| SEC-9 | No credentials, real tenant identifiers or real instance names in tests, fixtures or examples. |

## 10. Non-functional requirements

- **Stack:** Go, standard library plus a small set of dependencies (the YAML parser and `pkg/client` are the expected ones). The `go` directive follows the sibling repositories (`go 1.27`).
- **Platforms:** must compile for `darwin/arm64`, `linux/amd64`, `linux/arm64`. Native Windows is not a target: Windows users run the Linux build under WSL2 (decided).
- **Overhead:** the library adds little latency compared with the vendor round trip; `snow-cli-PRD.md` §11 sets "local overhead < 50 ms" for the tool, which the core must not consume on its own ⚠️ (not measured).
- **No `init()` side effects, no global mutable state, no network at import time;** everything is constructed explicitly so tests can inject fakes.
- **Dependency footprint** kept small and reviewed, because every consumer inherits it.
- **Documentation:** exported identifiers have doc comments; `user-docs/` gets a guide per package when there is a release.

## 11. Delivery plan and acceptance (proposed)

All milestones are proposed and unscheduled.

| Milestone | Scope | Acceptance |
|---|---|---|
| **M0 Prerequisites** | `agent-okta-d` publishes a tagged release containing `pkg/client`; confirm its surface; settle Q1-Q5 enough to start; CI (§14.1) in place | `go get github.com/stainedhead/agent-okta-d@<tag>` resolves; CI green on an empty module |
| **M1 Output contract** | `output` (envelope, exit codes, untrusted marking, bounds); golden tests | Golden tests pass; contract documented in `user-docs/` |
| **M2 Auth + httpx** | `auth` (daemon-backed), `httpx`, fake daemon and fake clock tests | `401`-refresh-retry, `reauth_required`, `403`, `429`/`503` behaviors proven against fakes; no-token-in-logs test passes |
| **M3 Policy + audit** | `policy`, `audit` | Policy decisions and audit records pass table and golden tests; fail-closed proven |
| **M4 selftest + docgen** | `selftest`, `docgen` | A sample tool in the tests generates a deterministic `SKILL.md` and runs a fake matrix |
| **M5 First release `0.1.0`** | CD (§14.2), first tag, downstream compatibility build in CI | Release published with checksum, SBOM and attestation; `snow-cli`, `outlook-cli`, `teams-cli` build against it |
| **M6 Conformance kit (if accepted)** | Q6 decision implemented | Each CLI runs the kit in its CI |
| **M7 Hardening** | API-compat check enforced, fuzzing, review | Security review sign-off |

The release pipeline (§14.2) is in place before the first tag. CI (§14.1) is in place before the first milestone that merges Go code.

## 12. Concerns and recommendations

1. **Upstream is not released.** The module cannot compile until `agent-okta-d` tags a release containing `pkg/client`. *Recommendation:* make that tag the first action of M0 and record it in both repositories.
2. **Unknown `pkg/client` surface.** The daemon PRD names the package and the HTTP endpoints but not the Go API ⚠️. *Recommendation:* review `pkg/client` when it exists and keep `auth` as the only importer inside this module (CORE-AUTH-10).
3. **Scope creep into a framework.** A "core" library tends to absorb vendor logic. *Recommendation:* enforce the dependency rule (§5.2) in review and by test, and keep vendor behavior in the tool repositories.
4. **One library, three consumers: coupling.** A breaking change forces three upgrades. *Recommendation:* small public surface, `internal/` by default, pre-1.0 minor bumps for breaks, the downstream compatibility build (§14.1) and possibly a conformance kit.
5. **Guardrail mistaken for control.** *Recommendation:* the policy documentation and generated skill document state that server-side permissions are the boundary (CORE-POL-9).
6. **Prompt-injection marking is a mitigation.** *Recommendation:* never describe it as sufficient; keep server-side limits.
7. **Windows statements upstream (resolved).** `snow-cli-PRD.md` once said "Windows for human mode" while its CI/CD section said no native Windows build. Decided: native Windows is not required and Windows users run the Linux build under WSL2; the `snow` PRD was updated to match.
8. **Sibling PRDs still call the core's location open.** *Recommendation:* update `snow-cli-PRD.md` §5/§15.3, `outlook-cli-PRD.md` and `teams-cli-PRD.md` to point at this repository (tracked as Q8).

## 13. Open questions

1. **Where do human-mode PKCE and the keychain `TokenSource` live?** Only `snow` needs them (`snow-cli-PRD.md` AUTH-H1..H6). Options: in `snow-cli`; in this library as an optional package (adds OS-keychain and OAuth dependencies that `outlook` and `teams` would inherit); or in a second small module. Leaning (unverified): keep it in `snow-cli` so this library stays small.
2. **Resolved: native Windows is not required.** The library does not need to compile for `windows/amd64`; Windows users run the Linux build under WSL2. CI compiles darwin/arm64, linux/amd64 and linux/arm64.
3. **Who owns the policy schema?** A common schema in this library, or per-tool schemas that the engine merely evaluates? `snow` (§9), `outlook` (§9) and `teams` (§7) each define their own policy shape today; a shared vocabulary is assumed possible ⚠️. Ownership also decides who approves schema changes.
4. **Library or tool: which package ships sample policy files?** The `snow` PRD ships sample `agent` and `human` policies with the binary (§15.5). This library should presumably ship none.
5. **`pkg/client` tag.** What is the first released version of `agent-okta-d` containing `pkg/client`, and when? Blocks M0. Related: should `pkg/client` instead be its own tiny module so the core does not pull in the daemon's dependencies ⚠️ (not raised in the daemon PRD; the daemon PRD places it in the same module).
6. **Conformance test kit.** Build one that each CLI runs? Where does it live (this module, or a sub-module so it does not burden consumers)? Who maintains the golden data?
7. **Release-notes policy for pre-1.0 breaks:** is a minor-bump-per-break (API-3) acceptable to the three tool teams?
8. **Updating sibling PRDs and docs** (`snow-cli`, `outlook-cli`, `teams-cli`, `agent-okta-d` AGENTS/README/PRD) to record that the core is its own repository.
9. **Owner.** The owner is TBD, as in the sibling PRDs.
10. **Shared pipeline.** The tool PRDs ask whether common workflow steps should live in one reusable workflow (`snow-cli-PRD.md` §15.7 item 6). With the core as its own repository, this is a natural home; not decided.
11. **API-compat tooling** (API-7): which tool, and does it block or advise?

## 14. CI/CD and release requirements

Applies to this repository. It is a **library**: there are no binaries and no container image, and nothing is deployed anywhere. The shape mirrors the pipeline used by the four Go tool repositories (`agent-okta-d`, `snow-cli`, `outlook-cli`, `teams-cli`), adapted for a library. Pipelines are GitHub Actions workflows under `.github/workflows/`. The scaffolded `ci.yml` is a starting point and must be brought in line with this section. Items marked ⚠️ are not confirmed and need a spike.

**Terminology.** *CI* verifies a change. *CD* produces and publishes a **release**. For a library, **publishing the release (the git tag plus the GitHub Release) is the whole of "deploy"**. Consumers pick a release up by bumping the version in their own `go.mod`.

### 14.1 Continuous integration

| ID | Requirement |
|---|---|
| BLD-1 | CI runs on **every pull request targeting `main`** and **on demand** (`workflow_dispatch`, optionally against a chosen ref). CI also runs as the first stage of every release (REL-9), so nothing is released untested. |
| BLD-2 | Checks: `gofmt -l .` is empty; `go mod tidy` leaves no diff; `go vet ./...`; `golangci-lint` at a pinned version; `go test -race ./...`; `govulncheck ./...`. |
| BLD-3 | **Compile check** on each PR for `darwin/arm64`, `linux/amd64` and `linux/arm64` (for example `GOOS=... GOARCH=... go build ./...` and `go vet`). Native Windows is not a target (Windows users run the Linux build under WSL2), so there is no `windows/amd64` check. |
| BLD-4 | PR CI needs **no credentials and no network access to real systems**: tests use fakes, a fake daemon and fake clocks. Module downloads are the only network use. |
| BLD-5 | The CI workflow is a **required status check** on `main` once branch protection is enabled. Branch protection is not configured yet; enabling it is a separate step. |
| BLD-6 | Workflows use least privilege, pin the Go version from `go.mod`, and pin third-party actions to a version or commit SHA. |
| BLD-7 | **Downstream compatibility build**, on PRs and on demand: check out `snow-cli`, `outlook-cli` and `teams-cli`, point each at the PR's version of this module with a **CI-only** `go mod edit -replace github.com/stainedhead/agent-cli-core=<path to PR checkout>` (never committed, and never pushed to those repositories), then run `go build ./...` and `go test ./...` in each. A failure means the PR breaks a consumer and the release label (or the change) must be reconsidered. Not implemented yet; the consumers have no code today. |
| BLD-8 | The downstream build also uses no credentials or real-system access (BLD-4) and the three consumer checkouts are read-only. |

### 14.2 Release, versioning and continuous delivery

| ID | Requirement |
|---|---|
| REL-1 | **Artifacts:** the release is a git tag `vX.Y.Z` on `main` and a GitHub Release. Attached: a source archive with a `SHA256` checksum, an SBOM (SPDX or CycloneDX), a build-provenance attestation, and a signature. **No binaries and no container image.** Keyless `cosign` signing from the workflow's GitHub OIDC identity ⚠️ is the proposed method (not verified for this repository). |
| REL-2 | The install documentation in `user-docs/` states how to verify the checksum, signature and attestation. |
| REL-5 | Releases follow **semantic versioning**. The git tag `vX.Y.Z` on `main` **is** the module version (Go resolves module versions from tags). Tags are immutable: a version is never re-tagged or re-published. |
| REL-6 | Releases start at `0.1.0` and stay `0.y.z` while this PRD is a draft. `1.0.0` is cut by an explicit decision, never automatically. |
| REL-7 | The bump is taken from a **PR label** `release:major`, `release:minor` or `release:patch`. An unlabeled PR that changes shipped code defaults to `patch`. A PR that touches only `docs/`, `user-docs/`, `specs/`, `*.md` or `INTENT.md` does **not** cause a release. Bumps must agree with §7 (a breaking change is `minor` while `0.y.z`, `major` after 1.0). |
| REL-8 | CD runs **on merge of a pull request to `main`** and **on demand** (`workflow_dispatch` with a `bump` of `major`, `minor` or `patch`, an optional explicit `version`, and a `dry_run` option that verifies but publishes nothing). |
| REL-9 | Stages, in order: CI gate (all of 14.1), compute version, verify (`go build ./...`, `go test -race ./...`, API-compat check per API-7 against the previous tag, release-notes generation), create the tag, publish the GitHub Release with notes generated from merged PR titles, attach the source archive and `SHA256` checksum, the SBOM, the provenance attestation and the signature. |
| REL-11 | **All-or-nothing:** if any stage fails, nothing is published. A failed run is safe to re-run, and a version is never published twice. |
| REL-12 | CD **does not roll out** anything. Consumers pick up a release by bumping the dependency in their `go.mod` (DEP-1); their own pipelines then build and release their binaries. |
| REL-13 | The release job gets only what it needs (`contents: write`, `id-token: write`, attestations) from a protected `release` environment. On-demand runs require write access to the repository, and a `major` bump on demand should require reviewer approval on the environment. No long-lived credentials are stored in the repository. |
| REL-14 | A bad release is not deleted. It is **superseded** by a newer patch release and marked as withdrawn in its release notes; its tag stays in place. Where a version must not be used, a `retract` directive in `go.mod` of a later version is the Go mechanism ⚠️ (not yet decided whether to use it). |

(Requirement IDs follow the numbering of the sibling PRDs; REL-3, REL-4 and REL-10, which concern binaries and artifact smoke tests, do not apply to a library and are omitted.)

### 14.3 Dependency requirements

| ID | Requirement |
|---|---|
| DEP-1 | Consumers (and this repository, for `agent-okta-d`) declare dependencies in `go.mod` **at a released semver tag**. No pseudo-versions and no `replace` directives on `main`. The CI-only `replace` of BLD-7 exists only inside a CI job and is never committed. |
| DEP-2 | Every workflow job that builds or tests resolves dependencies using the job's **dynamic `GITHUB_TOKEN`** (no personal access token, no stored secret), with `permissions: contents: read` and `packages: read`. |
| DEP-3 | Setup step before `go mod download`: set `GOPRIVATE=github.com/stainedhead/*` and configure git `url."https://x-access-token:${GITHUB_TOKEN}@github.com/".insteadOf "https://github.com/"` from the job token. The token is never echoed, and never written to caches or artifacts. |
| DEP-4 | The repositories are **public today**, so the token is not strictly needed. The step is standard so behavior is identical if visibility changes. |
| DEP-5 | ⚠️ `GITHUB_TOKEN` is scoped to the repository that runs the workflow, so it **cannot read a different private repository's contents**. If `agent-cli-core` or `agent-okta-d` ever become private, publish the dependency through GitHub Packages (for example as an OCI artifact) and grant the consumer repositories read access on the package (Manage Actions access) so that `packages: read` applies. Decide this **before** changing a repository's visibility. Also note that GitHub Packages has no Go module registry ⚠️ (unconfirmed; verify), so a Go-module-native private distribution is not assumed to exist. |

### 14.4 Repository-specific requirements

- **Release contents:** source archive, checksum, SBOM, attestation, signature, and release notes. Nothing else.
- **Not released from this repo:** any binary, any container image, any vendor client, any sample policy for a specific tool.
- **No secrets** in the pipeline other than the job token: no Okta, vendor or signing key material is needed (signing is keyless, ⚠️).
- **Upstream dependency:** CI and CD cannot pass until `agent-okta-d` has a tagged release containing `pkg/client` and this module requires it (M0).

### 14.5 Milestone placement

BLD-1 to BLD-6 are in place before the first milestone that merges Go code (M1). BLD-7 and BLD-8 are in place before `0.1.0` (M5), and once at least one consumer repository has code that imports the library. The release pipeline (REL-1 to REL-14) is in place before the first tag.

### 14.6 Open items (CI/CD)

1. **Branch protection.** Not configured. Enable and make CI a required check (BLD-5) once the first workflow has run on a PR.
2. **windows/amd64** compile check: not needed; native Windows is not a target.
3. **Signing.** Confirm keyless `cosign` is acceptable for a source-only library release ⚠️ (REL-1).
4. **API-compat tool** (API-7, Q11) and whether it blocks.
5. **Version bump rule.** PR labels are assumed (REL-7); conventional commits are the alternative.
6. **`retract` policy** (REL-14).
7. **Private-repository path** (DEP-5): decide before any visibility change.
8. **Shared pipeline** (Q10): whether common steps become a reusable workflow hosted here.

---

## 15. Agent skill document

Agents that adopt this tool need to know how to use it. That knowledge is a **skill document**, published in one place for the whole set: the root repository's `skills/` folder (https://github.com/stainedhead/agentic-teams/tree/main/skills), one file per repository, named `<repo-name>.md`. The root repository is where agents find and adopt it.

| ID | Requirement |
|---|---|
| SKILL-1 | **One home.** The skill for this repository is `skills/agent-cli-core.md` in the root `agentic-teams` repository. This repository does not keep a second copy. The root `README.md` and `skills/README.md` tell agents to adopt it from there. |
| SKILL-2 | **Minimum content.** An availability banner (how to check the tool is installed with `command -v`, and the version the skill applies to); when to use the tool and when not to; the shared conventions every CLI built on this library follows (output envelope, exit codes, untrusted-content marking, output bounds, policy write modes, retry and idempotency rules); the output shape and exit codes (shared conventions are in `skills/agent-cli-core.md`); the rules and forbidden actions; how untrusted content and instructions found in it are treated; the rule that the agent never asks for, reads, prints or stores credentials; a table mapping each error to the action the agent should take; and links to this repository. |
| SKILL-3 | **Source of truth.** Hand-written and maintained in the root repository as `skills/agent-cli-core.md`, because this library is not a command. The three CLI skills link to it instead of repeating it. `docgen` output for a CLI must reference it rather than duplicate it (CORE-DOC-1). |
| SKILL-4 | **Currency.** A change to the envelope, exit codes, untrusted-content marking, output bounds, policy semantics, retry or idempotency behavior is not complete until the root skill is updated and names the version it applies to. Release notes link to the skill revision for that version. |
| SKILL-5 | **Honest availability.** Until a release exists the skill carries a banner saying the tool is planned and not installed, and tells agents to report that instead of building or reimplementing it. The banner is removed only after a release is published and the skill's examples have been run against it. |
| SKILL-6 | **Format.** Plain Markdown with `name` and `description` frontmatter. The skill format each harness (Hermes, or the CLI harness we provide) expects is not defined yet ⚠️ (`agent-cli-core-PRD.md`, CORE-DOC-4); the format may be adapted without changing the content. |
| SKILL-7 | **Milestone placement.** A reviewed skeleton skill exists by the first milestone that produces a runnable binary, and a complete skill is an acceptance item of that milestone and of the hardening milestone, not only the latter. |

### 15.1 Open items (agent skill)

1. **Updating the root from this repository's release.** Publishing a change to another repository's `skills/` folder needs write access to that repository. The workflow's dynamic `GITHUB_TOKEN` is scoped to the repository running the workflow ⚠️, so release CD cannot do it with the token this PRD otherwise requires. Options: a manual pull request opened from the release artifact (assumed until decided), a GitHub App installation token, or a fine-grained personal access token. Decide before automating.
2. **Skill for the library and for the daemon.** Whether `docgen` should embed the shared conventions in each generated CLI skill instead of linking to `skills/agent-cli-core.md` is not decided; linking keeps one home.

## Appendix — Sources consulted

- `snow-cli-PRD.md` §5 (shared core, envelope, exit codes, untrusted content, output bounds), §9, §10, §11, §15.
- `outlook-cli-PRD.md` §7 (AUTH-1..4), `teams-cli-PRD.md` §5 and §6.
- `agent-okta-d-PRD.md` §9 (`pkg/client`), §11 (daemon socket API), §17.5 (importable `pkg/client`).
- Go module and versioning behavior (semver tags, `internal/`, `/v2` module paths) is stated from general knowledge of the Go toolchain and was not re-checked against documentation in this session.

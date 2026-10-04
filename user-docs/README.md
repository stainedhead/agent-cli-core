# agent-cli-core user documentation

Guides for developers building a CLI on `agent-cli-core`, the Go library that gives agent-facing CLIs one response envelope, fixed exit codes, token handling that never prints a token, client-side policy, an audit log, a retrying HTTP client, a self-test runner and skill-document generation.

Module path: `github.com/stainedhead/agent-cli-core`. Requires Go 1.27. Supported targets: darwin/arm64, linux/amd64, linux/arm64 (Windows users run the Linux build under WSL2).

## Start here

| Guide | What it covers |
|---|---|
| [Getting started](getting-started.md) | Add the dependency and wire policy, auth, HTTP, output and audit into one command |
| [Configuration reference](configuration.md) | Policy YAML, audit settings, HTTP options, output options; what is a flag or environment variable (nothing is) |
| [Usage examples](examples.md) | Short recipes per package |

## Per-package guides

| Package | Guide |
|---|---|
| `output` | [output.md](output.md) - envelope, exit codes, untrusted content, bounds |
| `auth` and `auth/authtest` | [auth.md](auth.md) - token source, authorizer, fakes |
| `auth/oktad` | [oktad.md](oktad.md) - adapter over the credential daemon (v0.2.0) |
| `clock` | [clock.md](clock.md) - injectable clock with a fake (v0.2.0) |
| `policy` | [policy.md](policy.md) - guardrail policy |
| `audit` | [audit.md](audit.md) - JSONL audit log |
| `httpx` | [httpx.md](httpx.md) - retrying HTTP client |
| `selftest` | [selftest.md](selftest.md) - self-test matrix |
| `docgen` | [docgen.md](docgen.md) - SKILL.md generation |

## Adding the dependency

No version is tagged yet, so there is nothing to add today. The pending release is `v0.2.0` (unreleased until the tag is cut); it adds the daemon adapter `auth/oktad`, which brings `github.com/stainedhead/agent-okta-d` v0.1.0 into your module graph. When it is tagged, depend on it at a released semver tag:

```
go get github.com/stainedhead/agent-cli-core@vX.Y.Z
```

(`vX.Y.Z` is a placeholder.) Do not use pseudo-versions or `replace` directives on your main branch.

## Stability

The library follows semantic versioning. While the version is `0.y.z`, a breaking change bumps the minor version and is called out in the release notes; `1.0.0` is an explicit decision. Everything exported from a package outside `internal/` is public API. The envelope shape, exit codes and audit schema are part of the contract and are pinned by golden tests. A deprecated API is marked `// Deprecated:` and kept for at least one minor release (proposed). An automated API-compatibility check is not wired into CI yet.

## Assumptions and deferred work

Some behavior rests on assumptions that are not yet confirmed, and some features are deliberately not built. Plan around these.

### Assumptions not yet confirmed

- The library defines its own `auth.DaemonClient` interface; the adapter `auth/oktad` (v0.2.0) implements it over the daemon's client v0.1.0. The adapter is tested against the daemon module's fake server, not a live daemon.
- The 401 and refresh behavior of the `auth/authtest` fake models the intended daemon semantics; it has not been verified against a real daemon.
- That one policy schema suits every tool is assumed, not verified. The library ships a generic schema and no tool-specific sample policies.
- Audit failure default: the library default is `warn`. A "block on write operations" default was proposed; because the library cannot tell reads from writes, your tool chooses the mode per Logger.
- The exit code of a failing self-test matrix is `1` (proposed).
- The skill format each agent harness requires is undefined; `docgen` writes one generic Markdown shape.
- The one-minor-release deprecation window is proposed.
- Local overhead of the library has not been measured.

### Deferred (not in this release)

| Not built | Why | What to do meanwhile |
|---|---|---|
| Human-mode login (browser PKCE, OS keychain) | Belongs in the tool that needs it | Implement `auth.TokenSource` (add `Refresh(ctx) (auth.Token, error)` for forced refresh) |
| Conformance test kit | Needs real consumers to define it | Use `auth/authtest` and the examples |
| API-compatibility check in CI | No earlier release to compare with; policy undecided | Read the release notes on upgrade |
| Release automation (tagging, SBOM, signing) | No tag or consumer exists yet | None |
| Policy file signature check | Lower priority | Keep the policy file read-only for the agent user |
| Native Windows | Not a target | Use WSL2 |

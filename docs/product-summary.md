# Product summary

`agent-cli-core` is the shared Go library (module `github.com/stainedhead/agent-cli-core`) behind the agent-facing CLIs `snow`, `outlook` and `teams`. It holds the behavior those tools must share so that each stays thin and an LLM harness sees identical conventions from all of them. It contains no vendor clients and no binary.

## What it provides

| Package | Provides |
|---|---|
| `output` | One response envelope, exit codes 0 to 9, untrusted-content marking, bounded and pageable output (including object data with an array field and an opaque continuation token), json/table/text formats |
| `auth` | Token acquisition behind a `TokenSource` / `DaemonClient` interface; a token type that never prints; typed errors that map to exit 3; `auth/authtest` fakes for tests; `auth/oktad`, the adapter over the credential daemon's Go client (v0.2.0) |
| `policy` | Client-side guardrail policy from strict YAML: verb/resource allow and deny, field allowlists, value constraints, write modes, rate limits, output caps |
| `audit` | Versioned JSON Lines audit log with no secrets and no bodies; optional `rule_id`, `target_ref` and bounded `extra` fields |
| `httpx` | Retrying HTTP transport: jittered backoff, `Retry-After`, idempotency rules, one token refresh on 401, typed errors, redacted tracing |
| `selftest` | Runner for a tool-supplied matrix of expected-allow/deny probes |
| `docgen` | Deterministic `SKILL.md` generation from a command tree, including nested commands |
| `clock` | Public `Clock` interface with `System` and a deterministic `Fake` (v0.2.0) |

## Status

v0.2.0 is implemented and tested on branch `feat/core-v0.2` and is unreleased: the tag is pending. v0.2.0 adds the daemon adapter `auth/oktad` (requires `agent-okta-d` v0.1.0), `output` continuation token and array-field bounds, nested `docgen` commands, a public `clock`, audit extension fields, `policy.CheckTrustedFile` and `httpx` body vendor codes. See "Assumptions and deferred work" in [technical-details.md](technical-details.md) for what is not built.

## Who uses it

Developers of CLIs that an autonomous agent will run. Start at [user-docs/README.md](../user-docs/README.md).

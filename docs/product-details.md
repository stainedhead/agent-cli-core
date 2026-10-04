# Product details

Behavior of the library as built. Requirement identifiers (`CORE-*`, `API-*`) refer to the product requirements document in `specs/archive/`.

## Dependency direction

`agent-okta-d` (credential daemon) <- `agent-cli-core` <- `snow-cli`, `outlook-cli`, `teams-cli`. `auth` does not import `agent-okta-d`; the one package that does is the adapter `auth/oktad` (v0.2.0, unreleased; requires `agent-okta-d` v0.1.0). See [architectural-decision-record.md](architectural-decision-record.md) (ADR-2, ADR-16).

## Output contract (`output`)

- Success: `{"ok":true,"data":...,"meta":{"truncated":false,"next_offset":null,"count":N,"request_id":"...","next_page_token":"..."}}`. `request_id` and `next_page_token` are omitted when empty; `count` is set by `Write` for array data.
- Failure: `{"ok":false,"error":{"code":"<category>","message":"...","hint":"..."}}`. `hint` is omitted when empty. Failure envelopes are never truncated.
- The process exit code always equals the envelope's code: `Envelope.ExitCode()` or `output.ExitOf(err)`.

| Code | Category | Meaning in this library |
|---|---|---|
| 0 | `ok` | success |
| 1 | `general` | any error without a category; a failing `selftest` matrix |
| 2 | `usage` | malformed input: invalid bounds, offset past the end, bound too small, unknown format, invalid selftest matrix |
| 3 | `auth` | daemon unreachable, `reauth_required`, revoked, provider not configured or caller unauthorized (`oktad.AccessError`), any token failure, a 401 that one refresh did not cure |
| 4 | `forbidden` | a 403 (`*httpx.ForbiddenError`) |
| 5 | `not_found` | defined, produced by the tool |
| 6 | `policy_denied` | client policy refusal (the tool maps `policy.Decision`), including a policy rate limit; an untrusted file (`policy.TrustError`) |
| 7 | `conflict` | defined, produced by the tool |
| 8 | `rate_limited` | 429/502/503/504 or network error persisted after bounded retries, or a non-retryable request that got one; the credential daemon degraded or giving a retry hint (`oktad.TransientError`) |
| 9 | `validation` | invalid policy file, invalid `docgen` tree, tool-defined input failures |

Untrusted content: the tool wraps free-text fields in `output.Untrusted`. JSON gets `{"untrusted":true,"value":...,"author":...,"timestamp":...}`; table and text get `<<<UNTRUSTED author="..." timestamp="...">>>` ... `<<<END UNTRUSTED>>>`. Only fields the tool declares are marked. This is a mitigation, not a guarantee.

Bounds: default 32768 bytes. Arrays drop whole trailing items, strings are cut on a character boundary; `meta.truncated` and `meta.next_offset` (an item index for arrays, a byte offset for strings) let the caller resume through `Bounds.Offset`. The tool chooses the flag name (for example `--offset`). With `Bounds.ArrayField` (v0.2.0) the same applies to a named array inside object data such as `{items, total}`, leaving the other fields intact. The opaque `meta.next_page_token`, set by the tool and never cut, carries a vendor's own paging token; resume with `next_offset` first and use the token once `truncated` is false.

## Host safety (`httpx`)

Requests and redirects go only to hosts in `Config.AllowedHosts` (default: the first request's host); plain http to non-loopback hosts needs `AllowInsecureHTTP`. Violations return `*ForbiddenHostError` (code `auth/forbidden-host`, exit 4) before a credential is attached. Transport errors are scrubbed of URL query, userinfo and held credentials.

## Token handling (`auth`)

No exported function returns a token as text. `auth.Token` prints `[redacted]` under every fmt verb (also inside an unexported struct field), JSON, text marshaling and `log/slog`. The only way the value reaches the network is `Authorizer.Authorize` writing the `Authorization` header. There is no fallback credential path: an unreachable daemon is exit 3 and nothing else is tried. The provider name is supplied by the tool. `auth/oktad` (v0.2.0) is the ready-made `auth.DaemonClient` over the credential daemon's socket: it maps daemon failures to existing categories (unreachable, reauth and revoked are exit 3; degraded or retry-hinted is exit 8 with the wait in the hint).

## Policy (`policy`)

Strict YAML, fail closed. Deny rules always win; unmatched requests are denied (`default-deny`). Write modes `allow`, `dry_run_only` (previews only; `Allowed` is false) and `deny`. Rate limits `per_hour` (sliding; only these keep hit timestamps) and `per_run`. A policy is a guardrail, not a security control; server-side permissions remain the boundary. Optional warn/refuse when the policy file is writable by the current user (POSIX only). `policy.CheckTrustedFile` (v0.2.0) is a stricter standalone check that a file and every directory above it are owned by root or a configured uid and not group/world writable; it fails closed and reports `policy_denied`.

## Audit (`audit`)

One JSON line per action: `schema_version, ts, tool, agent_id, run_id, verb, resource, outcome, http_status, duration, policy_decision`, plus optional `rule_id`, `target_ref` and a bounded `extra` string map (v0.2.0; omitted when empty, schema version stays 1). No body or credential field exists; text fields are redacted and capped at 512 bytes. File mode 0600, directory 0700. Write failure mode `warn` (default) or `block`.

## HTTP (`httpx`)

Retries 429, 502, 503, 504 and network errors for idempotent methods or requests marked with `MarkSafeToRetry`, with replayable bodies only. Defaults: 3 retries, 500ms base delay doubling to 30s, 20% jitter, 60s ceiling on any wait including `Retry-After`. One 401 triggers one `Refresh` and one resend from the same budget. Tracing is off by default and redacted. For a 403, `Config.VendorCodeFromBody` (v0.2.0) can derive a vendor code from a bounded prefix of the body; the body is never stored on an error.

## Self-test and skill generation

`selftest` runs a tool-supplied matrix only when the tool calls `Run`; read-only mode skips rows not marked read-only. `docgen` renders a byte-identical `SKILL.md` for the same command tree (commands may be nested up to `docgen.MaxDepth` levels), always including the untrusted-content rule, the envelope and the exit-code table built from `output`.

## Versioning and platforms

Semantic versioning with the pre-1.0 policy in [technical-details.md](technical-details.md). Supported targets: darwin/arm64, linux/amd64, linux/arm64 (Windows users run the Linux build under WSL2). Go 1.27, third-party dependencies `github.com/goccy/go-yaml` (used by `policy`) and, since v0.2.0 (unreleased), `github.com/stainedhead/agent-okta-d` v0.1.0 (used only by `auth/oktad`).

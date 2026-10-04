# Configuration reference

Everything the library can be configured with. The library defines no command-line flags and reads no environment variables itself (the one exception is the daemon client behind `auth/oktad`, which reads `AGENT_OKTA_D_SOCKET` unless you pass a socket path): your CLI decides how values reach it (flags, files, environment) and passes them in. Where a conventional name helps, the table gives a suggested flag.

## Policy file (YAML)

Loaded with `policy.Load(path, opts...)` or `policy.Parse(data)`. Parsing is strict: unknown keys, duplicate keys, an empty file, a wrong `version`, duplicate rule ids, an invalid regular expression, a negative limit, or a deny rule with a `mode` return `*policy.InvalidError`. Treat that as fatal.

| Key | Type | Meaning |
|---|---|---|
| `version` | int, required | Must be `1` |
| `limits.max_results` | int | Cap on result items; 0 or absent means none |
| `limits.max_bytes` | int | Cap on output bytes; 0 or absent means none |
| `rate_limit.per_hour` | int | Allowed requests in any sliding hour, all rules |
| `rate_limit.per_run` | int | Allowed requests since the `Engine` was created |
| `rules[].id` | string, required, unique | Appears in decisions and audit records |
| `rules[].effect` | `allow` or `deny` | A matching deny always wins |
| `rules[].verbs` | list of patterns | `*` matches any run of characters |
| `rules[].resources` | list of patterns | Same |
| `rules[].mode` | `allow` (default), `dry_run_only`, `deny` | Allow rules only |
| `rules[].fields` | list of names | Allowlist of request field names; absent means no allowlist |
| `rules[].constraints.<field>` | object | `enum` (list), `pattern` (regexp), `max_len` (characters), `min`, `max` (numeric only) |
| `rules[].rate_limit` | `per_hour`, `per_run` | Applies to this rule only |

Requests no rule allows are denied with rule id `default-deny`. Only requests that would run (`allow` mode) consume rate budget.

`policy.Load` options:

| Option | Values | Meaning |
|---|---|---|
| `policy.WithWritable(m)` | `WritableWarn` (default), `WritableRefuse`, `WritableIgnore` | What to do when the current user can write the policy file or its directory. POSIX only. Warnings are in `p.Warnings()`; refusal returns `*policy.WritableError` |

`policy.NewEngine(p, clock)`: `clock` is any value with `Now() time.Time` (a [`clock.Clock`](clock.md) works); `nil` means real time.

`policy.CheckTrustedFile(path, opts...)` (v0.2.0) is a separate call, not a `Load` option. `policy.WithTrustedUIDs(uids...)` adds trusted owners besides root. See [policy](policy.md).

The policy file is a guardrail, not a security control. Keep it where the agent user cannot edit it.

## Audit

`audit.Config` (JSON or YAML tags in brackets):

| Field | Type | Meaning |
|---|---|---|
| `Path` [`path`] | string, required | Log file. No default; empty returns `audit.ErrNoPath`. Created with mode 0600 (an existing wider file is tightened), parent directory 0700 |
| `OnFailure` [`on_failure`] | `warn` (default) or `block` | Both parse from text, case-insensitively |

`audit.Open(cfg, opts...)` and `audit.NewLogger(w, opts...)` options:

| Option | Meaning |
|---|---|
| `WithFailureMode(m)` | `audit.Warn` or `audit.Block` (alternative to `Config.OnFailure`) |
| `WithOnWriteError(f)` | Called in warn mode with the failed write's error; print it on stderr |
| `WithSecrets(s...)` | Literal values that must never be written |
| `WithClock(c)` | Timestamp source for tests; a [`clock.Clock`](clock.md). Nil keeps the system clock |

Record schema (version 1): `schema_version, ts, tool, agent_id, run_id, verb, resource, outcome, http_status, duration, policy_decision`, plus optional `rule_id`, `target_ref`, `extra` (v0.2.0). Text fields are redacted and capped at 512 bytes. `extra`: at most 16 keys matching `[a-z0-9_.-]{1,32}`, values redacted and cut to 256 bytes.

## HTTP (`httpx.Config`)

The zero value is usable.

| Field | Default | Meaning |
|---|---|---|
| `MaxRetries` | 3 | Re-sends after the first attempt, one budget for transient retries and the 401 refresh. Negative disables retries and refresh |
| `BaseDelay` | 500ms | First backoff delay; doubles per retry |
| `MaxDelay` | 30s | Cap on the backoff |
| `MaxWait` | 60s | Cap on any single wait, `Retry-After` included |
| `Jitter` | 0.2 | Plus or minus fraction applied to backoff; negative disables |
| `Clock` | real | `Now()` and `Sleep(ctx, d) error`; inject a fake in tests |
| `Rand` | random | `func() float64` in [0,1) for jitter |
| `Refresher` | none | Authorizes each attempt and refreshes once on 401. `*auth.Authorizer` fits. Without it a 401 is `*AuthError` |
| `VendorCode` | none | `func(http.Header) string` extracting a vendor code from a 403's headers |
| `VendorCodeFromBody` | none | `func(status int, prefix []byte) string` extracting a vendor code from a 403's body when the headers gave none (v0.2.0) |
| `VendorBodyLimit` | 4096 | Bytes of body the hook sees; capped at 65536 (v0.2.0) |
| `Trace` | nil (off) | `io.Writer` receiving one redacted line per attempt; leave nil in agent mode |
| `AllowedHosts` | first request's host | Hosts the client may contact (`host` for any port, `host:port` to pin). Others, including redirect targets, fail with `*httpx.ForbiddenHostError` (exit 4) before any credential is attached |
| `AllowInsecureHTTP` | false | Permit plain `http` to non-loopback hosts; loopback is always allowed |
| `Redactor` | default | Cannot be constructed outside the module; leave nil |

Per request: `httpx.MarkSafeToRetry(req)` allows retrying a POST or PATCH that carries an idempotency key. The package constants `DefaultMaxRetries`, `DefaultBaseDelay`, `DefaultMaxDelay`, `DefaultMaxWait`, `DefaultJitter` hold the defaults.

## Auth

| Setting | Meaning |
|---|---|
| provider name (`NewDaemonTokenSource` argument) | Required; the library has no default |
| `auth.WithRemediation(text)` | Tool-specific instruction added to the re-enrollment hint |
| daemon socket | With `auth/oktad`: `oktad.WithSocketPath(path)`, else the `AGENT_OKTA_D_SOCKET` environment variable read by the daemon client, else the platform default. With your own `DaemonClient` it is yours. `authtest.WithSocket(path)` sets the name a fake reports |
| `oktad.WithTimeout(d)` | Per-request timeout of the daemon client; zero or less keeps its default |

## Output (`output.Options`)

| Field | Default | Meaning | Suggested flag |
|---|---|---|---|
| `Format` | `json` | `json`, `table` or `text`; `output.ParseFormat` parses the string | `--format` |
| `Bounds.MaxBytes` | 32768 | Largest output of one write; negative is invalid | `--max-bytes` |
| `Bounds.Offset` | 0 | Where to resume: item index for arrays, byte offset for strings; use the previous `meta.next_offset` | `--offset` |
| `Bounds.ArrayField` | empty | Top-level key of object data whose array is bounded; `Offset` then counts items of that array (v0.2.0) | none |
| `Secrets` | none | Literal values scrubbed from error messages and hints | none |

The envelope's `Meta.NextPageToken` (v0.2.0) is set by your tool, not by an option; see [output](output.md).

Use `policy` `Limits.ClampResults(n)` and `ClampBytes(n)` to combine policy caps with your own.

## Self-test (`selftest.Runner`)

| Field | Meaning | Suggested flag |
|---|---|---|
| `Rows` | The matrix: `Name`, `Verb`, `Resource`, `Expect` (`allow` or `deny`), `ReadOnly` | none |
| `Probe` | Required function that exercises a row | none |
| `ReadOnly` | Run only rows marked `ReadOnly`; others are skipped | `--read-only` |

## Skill generation (`docgen.CommandTree`)

`Name` (required; letters, digits, `.`, `_`, `-`), `Description`, and `Commands` each with `Name` (unique among siblings), `Description`, `Usage`, `Examples`, `Forbidden`, and `Subcommands` (same fields, up to 4 levels deep; v0.2.0).

## Build-time requirements

Go 1.27; targets darwin/arm64, linux/amd64, linux/arm64. Third-party modules: `github.com/goccy/go-yaml` and, from v0.2.0, `github.com/stainedhead/agent-okta-d` v0.1.0 (only for `auth/oktad`).

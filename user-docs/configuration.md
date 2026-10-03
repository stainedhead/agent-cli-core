# Configuration reference

Everything the library can be configured with. The library defines no command-line flags and reads no environment variables: your CLI decides how values reach it (flags, files, environment) and passes them in. Where a PRD-style name is useful, the table says "suggested flag".

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

`policy.NewEngine(p, clock)`: `clock` is any value with `Now() time.Time`; `nil` means real time.

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
| `WithClock(c)` | Timestamp source for tests; any value with `Now() time.Time` and `Sleep(ctx, d) error` |

Record schema (version 1): `schema_version, ts, tool, agent_id, run_id, verb, resource, outcome, http_status, duration, policy_decision`. Text fields are redacted and capped at 512 bytes.

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
| `Trace` | nil (off) | `io.Writer` receiving one redacted line per attempt; leave nil in agent mode |
| `Redactor` | default | Cannot be constructed outside the module; leave nil |

Per request: `httpx.MarkSafeToRetry(req)` allows retrying a POST or PATCH that carries an idempotency key. The package constants `DefaultMaxRetries`, `DefaultBaseDelay`, `DefaultMaxDelay`, `DefaultMaxWait`, `DefaultJitter` hold the defaults.

## Auth

| Setting | Meaning |
|---|---|
| provider name (`NewDaemonTokenSource` argument) | Required; the library has no default |
| `auth.WithRemediation(text)` | Tool-specific instruction added to the re-enrollment hint |
| daemon socket | Owned by your `DaemonClient` implementation; the library never reads one. `authtest.WithSocket(path)` sets the name a fake reports |

## Output (`output.Options`)

| Field | Default | Meaning | Suggested flag |
|---|---|---|---|
| `Format` | `json` | `json`, `table` or `text`; `output.ParseFormat` parses the string | `--format` |
| `Bounds.MaxBytes` | 32768 | Largest output of one write; negative is invalid | `--max-bytes` |
| `Bounds.Offset` | 0 | Where to resume: item index for arrays, byte offset for strings; use the previous `meta.next_offset` | `--offset` |
| `Secrets` | none | Literal values scrubbed from error messages and hints | none |

Use `policy` `Limits.ClampResults(n)` and `ClampBytes(n)` to combine policy caps with your own.

## Self-test (`selftest.Runner`)

| Field | Meaning | Suggested flag |
|---|---|---|
| `Rows` | The matrix: `Name`, `Verb`, `Resource`, `Expect` (`allow` or `deny`), `ReadOnly` | none |
| `Probe` | Required function that exercises a row | none |
| `ReadOnly` | Run only rows marked `ReadOnly`; others are skipped | `--read-only` |

## Skill generation (`docgen.CommandTree`)

`Name` (required; letters, digits, `.`, `_`, `-`), `Description`, and `Commands` each with `Name` (unique), `Description`, `Usage`, `Examples`, `Forbidden`.

## Build-time requirements

Go 1.27; targets darwin/arm64, linux/amd64, linux/arm64. One third-party dependency, `github.com/goccy/go-yaml`.

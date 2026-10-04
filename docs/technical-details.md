# Technical details

Implementation-level design: package layout, the dependency rule, and the public API of every package. Each package has one `## API: <pkg>` section, maintained by the workstream that owns the package.

## Package layout and dependency rule

`agent-cli-core` is a library (module `github.com/stainedhead/agent-cli-core`, Go 1.27, no binary). It knows nothing of any vendor: no ServiceNow, Graph or Teams code, names or identifiers. Dependencies point inward:

```
clock, internal/redact            (leaf)
internal/clock -> clock            (type aliases)
output     -> internal/redact
httpx      -> output, internal/redact, internal/clock
auth       -> output, internal/redact, internal/clock      (never httpx)
auth/oktad -> auth, output, agent-okta-d/pkg/client        (the only importer of the daemon client)
policy     -> output, goccy/go-yaml
audit      -> internal/redact, internal/clock, clock
selftest   -> output
docgen     -> output
```

`internal/archtest` enforces this graph, import cycles, the dependency allow-list and the no-vendor-name rule (`go test ./internal/archtest`). `auth` and `httpx` meet only through the structural `httpx.TokenRefresher` interface (see `## API: httpx`).

Third-party dependencies: `github.com/goccy/go-yaml`, for `policy`, and (since v0.2.0, unreleased) `github.com/stainedhead/agent-okta-d` v0.1.0, imported only by `auth/oktad`, resolved from the Go proxy with no `replace`. Consumers of v0.2.0 gain the daemon module in their module graph.

The `goccy/go-yaml` library was chosen because it is maintained and supports strict decoding that rejects unknown keys (`yaml.Strict()` / `DisallowUnknownField`), which `policy` needs to fail closed. `gopkg.in/yaml.v3` offers `KnownFields(true)` too but is archived upstream.

The public package `clock` (v0.2.0) provides `Clock` (`Now`, `Sleep`), the real `System` clock and a deterministic `Fake` (`NewFake`, `Advance`, `SetAutoAdvance`, `Slept`, `Waiters`, `BlockUntil`). `internal/clock` keeps its names as type aliases of it. `audit.WithClock` takes a `clock.Clock`; `policy.Clock` and `httpx.Clock` stay declared in their packages and public clocks satisfy them structurally. `internal/redact` provides `Redactor` (`New`, `String`, `Header`, `Body`, `Error`). A bare `bearer`/`basic` is redacted only after an `authorization` key or when the next word looks like a credential (prose such as "basic usage" stays); runs of `OpaqueRunMin` (40) characters are redacted except a 40-digit lowercase hex git SHA-1.

## API: output

Package `output` is the contract between a CLI and the LLM harness: one envelope, exit codes 0 to 9, untrusted-content marking, bounded output. Imports `internal/redact` only. All identifiers below are public API and governed by semver; the envelope shape, exit codes and category names are pinned by golden tests in `output/testdata`.

### Exit codes and categories

| Code | `ExitCode` constant | `Category` (error.code) |
|---|---|---|
| 0 | `ExitOK` | `CategoryOK` = `ok` |
| 1 | `ExitGeneral` | `CategoryGeneral` = `general` |
| 2 | `ExitUsage` | `CategoryUsage` = `usage` |
| 3 | `ExitAuth` | `CategoryAuth` = `auth` |
| 4 | `ExitForbidden` | `CategoryForbidden` = `forbidden` |
| 5 | `ExitNotFound` | `CategoryNotFound` = `not_found` |
| 6 | `ExitPolicyDenied` | `CategoryPolicyDenied` = `policy_denied` |
| 7 | `ExitConflict` | `CategoryConflict` = `conflict` |
| 8 | `ExitRateLimited` | `CategoryRateLimited` = `rate_limited` |
| 9 | `ExitValidation` | `CategoryValidation` = `validation` |

- `type ExitCode int`, `type Category string`.
- `func Categories() []Category` returns all ten in exit-code order.
- `func ExitFor(Category) ExitCode`: each category maps to exactly one code; an unknown or empty category maps to `ExitGeneral`.
- `type CategoryError interface { error; Category() Category }`: implemented structurally by errors in other packages, so they reach an exit code without importing each other.
- `type Hinter interface { Hint() string }`: optional, supplies `error.hint`.
- `func CategoryOf(err error) Category`: `ok` for nil, the first `CategoryError` in the `errors.As` chain, else `general`.
- `func ExitOf(err error) ExitCode` is `ExitFor(CategoryOf(err))`.

### Envelope

- `type Envelope struct { OK bool; Data any; Meta *Meta; Error *Error }`; `(Envelope).ExitCode() ExitCode` (0 for success, else the error's category; a failure with no `Error` is 1); `(Envelope).MarshalJSON()` emits `{"ok","data","meta"}` or `{"ok","error"}`.
- `type Meta struct { Truncated bool; NextOffset *int; Count int; RequestID string; NextPageToken string }` with JSON keys `truncated`, `next_offset` (null when absent), `count`, `request_id` (omitted when empty), `next_page_token` (omitted when empty; v0.2.0). `NextPageToken` is an opaque value the tool sets (for example a vendor paging token); `Write` passes it through, counts it in the byte budget, never cuts it, and keeps it when output is truncated. Resume with `next_offset` first and use the token once `truncated` is false. Table and text footers show `next_page_token=` after `request_id`. Put only non-secret values in it.
- `type Error struct { Code Category; Message, Hint string }` with JSON keys `code`, `message`, `hint` (omitted when empty).
- `func Success(data any, meta *Meta) Envelope`.
- `func Failure(c Category, message, hint string) Envelope`: message and hint are redacted with the built-in patterns. `FailureWithSecrets(c, message, hint, secrets...)` also removes the literal secrets.
- `func FromError(err error) Envelope`: nil gives a success envelope with no data; otherwise code from `CategoryOf`, message `err.Error()`, hint from a `Hinter` found with `errors.As` (wrapped and `errors.Join`ed errors included), redacted. `FromErrorWithSecrets(err, secrets...)` also removes literal secrets.

### Untrusted content

- `type Untrusted struct { Value, Author string; Timestamp time.Time }`. The tool decides which fields are free text and wraps them; undeclared fields are never marked.
- JSON: `{"untrusted":true,"value":"...","author":"...","timestamp":"RFC 3339 UTC"}` (author and timestamp omitted when empty).
- Table and text output: `<<<UNTRUSTED author="..." timestamp="...">>>`, the value, `<<<END UNTRUSTED>>>` (one line in table cells, with line breaks escaped). Any `<<<` inside the value is rewritten to `<< <` so content cannot close its own block; the author and timestamp are quoted.
- `(Untrusted).String()` returns the text form, so `fmt` also marks it.

### Writing, formats and bounds

- `type Format string`; `FormatJSON` (default), `FormatTable`, `FormatText`; `func ParseFormat(string) (Format, error)` (empty means JSON).
- `type Bounds struct { MaxBytes, Offset int; ArrayField string }`; `const DefaultMaxBytes = 32768`. `MaxBytes` 0 means the default; negative is invalid.
- `type Options struct { Format Format; Bounds Bounds; Secrets []string }`.
- `func Write(w io.Writer, env Envelope, opts Options) error` and `func Render(env Envelope, opts Options) ([]byte, error)` (the bytes `Write` writes). Output ends with a newline. JSON is compact on one line and does not escape HTML characters.
- Error envelopes are never truncated. Their message and hint are redacted again at write time, together with the literal `Options.Secrets`.
- Success data is marshaled with `encoding/json` (struct field order is kept). `Write` sets `meta`: `count` for array data (the items in this page), and `truncated` and `next_offset` when data is cut.
- Truncation: array data drops whole trailing items; string data is cut at a rune boundary; the output (in the chosen format, footer included) never exceeds `MaxBytes`, stays valid JSON and valid UTF-8, and `next_offset` is the index of the first dropped item (arrays) or the byte offset of the first dropped byte (strings). Passing it as `Bounds.Offset` resumes; paging to the end reassembles the original data (property test). Invalid UTF-8 in a string is replaced with U+FFFD before offsets are computed. An `Offset` inside a character moves forward to the next character start.
- Data that is neither an array nor a string (objects, numbers) cannot be cut; if it exceeds the bound `Write` fails, unless `Bounds.ArrayField` is set.
- `Bounds.ArrayField` (v0.2.0) names a top-level key of object data (struct or map) whose value is an array (`null` counts as empty and stays `null`). `Write` drops whole trailing items of that array only; key order and the bytes of every other field are preserved. `Offset` skips that many items, `Meta.Count` is the items kept, and `next_offset` is an absolute item index (`Offset` + kept), as for array data. `Offset == len` gives an empty page; `Offset > len` returns `ErrOffsetOutOfRange`. A missing key, a non-array value, or non-object data returns `ErrArrayField` (usage, exit 2); a single item larger than the budget returns `ErrBoundTooSmall`. Only top-level keys are supported, not paths. Failure envelopes ignore it; unset, output is byte-identical to v0.1.0.
- Table: an array of objects becomes aligned columns (first-seen key order, header and separator rows); an object becomes `key`/`value` rows. Text: `key: value` lines, arrays as `-` items. Both end with `-- count=N truncated=B [next_offset=N] [request_id=ID] [next_page_token=T]` (`count` only for arrays). Errors render as `error: <code>: <message>` and `hint: <hint>`.
- Sentinel errors (all `CategoryUsage`, exit 2): `ErrInvalidBounds`, `ErrOffsetOutOfRange` (offset beyond the data, or a non-zero offset on non-sliceable data), `ErrBoundTooSmall` (smallest unit does not fit), `ErrArrayField` (v0.2.0), `ErrUnknownFormat`.

### Tests

Golden files in `output/testdata` pin every format of success, error, untrusted, truncated and resumed output and the exit-code table (`go test ./output -update` rewrites them; review the diff, a change is a breaking change). A paging property test covers multi-byte text at cut points. Runnable examples: `ExampleWrite`, `ExampleWrite_table`, `ExampleWrite_truncated`, `ExampleWrite_failure`, `ExampleFromError`, `ExampleExitOf`, `ExampleUntrusted`.

## API: auth

Package `auth` (`github.com/stainedhead/agent-cli-core/auth`) obtains short-lived bearer tokens. Imports: `output`, `internal/redact` only. It never imports `httpx` or the credential daemon's module; the daemon adapter is the separate package `auth/oktad`.

- `Token` - redacting value: `String`, `GoString`, `Format` (every fmt verb), `MarshalJSON`, `MarshalText` and `LogValue` all give `[redacted]`. `NewToken(string)` builds one, `IsZero()` tests it; there is no accessor. The value is held in a closure, so it also stays out of fmt output when a Token sits in an unexported struct field; in-process reflect/unsafe access and memory dumps remain out of scope. The value reaches the network only through `Authorizer.Authorize`.
- `TokenSource` `{Token(ctx) (Token, error)}`; optional `Refresher` `{Refresh(ctx) (Token, error)}`.
- `DaemonClient` `{Fetch(ctx, provider) (Token, error); Refresh(ctx, provider) (Token, error)}` - the adapter point; `auth/oktad` implements it for the real daemon.
- `NewDaemonTokenSource(client, provider, ...Option) (*DaemonTokenSource, error)`: provider is required (no default), no caching, no fallback. `WithRemediation(text)` adds the tool's exact re-enrollment instruction.
- `NewAuthorizer(TokenSource) *Authorizer` with `Authorize(ctx, *http.Request) error` (sets `Authorization: Bearer ...` itself) and `Refresh(ctx) error` - exactly the methods of `httpx.TokenRefresher`; `Refresh` returns `ErrRefreshUnsupported` for a source that is not a `Refresher`.
- Errors, all `output.CategoryAuth` (exit 3) and `output.Hinter` where useful: `ErrReauthRequired`, `ErrRevoked`, `ErrRefreshUnsupported` (sentinels); `*UnreachableError{Socket, Err}` (message names the socket and says the service may not be running); `*ActionRequiredError{Provider, Err, Remediation}` (generic human-action hint plus remediation); `*TokenError{Provider, Op, Err}` (other failures, message scrubbed).
- `auth/authtest`: `Fake` (a `DaemonClient`) with `Scenario` `Valid`, `ExpiredNeedsRefresh`, `ReauthRequired`, `Revoked`, `Unreachable`, `UnauthorizedThenSuccess`, `UnauthorizedTwice`; `New(scenario, WithSocket(path))`; `Handler()` is the fake resource server (200 or 401); counters `Fetches`, `Refreshes`, `Requests`, `Providers`.

Deferred: human-mode sources (PKCE login, OS keychain). A tool can supply its own `TokenSource`.

### API: auth/oktad (v0.2.0, unreleased)

Package `auth/oktad` implements `auth.DaemonClient` over `github.com/stainedhead/agent-okta-d/pkg/client` (v0.1.0). It is the only package that imports that client and the only vendor-exempt package in `internal/archtest`.

- `New(...Option) *Client`; `WithSocketPath(path)` (overrides `AGENT_OKTA_D_SOCKET` and the platform default; empty ignored), `WithTimeout(d)` (per-request; zero or less ignored). `(*Client)` has `Fetch`, `Refresh`, `SocketPath`, `Close` (idempotent; the client stays usable). The zero `Client` returns a plain error and makes no socket call. `Fetch`/`Refresh` pass the daemon's token to `auth.NewToken` at a single place; it is never formatted or logged.
- Error mapping, evaluated top to bottom (`errors.Is`/`errors.As`):

| Cause | Result | Category | Exit |
|---|---|---|---|
| caller's context cancelled or past deadline | `oktad: ` + context error | general | 1 |
| daemon unreachable | `*auth.UnreachableError{Socket, Err}` | auth | 3 |
| `reauth_required` (401) | wraps `auth.ErrReauthRequired` (`DaemonTokenSource` turns it into `*auth.ActionRequiredError`) | auth | 3 |
| revoked (403) | wraps `auth.ErrRevoked`, same | auth | 3 |
| degraded (503), or any `*APIError` with a retry hint | `*oktad.TransientError` | rate_limited | 8 |
| not configured (404), unauthorized (403) | `*oktad.AccessError` | auth | 3 |
| invalid response, empty provider, other | plain wrapped error | general | 1 |

- `*TransientError{Wait, Err}`: `RetryAfter() time.Duration` (zero if none), `Hint()` names the wait in whole seconds rounded up, `Category()`, `Unwrap()`. `output.Error` has no `retry_after` field, so the number reaches the envelope only through `error.hint`; consumers read it with `errors.As` or `interface{ RetryAfter() time.Duration }`.
- `*AccessError{Provider, Err}`: `Hint()`, `Category()` (auth, exit 3, not 4: these are local setup problems a human fixes), `Unwrap()`.
- The tests run against the fake daemon server from the daemon module's `clienttest` package; behaviour against a live daemon is not verified here.

Tests: table tests with the fake; `TestNoExportedFunctionReturnsTokenText` scans the package so no exported function returns string or bytes other than the redacted formatters and error text; leak tests across fmt, JSON, slog and error strings. Examples: `ExampleToken`, `ExampleAuthorizer`, `ExampleUnreachableError`, `ExampleWithRemediation`.

## API: policy

Package `policy` evaluates a client-side guardrail policy loaded from strict YAML. Imports `output` (for `TrustError` only), the standard library and `github.com/goccy/go-yaml`. **A policy is a guardrail, not a security control**: an allowed decision does not mean the server will allow the action; server-side permissions remain the boundary.

| Symbol | Purpose |
|---|---|
| `Parse([]byte) (*Policy, error)`, `Load(path, ...Option) (*Policy, error)` | Strict parse. Unknown or duplicate keys, an empty file, a bad version, duplicate rule ids, bad patterns or negative limits return `*InvalidError`; no `Policy` is returned (fail closed). `Load` also checks writability. |
| `Policy`, `Rule`, `Constraint`, `Rate`, `Limits`, `Effect`, `Mode`, `Version` | The schema. `Effect` is `allow`/`deny`; `Mode` is `allow`/`dry_run_only`/`deny`. |
| `Request{Verb, Resource, Fields}` | What the tool asks about. Verb and resource are opaque strings; `*` in rule patterns matches any run of characters. |
| `(*Policy).Evaluate(Request) Decision` | Pure decision, no rate limits. Deny rules always win; the first allow rule whose field allowlist and constraints hold decides; nothing matching is denied (`DefaultDenyID`). A nil policy denies. |
| `Decision{Allowed, Mode, RuleID, Reason, RetryAfter}`, `DryRunOnly()`, `Err()`, `DeniedError` | Decision as data. `Allowed` is true only for mode `allow`; `dry_run_only` has `Allowed=false` so a caller checking only `Allowed` fails safe. `Err()` returns `*DeniedError` for any non-allowed decision; the caller maps it to `output.CategoryPolicyDenied` (exit 6) and records `RuleID` and `Reason` as the audit `policy_decision`. |
| `NewEngine(*Policy, Clock)`, `(*Engine).Check`, `Clock` | Adds global and per-rule rate limits (`per_hour` sliding window, `per_run` since engine creation), mutex-protected. Only `allow` decisions consume budget; a limit denial carries `RetryAfter`. the public `clock.System`/`clock.Fake` satisfy `Clock`; nil means real time. |
| `Limits{MaxResults, MaxBytes}`, `ClampResults`, `ClampBytes` | Caps the tool feeds into the output bounds. Zero means no cap. |
| `WithWritable(WritableWarn\|WritableRefuse\|WritableIgnore)`, `WritableError`, `(*Policy).Warnings()` | POSIX only: warn (default) or refuse when the current user can write the policy file or its directory. |
| `CheckTrustedFile(path, ...TrustOption) error`, `WithTrustedUIDs(uids...)`, `*TrustError{Path, Reason, Err}`, `ErrNotTrusted` (v0.2.0) | Standalone check that a file (for example a policy or config file) can only be changed by a trusted user. See below. |

`DeniedError`, `InvalidError` and `WritableError` do not implement `output.CategoryError`; the caller maps them. A parse failure should be reported as category `validation` and the tool must not run. `*TrustError` is the exception (v0.2.0): it implements `output.CategoryError` (`policy_denied`, exit 6) and `Hinter`, and matches `errors.Is(err, ErrNotTrusted)`.

### CheckTrustedFile (v0.2.0)

Trusted owners are root plus any uids passed to `WithTrustedUIDs`. The effective uid is never accepted unless it is root: passing it returns a `*TrustError`, because the agent's own user can rewrite its own file. The path is made absolute and walked component by component with `lstat`; symlinks are resolved by the check (each link's owner must be trusted; at most 40 hops, loops fail), and every directory on the real route, `/` included, must be owned by a trusted uid and not group/world writable (`mode&022`). A sticky world-writable ancestor such as `/tmp` is rejected. The final check opens the file `O_RDONLY|O_NOFOLLOW|O_NONBLOCK`, then `fstat`s the descriptor (regular file, trusted owner, not group/world writable) and compares it with `os.SameFile` against the earlier `lstat`, so a swap between the walk and that open fails; non-regular files are rejected before opening. **Guarantee and limits:** at check time no untrusted user could have written the file or any directory on its resolved path, and none can change it afterwards unless a trusted user or root does. The function opens, checks and closes the file and returns only an error, no descriptor, so the caller's later open of the path is a separate step; safety there rests on every ancestor being trusted and not group/world writable. POSIX ACLs and extended attributes are not examined. Everything fails closed: missing file, permission error on an ancestor, and non-unix platforms (always a `*TrustError`). `Load` and `WritableMode` are unchanged.

## API: audit

Package `audit` writes a versioned JSON Lines audit log. Imports only `internal/redact` and the public `clock`.

- `SchemaVersion` (const, 1); `Record` {SchemaVersion, Timestamp, Tool, AgentID, RunID, Verb, Resource, Outcome, HTTPStatus, Duration, PolicyDecision, RuleID, TargetRef, Extra} with JSON keys `schema_version, ts, tool, agent_id, run_id, verb, resource, outcome, http_status, duration, policy_decision, rule_id, target_ref, extra`. The last three are v0.2.0, optional and omitted when empty; `SchemaVersion` stays 1 and output with them unset is byte-identical to v0.1.0 (readers must tolerate unknown keys). `Extra` is `*ExtraFields` (`type ExtraFields map[string]string`), a pointer to a named map so `Record` stays comparable with `==`; callers write `Extra: &audit.ExtraFields{"k": "v"}`. Limits: at most `MaxExtraKeys` (16) keys, keys match `[a-z0-9_.-]{1,32}` (`MaxExtraKeyLen`), values are redacted and cut to `MaxExtraValueLen` (256) bytes on a rune boundary; `RuleID` and `TargetRef` use the normal redact and 512-byte cap. Too many keys or a bad key writes nothing and `Log` returns a `*WriteError` matching both `ErrWrite` and `ErrInvalidExtra`. The caller's map is never mutated; keys serialise sorted. No body or credential field exists. Pinned by `audit/testdata/golden.jsonl`.
- `Config` {Path, OnFailure}; `Open(Config, ...Option) (*Logger, error)` (file mode 0600, parent 0700, append); `NewLogger(io.Writer, ...Option)`; `(*Logger).Log(Record) error`, `Handle(Record, actionErr) error`, `Close() error`. Safe for concurrent use; one atomic line per record.
- Options: `WithClock(clock.Clock)` (nil keeps the system clock), `WithSecrets`, `WithFailureMode`, `WithOnWriteError`.
- `WriteFailureMode` (`Warn` default, `Block`) with text marshaling; `*WriteError`, `ErrWrite`, `ErrNoPath`.
- Every string field is redacted (`internal/redact`) and capped at 512 bytes before writing. Write failures are always returned from `Log`; `Handle` in `Block` mode returns them joined with the action's error.

## API: httpx

Frozen cross-package contract, already declared in `httpx/doc.go`:

```go
type TokenRefresher interface {
    Authorize(ctx context.Context, req *http.Request) error // sets the Authorization header itself
    Refresh(ctx context.Context) error                      // forces one token refresh
}
```

Package `httpx` imports `output`, `internal/redact` and `internal/clock` only (never `auth`). All identifiers are public API under semver.

- `Config` {`MaxRetries` (0 = 3, negative = none), `BaseDelay`, `MaxDelay`, `MaxWait`, `Jitter`, `Clock`, `Rand`, `Refresher TokenRefresher`, `VendorCode func(http.Header) string`, `VendorCodeFromBody func(status int, prefix []byte) string`, `VendorBodyLimit int`, `Trace io.Writer`, `Redactor`, `AllowedHosts []string`, `AllowInsecureHTTP bool`}; zero fields take the `Default*` constants.
- `Clock` {`Now`, `Sleep(ctx, d) error`}: structurally satisfied by `internal/clock` (`System`, `Fake`).
- `NewTransport(base http.RoundTripper, cfg Config) *Transport` (implements `http.RoundTripper`); `NewClient(cfg Config) *http.Client`.
- `MarkSafeToRetry(*http.Request) *http.Request`, `IsMarkedSafe(*http.Request) bool`.
- Errors (all implement `output.CategoryError` and `output.Hinter`): `*RateLimitedError{Status, Attempts, Err}` (exit 8), `*AuthError{Err}` (exit 3), `*ForbiddenError{VendorCode}` (exit 4), `*ForbiddenHostError{Host, Insecure}` (exit 4, `Code()` = `auth/forbidden-host`). None carries a response body. Transport errors in `RateLimitedError.Err` and non-retried send errors are scrubbed (configured redactor plus the credentials the call attached) and the query string and userinfo of any `*url.Error` URL are dropped from the message; the original error stays reachable through `errors.As`.

Semantics: one attempt budget (`MaxRetries` re-sends) is shared by transient retries and the single 401 refresh. Idempotent methods (GET, HEAD, OPTIONS, TRACE, PUT, DELETE) or requests marked safe are retried on 429, 502, 503, 504 and network errors, only if the body is replayable (`GetBody`). `Retry-After` (seconds or HTTP date) on 429/503 replaces the backoff; every wait is capped at `MaxWait`; waits use the injected `Clock`, and context cancellation aborts them. Backoff is `BaseDelay * 2^n` capped at `MaxDelay`, varied by plus or minus `Jitter`. A 401 triggers `Refresh` once and a resend (allowed for non-idempotent requests, since the server rejected it unprocessed); a second 401, a missing refresher, an exhausted budget or an unreplayable body gives `*AuthError`. A 403 gives `*ForbiddenError` with a vendor code (from headers via `VendorCode`, redacted, at most 64 bytes). v0.2.0: if no header code results, `VendorCodeFromBody` (403 only) is called with up to `VendorBodyLimit` bytes of the body (default `DefaultVendorBodyLimit` 4096, capped at `MaxVendorBodyLimit` 64 KiB; read with `io.LimitReader`, skipped on a read error), then the body is drained and closed so the connection is reused. A non-empty header code wins and the body is not read. The result is trimmed, control characters removed, scrubbed and cut to 64 bytes on a rune boundary; no body prefix is stored on any error type, and a panic in the hook is not recovered. 401 and 429 are unchanged. Tracing is off by default; when on it writes one redacted line per attempt, never bodies, with the URL reduced to scheme, host and path.

Host safety (FR-001): every request is checked before it is authorized or sent. The host must be in `AllowedHosts` (entries are `host` for any port or `host:port`; empty means the first request's host is pinned), and plain `http` is refused for non-loopback hosts unless `AllowInsecureHTTP` is set; otherwise `*ForbiddenHostError` is returned, traced, and no `Authorization` is attached. `NewClient` installs a `CheckRedirect` applying the same check (and the 10-redirect limit), so a cross-host redirect is refused and never carries credentials. `NewTransport` used under a caller's own `http.Client` still refuses the redirected request at the transport.

## API: selftest

Imports `output` and the standard library only.

- `Outcome` (`Allow`, `Deny`), `Status` (`StatusPass`, `StatusFail`, `StatusSkip`).
- `Row{Name, Verb, Resource, Expect, ReadOnly}`: one matrix line. `Probe func(ctx, Row) (Outcome, error)`: supplied by the tool; an error fails the row.
- `Runner{Rows, Probe, ReadOnly}` and `Run(ctx) (Result, error)`: runs rows in order; with `ReadOnly` set, non-read-only rows are skipped. Returns a usage-category error (exit 2) for a nil probe or an invalid `Expect`, and the context error on cancellation.
- `Result{Rows []RowResult, Passed, Failed, Skipped}`; `RowResult` embeds `Row` plus `Status`, `Actual`, `Detail`. `OK()`, `Envelope()` (success with the result as data, or a general-category failure naming each failing row), `ExitCode()` (0 or 1), `Write(w, output.Options) (output.ExitCode, error)`.

The runner executes only when the tool calls `Run` (an explicit selftest command); it does no network access of its own. See `user-docs/selftest.md`.

## API: docgen

Package `docgen` renders a deterministic SKILL.md from a command tree. It imports only `output` and the standard library.

- `type CommandTree struct{ Name, Description string; Commands []Command }` - the tool. `Name` is required (letters, digits, `.`, `_`, `-`).
- `type Command struct{ Name, Description, Usage string; Examples, Forbidden []string; Subcommands []Command }` - one command. Names are unique among siblings (the same name under different parents is allowed). `const MaxDepth = 4`.
- `func Generate(tree CommandTree) ([]byte, error)` - front matter (`name`, `description`), a `## Commands` section (commands sorted by name, `Forbidden` sorted, `Examples` in given order), then the fixed sections. The input is not modified. No timestamps; the same tree always gives the same bytes.
- `const SharedSkill = "agent-cli-core"` - the shared conventions skill that the last section points to instead of duplicating policy, credential and output-bound guidance.
- `type Error` - returned for an invalid tree; implements `output.CategoryError` with `CategoryValidation` (exit 9).

Always-included sections: `## Untrusted content` (the rule, plus the JSON and text marking built from `output.Untrusted`), `## Output envelope` (success and failure examples marshaled from `output.Success` and `output.Failure`), `## Exit codes` (one row per `output.Categories()` entry with `output.ExitFor`) and `## Shared conventions`. Nested commands (v0.2.0): siblings are sorted by name and rendered depth-first under their parent, with heading level 3 plus depth minus 1 and the full path as title (`### mail`, `#### mail send`); a parent renders its own text and then its children. Validation errors (validation category): empty name, control characters, duplicate sibling names after trimming, depth over `MaxDepth`. `Usage` and `Examples` stay caller text and should carry the full invocation. An empty `Subcommands` is a leaf, and flat trees render byte-identically to v0.1.0. A test fails if `output` gains a category with no meaning text in `docgen`.

User-supplied text is single-lined (descriptions, forbidden items) or placed in a code fence longer than any backtick run it contains (usage, examples), so it cannot inject headings or front matter. The golden file is `docgen/testdata/SKILL.golden.md`; refresh it with `go test ./docgen -update`.

## Exported API summary

`go doc -all ./<pkg>` is authoritative. Every exported identifier outside `internal/` is public API (API-2).

| Package | Exported identifiers |
|---|---|
| `output` | `DefaultMaxBytes`; `ErrInvalidBounds`, `ErrOffsetOutOfRange`, `ErrBoundTooSmall`, `ErrArrayField`, `ErrUnknownFormat`; `Envelope` (+ `ExitCode`, `MarshalJSON`), `Success`, `Failure`, `FailureWithSecrets`, `FromError`, `FromErrorWithSecrets`; `Meta` (incl. `NextPageToken`), `Error`; `Category`, `Category*` constants, `Categories`, `CategoryOf`, `CategoryError`, `Hinter`; `ExitCode`, `Exit*` constants, `ExitFor`, `ExitOf`; `Format`, `Format*`, `ParseFormat`; `Bounds` (incl. `ArrayField`), `Options`, `Write`, `Render`; `Untrusted` |
| `auth` | `Token`, `NewToken`; `TokenSource`, `Refresher`, `DaemonClient`; `NewDaemonTokenSource`, `DaemonTokenSource`, `Option`, `WithRemediation`; `Authorizer`, `NewAuthorizer`; `ErrReauthRequired`, `ErrRevoked`, `ErrRefreshUnsupported`; `UnreachableError`, `ActionRequiredError`, `TokenError` |
| `auth/oktad` | `Client`, `New`, `Option`, `WithSocketPath`, `WithTimeout`, `TransientError`, `AccessError` |
| `clock` | `Clock`, `System`, `Fake`, `NewFake` |
| `auth/authtest` | `Fake`, `New`, `Option`, `WithSocket`, `DefaultSocket`; `Scenario` with `Valid`, `ExpiredNeedsRefresh`, `ReauthRequired`, `Revoked`, `Unreachable`, `UnauthorizedThenSuccess`, `UnauthorizedTwice` |
| `policy` | `Version`, `DefaultDenyID`; `Policy`, `Rule`, `Constraint`, `Rate`, `Limits`, `Effect`, `Mode` (+ constants); `Parse`, `Load`, `Option`, `WithWritable`, `WritableMode` (`WritableWarn`, `WritableRefuse`, `WritableIgnore`); `Request`, `Decision`, `DeniedError`, `Engine`, `NewEngine`, `Clock`; `InvalidError`, `WritableError`; `CheckTrustedFile`, `TrustOption`, `WithTrustedUIDs`, `TrustError`, `ErrNotTrusted` |
| `audit` | `SchemaVersion`, `MaxExtraKeys`, `MaxExtraKeyLen`, `MaxExtraValueLen`, `ErrNoPath`, `ErrWrite`, `ErrInvalidExtra`; `ExtraFields`; `Config`, `Open`, `NewLogger`, `Logger`, `Record`; `Option`, `WithClock`, `WithSecrets`, `WithFailureMode`, `WithOnWriteError`; `WriteFailureMode` (`Warn`, `Block`), `WriteError` |
| `httpx` | `DefaultMaxRetries`, `DefaultBaseDelay`, `DefaultMaxDelay`, `DefaultMaxWait`, `DefaultJitter`, `DefaultVendorBodyLimit`, `MaxVendorBodyLimit`; `Config`, `Clock`, `TokenRefresher`, `Transport`, `NewTransport`, `NewClient`; `MarkSafeToRetry`, `IsMarkedSafe`; `RateLimitedError`, `AuthError`, `ForbiddenError` |
| `selftest` | `Outcome` (`Allow`, `Deny`), `Status` (`StatusPass`, `StatusFail`, `StatusSkip`), `Row`, `RowResult`, `Probe`, `Runner`, `Result` |
| `docgen` | `SharedSkill`, `MaxDepth`, `CommandTree`, `Command`, `Generate`, `Error` |

Notes a consumer should know:

- `httpx.Config.Redactor` mentions a type from `internal/`. Outside this module the `Redactor` field can only be left nil (a default is used). Since v0.2.0 `audit.WithClock` takes the importable `clock.Clock`; `httpx.Clock` and `policy.Clock` remain declared in their packages and public clocks satisfy them structurally.
- `policy` imports `output` only for `TrustError`; the other policy errors carry no category and the caller maps them (denial to `policy_denied`, invalid policy to `validation`).
- Adding fields to `output.Meta`, `output.Bounds`, `docgen.Command` and `audit.Record` breaks unkeyed struct literals of those types; use keyed literals.

## Versioning and API stability (PRD API-1..API-9)

| ID | Policy | State in this repository |
|---|---|---|
| API-1 | Semantic versioning; git tag `vX.Y.Z` is the module version | No tag exists yet. v0.2.0 is the pending release (unreleased; tag not cut) |
| API-2 | Everything exported outside `internal/` is public API | Enforced by layout; `internal/` holds `redact`, `clock` (aliases of the public `clock`), `archtest`, `integration` |
| API-3 | Pre-1.0: a breaking change bumps the minor version (`0.y.z` to `0.(y+1).0`), noted in release notes; `1.0.0` is an explicit decision | Policy only |
| API-4 | After 1.0 a break needs a new major version | Policy only |
| API-5 | Deprecate with `// Deprecated:` and a replacement, kept at least one minor release (proposed, unconfirmed) | Policy only |
| API-6 | A major version 2 or higher needs a `/v2` module path and a migration of every consumer; avoided by a small surface and `internal/` | Policy only |
| API-7 | CI compares the API against the previous release (`apidiff` or `gorelease`) | Documented, NOT wired: no previous release exists to compare with; tool choice and blocking-versus-advisory are undecided |
| API-8 | Envelope, exit codes and audit schema are API contract, pinned by golden tests | Implemented: `output/testdata`, `audit/testdata/golden.jsonl`, `docgen/testdata`, `selftest/testdata` |
| API-9 | A bad release is superseded, never deleted or re-tagged | Policy only; no release workflow yet |

## Platform targets

Supported: darwin/arm64, linux/amd64, linux/arm64, built with `CGO_ENABLED=0`. Native Windows is not a target; Windows users run the Linux build under WSL2, which is a Linux environment. The policy writable-file check uses a `unix` build-tagged implementation and finds nothing on other platforms. CI compiles and vets all three targets and runs the tests natively on Linux and on macOS arm64.

## Build, test and CI

- Gate: `gofmt -l .` empty, `go vet ./...`, `golangci-lint run`, `go test -race ./...`, `go mod tidy` leaves no diff, cross-compile for the three targets, `govulncheck`. `make check` runs the local part; `make cross`, `make vuln`, `make fuzz` and `make tidy-check` run the rest individually.
- `.github/workflows/ci.yml` runs on pull requests and manual dispatch only, with read-only permissions and no secrets. There is no release, publish or deploy job. The `downstream` job (matrix: snow-cli, outlook-cli, teams-cli, all public) checks out each consumer's `main` next to this pull request's core, links them with a `go.work` file (the consumer's `go.mod` is not edited, so its no-replace guard tests still run) and runs build, vet and `go test -race`; it verifies backward compatibility.
- Tests: golden files for the envelope, exit codes, audit schema, selftest output and generated skill document; `internal/archtest` (import graph, cycles, allowed third-party modules, no vendor names); `internal/integration` (end-to-end flow through `examples/sampletool`, and a fuzz/property test that no token value appears in any envelope, error, trace or audit line).
- `examples/sampletool` is a vendor-neutral sample tool showing the order policy, auth, httpx, output, audit. It is documentation that compiles and is tested, not API.

## Assumptions and deferred work

Assumptions the PRD marks unconfirmed or proposed, and how the code treats them:

| Item | PRD status | Treatment |
|---|---|---|
| Go surface of the daemon's `pkg/client` (types, how `reauth_required` is signalled) | was unconfirmed | Confirmed against `agent-okta-d` v0.1.0; `auth/oktad` maps its errors (see `## API: auth`). Tested against a fake daemon, not a live one |
| 401 scenarios match the daemon's refresh semantics | unverified | `authtest` models them; not checked against a real daemon |
| Common policy schema across `snow`, `outlook`, `teams` | assumed possible, unverified | Generic schema shipped, no tool-specific sample policies |
| Audit write failure default "block write operations" | proposed | Mode is per Logger (`warn` default, `block`); the library cannot tell reads from writes, so a tool wanting the proposed default chooses per call |
| Failing selftest exit code `1` | proposed | Implemented as exit 1 (`general`) |
| Skill format required by each harness | unconfirmed | `docgen` emits one generic Markdown shape with `name` and `description` front matter |
| Deprecation window of one minor release (API-5) | proposed | Policy text only |
| `apidiff` / `gorelease` as the compatibility tool (API-7) | not verified | Not wired |
| Local overhead under 50 ms | not measured | No benchmark |

Deferred (not built), with rationale:

| Item | Rationale |
|---|---|
| Human-mode login (browser PKCE, OS keychain) `TokenSource` | Decided to live in `snow-cli`; the `TokenSource` interface already allows it |
| Conformance test kit | Needs real consumers to define it; revisit once a release is adopted |
| `apidiff` / `gorelease` in CI | No earlier release to compare with; blocking-versus-advisory is undecided |
| Release workflows: tagging, GitHub Release, SBOM, provenance, signing (the pull-request `downstream` compatibility job exists) | No consumer code and no tag exist; CI is verify-only |
| Policy file signature check (CORE-POL-7, P2) | Lower priority |
| Native Windows support | Out of scope; WSL2 uses the Linux build |
| Pinning CI actions to commit SHAs | Needs network lookups; actions are pinned to release tags |

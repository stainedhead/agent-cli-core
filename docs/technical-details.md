# Technical details

Implementation-level design: package layout, the dependency rule, and the public API of every package. Each package has one `## API: <pkg>` section, maintained by the workstream that owns the package.

## Package layout and dependency rule

`agent-cli-core` is a library (module `github.com/stainedhead/agent-cli-core`, Go 1.27, no binary). It knows nothing of any vendor: no ServiceNow, Graph or Teams code, names or identifiers. Dependencies point inward:

```
internal/redact, internal/clock   (leaf)
output   -> internal/redact
httpx    -> output, internal/redact, internal/clock
auth     -> output, internal/redact, internal/clock      (never httpx)
policy   -> goccy/go-yaml only
audit    -> internal/redact, internal/clock
selftest -> output
docgen   -> output
```

`internal/archtest` enforces this graph, import cycles, the dependency allow-list and the no-vendor-name rule (`go test ./internal/archtest`). `auth` and `httpx` meet only through the structural `httpx.TokenRefresher` interface (see `## API: httpx`).

Third-party dependency: `github.com/goccy/go-yaml`, for `policy`. It was chosen because it is maintained and supports strict decoding that rejects unknown keys (`yaml.Strict()` / `DisallowUnknownField`), which `policy` needs to fail closed. `gopkg.in/yaml.v3` offers `KnownFields(true)` too but is archived upstream.

`internal/clock` provides `Clock` (`Now`, `Sleep`), the real `System` clock and a deterministic `Fake`. `internal/redact` provides `Redactor` (`New`, `String`, `Header`, `Body`, `Error`).

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
- `type Meta struct { Truncated bool; NextOffset *int; Count int; RequestID string }` with JSON keys `truncated`, `next_offset` (null when absent), `count`, `request_id` (omitted when empty).
- `type Error struct { Code Category; Message, Hint string }` with JSON keys `code`, `message`, `hint` (omitted when empty).
- `func Success(data any, meta *Meta) Envelope`.
- `func Failure(c Category, message, hint string) Envelope`: message and hint are redacted.
- `func FromError(err error) Envelope`: nil gives a success envelope with no data; otherwise code from `CategoryOf`, message `err.Error()`, hint from a `Hinter` in the chain, redacted.

### Untrusted content

- `type Untrusted struct { Value, Author string; Timestamp time.Time }`. The tool decides which fields are free text and wraps them; undeclared fields are never marked.
- JSON: `{"untrusted":true,"value":"...","author":"...","timestamp":"RFC 3339 UTC"}` (author and timestamp omitted when empty).
- Table and text output: `<<<UNTRUSTED author="..." timestamp="...">>>`, the value, `<<<END UNTRUSTED>>>` (one line in table cells, with line breaks escaped). Any `<<<` inside the value is rewritten to `<< <` so content cannot close its own block; the author and timestamp are quoted.
- `(Untrusted).String()` returns the text form, so `fmt` also marks it.

### Writing, formats and bounds

- `type Format string`; `FormatJSON` (default), `FormatTable`, `FormatText`; `func ParseFormat(string) (Format, error)` (empty means JSON).
- `type Bounds struct { MaxBytes, Offset int }`; `const DefaultMaxBytes = 32768`. `MaxBytes` 0 means the default; negative is invalid.
- `type Options struct { Format Format; Bounds Bounds; Secrets []string }`.
- `func Write(w io.Writer, env Envelope, opts Options) error` and `func Render(env Envelope, opts Options) ([]byte, error)` (the bytes `Write` writes). Output ends with a newline. JSON is compact on one line and does not escape HTML characters.
- Error envelopes are never truncated. Their message and hint are redacted again at write time, together with the literal `Options.Secrets`.
- Success data is marshaled with `encoding/json` (struct field order is kept). `Write` sets `meta`: `count` for array data (the items in this page), and `truncated` and `next_offset` when data is cut.
- Truncation: array data drops whole trailing items; string data is cut at a rune boundary; the output (in the chosen format, footer included) never exceeds `MaxBytes`, stays valid JSON and valid UTF-8, and `next_offset` is the index of the first dropped item (arrays) or the byte offset of the first dropped byte (strings). Passing it as `Bounds.Offset` resumes; paging to the end reassembles the original data (property test). Invalid UTF-8 in a string is replaced with U+FFFD before offsets are computed. An `Offset` inside a character moves forward to the next character start.
- Data that is neither an array nor a string (objects, numbers) cannot be cut; if it exceeds the bound `Write` fails.
- Table: an array of objects becomes aligned columns (first-seen key order, header and separator rows); an object becomes `key`/`value` rows. Text: `key: value` lines, arrays as `-` items. Both end with `-- count=N truncated=B [next_offset=N] [request_id=ID]` (`count` only for arrays). Errors render as `error: <code>: <message>` and `hint: <hint>`.
- Sentinel errors (all `CategoryUsage`, exit 2): `ErrInvalidBounds`, `ErrOffsetOutOfRange` (offset beyond the data, or a non-zero offset on non-sliceable data), `ErrBoundTooSmall` (smallest unit does not fit), `ErrUnknownFormat`.

### Tests

Golden files in `output/testdata` pin every format of success, error, untrusted, truncated and resumed output and the exit-code table (`go test ./output -update` rewrites them; review the diff, a change is a breaking change). A paging property test covers multi-byte text at cut points. Runnable examples: `ExampleWrite`, `ExampleWrite_table`, `ExampleWrite_truncated`, `ExampleWrite_failure`, `ExampleFromError`, `ExampleExitOf`, `ExampleUntrusted`.

## API: auth

Package `auth` (`github.com/stainedhead/agent-cli-core/auth`) obtains short-lived bearer tokens. Imports: `output`, `internal/redact` only. It never imports `httpx` or the credential daemon's module.

- `Token` - redacting value: `String`, `GoString`, `Format` (every fmt verb), `MarshalJSON`, `MarshalText` and `LogValue` all give `[redacted]`. `NewToken(string)` builds one, `IsZero()` tests it; there is no accessor. The value reaches the network only through `Authorizer.Authorize`.
- `TokenSource` `{Token(ctx) (Token, error)}`; optional `Refresher` `{Refresh(ctx) (Token, error)}`.
- `DaemonClient` `{Fetch(ctx, provider) (Token, error); Refresh(ctx, provider) (Token, error)}` - the adapter point.
- `NewDaemonTokenSource(client, provider, ...Option) (*DaemonTokenSource, error)`: provider is required (no default), no caching, no fallback. `WithRemediation(text)` adds the tool's exact re-enrollment instruction.
- `NewAuthorizer(TokenSource) *Authorizer` with `Authorize(ctx, *http.Request) error` (sets `Authorization: Bearer ...` itself) and `Refresh(ctx) error` - exactly the methods of `httpx.TokenRefresher`; `Refresh` returns `ErrRefreshUnsupported` for a source that is not a `Refresher`.
- Errors, all `output.CategoryAuth` (exit 3) and `output.Hinter` where useful: `ErrReauthRequired`, `ErrRevoked`, `ErrRefreshUnsupported` (sentinels); `*UnreachableError{Socket, Err}` (message names the socket and says the service may not be running); `*ActionRequiredError{Provider, Err, Remediation}` (generic human-action hint plus remediation); `*TokenError{Provider, Op, Err}` (other failures, message scrubbed).
- `auth/authtest`: `Fake` (a `DaemonClient`) with `Scenario` `Valid`, `ExpiredNeedsRefresh`, `ReauthRequired`, `Revoked`, `Unreachable`, `UnauthorizedThenSuccess`, `UnauthorizedTwice`; `New(scenario, WithSocket(path))`; `Handler()` is the fake resource server (200 or 401); counters `Fetches`, `Refreshes`, `Requests`, `Providers`.

Deferred: the adapter over the daemon's `pkg/client` (no tagged release yet; its Go surface is unconfirmed) and human-mode sources (PKCE login, OS keychain). A tool can supply its own `TokenSource`.

Tests: table tests with the fake; `TestNoExportedFunctionReturnsTokenText` scans the package so no exported function returns string or bytes other than the redacted formatters and error text; leak tests across fmt, JSON, slog and error strings. Examples: `ExampleToken`, `ExampleAuthorizer`, `ExampleUnreachableError`, `ExampleWithRemediation`.

## API: policy

Not yet implemented. This section is filled in by the workstream that owns `policy`.

## API: audit

Not yet implemented. This section is filled in by the workstream that owns `audit`.

## API: httpx

Frozen cross-package contract, already declared in `httpx/doc.go`:

```go
type TokenRefresher interface {
    Authorize(ctx context.Context, req *http.Request) error // sets the Authorization header itself
    Refresh(ctx context.Context) error                      // forces one token refresh
}
```

The rest of the package is not yet implemented. This section is filled in by the workstream that owns `httpx`.

## API: selftest

Not yet implemented. This section is filled in by the workstream that owns `selftest`.

## API: docgen

Not yet implemented. This section is filled in by the workstream that owns `docgen`.

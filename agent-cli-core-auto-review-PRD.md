# agent-cli-core Automated Code Review (Step 5)

Branch: feat/agent-cli-core vs main. Reviewer: dev-flow review-code. Spec: specs/261003-agent-cli-core.

## Executive summary

Overall quality is high. Architecture is clean (leaf `internal/redact`, `internal/clock`, `output` as the shared contract, archtest guarding the import graph), the public surface is small, and tests are behavioral with golden files. Verified locally on go1.27.1: `go build`, `go vet`, `go test -race -count=1 ./...` and `golangci-lint run` all pass (0 issues). Coverage is 90-100% in every package.

Weak spot: credential handling at the edges. The token-redaction guarantee holds for direct use but was broken in two ways I reproduced with throwaway tests (since removed). One of them, bearer-token forwarding on cross-host redirects, is a P0 for a library whose purpose is safe agent credential handling.

Counts: P0 = 1, P1 = 1, P2 = 5.

## Functional requirements (findings)

### FR-1 (P0) Bearer token is re-attached to cross-host redirect targets
Evidence: `auth/source.go:113-128` (`Authorize` sets `Authorization` unconditionally) called per attempt from `httpx/transport.go:147-162` (`prepare`) and `httpx/transport.go:73`. `http.Client` strips `Authorization` on a cross-domain redirect, but it then calls `Transport.RoundTrip` for the new request, and `prepare` sets the header again. Reproduced: `httpx.NewClient` with `auth.NewAuthorizer`, first server 302-redirects to `http://localhost:<port>/x`; the second host received `Authorization: Bearer <token>`. `NewClient` (`httpx/transport.go:68`) installs no `CheckRedirect`. Also sends over plain `http://`.
Why it matters: an upstream open redirect, or a malicious server, exfiltrates the token to an arbitrary host; this defeats the library's core guarantee (docs/product-details.md:34, auth/doc.go:15).
Acceptance criteria:
- Config gains an allowed-host list (or the Transport binds to the first request's host); `Authorize` is skipped, and the request fails with a typed error, for any other host.
- `NewClient` sets a `CheckRedirect` that refuses cross-host redirects (or drops auth) by default.
- Plain `http` to a non-loopback host is refused unless explicitly enabled.
- Tests: redirect to a different hostname never carries `Authorization`; same-host redirect still works.

### FR-2 (P1) `auth.Token` leaks when held in an unexported struct field
Evidence: `auth/token.go:12-16,31-32`. `fmt` cannot call `Format`/`String` on unexported fields, so it prints the inner struct. Reproduced: `struct{ tok auth.Token }` printed `{tok:{v:SECRETTOKENVALUE99}}` (`%+v`), `{{SECRETTOKENVALUE99}}` (`%v`), `auth.Token{v:"SECRETTOKENVALUE99"}` (`%#v`). Contradicts docs/technical-details.md:90 and docs/product-details.md:34 ("under every fmt verb").
Why it matters: tools commonly keep a Token in a private field of a client struct that gets logged or dumped in a panic or debug print.
Acceptance criteria:
- Token no longer stores the secret as a plain string field readable by reflection: store it behind a pointer to a type with the redacting methods (e.g. `*secret`) or a closure, so every print path shows `[redacted]`.
- Test prints a struct with an unexported Token field under `%v %+v %#v %s %x` and asserts the value is absent.
- Docs state any remaining limits (e.g. `reflect`, memory dumps).

### FR-3 (P2) Transport and error messages carry unscrubbed text
Evidence: `httpx/errors.go:35-36` appends `e.Err.Error()` unscrubbed; `httpx/transport.go:104` wraps `sendErr` raw. `http.Client` additionally wraps in `*url.Error` containing the full URL including query. Reproduced: `Get "http://.../p?api_key=SUPERSECRETQUERY1": rate limited ...` (the trace path scrubs via `safeURL`, errors do not). The built-in redactor only masks well-known key names, so a caller's own query secrets are exposed in the error and then in the envelope.
Acceptance criteria:
- `RateLimitedError.Err` message is passed through `Config.Redactor` (and held credentials) before being stored or formatted.
- Documentation states that URL queries must not carry secrets, or `NewClient` strips the query from `url.Error` text.
- Test: a transport error containing a registered secret never appears in `Error()`.

### FR-4 (P2) Envelope failure path ignores registered secrets
Evidence: `output/envelope.go:53-55` uses `redact.New()` with no secrets; `FromError` (`:62-71`) has no way to pass them, whereas `Write` accepts secrets (`output/write.go:139`). A literal secret that is not pattern-shaped (short or non-opaque, e.g. an API key like `abc-123-xyz`) in an error message is emitted by `FromError`.
Acceptance criteria:
- `FromError`/`Failure` have a variant or option accepting secrets (or docs direct callers to always go through `Write` and `Write` is verified to rescrub the error), with a test using a non-pattern literal secret in an error.

### FR-5 (P2) Redactor over-matches ordinary words
Evidence: `internal/redact/redact.go:23` `reAuthScheme` matches any `bearer|basic` followed by a word. Reproduced: `"use basic authentication; bearer of news"` -> `"use [redacted]; [redacted] news"`. Also `reOpaque` (`:28`) redacts any 40+ char identifier (commit SHAs, long resource names), which corrupts audit `Resource` and error messages.
Acceptance criteria:
- Auth-scheme pattern requires `Authorization:`/header context or a minimum credential length/charset; table tests for prose containing "basic" and "bearer".
- Document (or make configurable) the opaque-run threshold; git SHA-1 (40 hex) handling decided and tested.

### FR-6 (P2) `findHinter` ignores multi-error chains
Evidence: `output/envelope.go:73-86` only follows `Unwrap() error`, while `CategoryOf` uses `errors.As` (`output/exit.go`), which also traverses `Unwrap() []error` (`audit.Handle` returns `errors.Join` in Block mode, `audit/audit.go:281`). A joined error gets its category but no hint.
Acceptance criteria: hint lookup uses `errors.As(err, &Hinter)` semantics; test with `errors.Join(actionErr, audit.WriteError)`.

### FR-7 (P2) Policy engine hit history grows without bound
Evidence: `policy/engine.go:77-80` appends to `e.hits[key]` on every allowed request for every limit, but `prune` runs only when `PerHour > 0` (`:69-70`). With no hourly limit (or per-run only) the slice grows for the life of the process; the global key is always appended.
Acceptance criteria: record hits only for keys with `PerHour > 0`; test that `len(e.hits)` stays zero for a policy with only per-run limits, and that a long run with hourly limits stays bounded.

## Verified non-findings
- Token JSON/text/slog/fmt on a bare value is redacted; audit fields are redacted, capped and JSON-escaped (no line injection); log file is 0600; writes are mutex-serialized.
- CI is least privilege, pinned tool versions, tidy/gofmt/vet/race/lint/vuln plus three-target compile and native darwin test. Minor: actions are pinned to tags, not SHAs (already tracked in the workflow comment).
- No P0 outside FR-1; no Clean Architecture inversions found (archtest passes).

## Verdict
Request changes: fix FR-1 (P0) and FR-2 (P1) before merge; FR-3 to FR-7 can follow.

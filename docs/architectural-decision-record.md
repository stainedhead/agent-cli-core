# Architectural decision record

One entry per decision: context, decision, consequences. Status is "accepted" unless noted.

## ADR-1: Its own repository, a library, no vendor code

Context: three CLIs (`snow`, `outlook`, `teams`) must behave identically to an LLM harness. Decided 2026-10-03.
Decision: shared behavior lives in `github.com/stainedhead/agent-cli-core`. It is a library (no binary, no container image). It contains no vendor clients and knows nothing of ServiceNow, Graph or Teams.
Consequences: the CLIs stay separate binaries in separate repositories that import this module. `internal/archtest` fails the build if a vendor name appears in an import path, identifier or file path.

## ADR-2: `auth` defines its own `DaemonClient`; `agent-okta-d` is not imported

Context: the daemon's `pkg/client` has no tagged release and its Go surface (types, how `reauth_required` is signalled) is unconfirmed. Importing it would block compilation and couple the public API to an unknown.
Decision: `auth` declares `TokenSource`, `Refresher` and `DaemonClient` (`Fetch`, `Refresh`) and the error vocabulary (`ErrReauthRequired`, `ErrRevoked`, `*UnreachableError`). A fake lives in `auth/authtest`. `go.mod` has no `agent-okta-d` requirement.
Consequences: the library compiles and is fully tested today. The future adapter over `pkg/client` is the single deferred integration point: it implements `auth.DaemonClient`, is the only importer of `pkg/client`, and maps the daemon's signals to the sentinel errors. Nothing else changes. Until then a consumer writes its own small adapter.

## ADR-3: Fake daemon in `auth/authtest`

Context: tool authors need a fake credential daemon and resource server in their tests.
Decision: ship it as an exported sub-package `auth/authtest`, not in `internal/`, so other modules can import it.
Consequences: it is public API under semver. It is test support only and holds no real credentials.

## ADR-4: `auth` and `httpx` meet through a structural `TokenRefresher`

Context: a 401 must trigger exactly one refresh and one resend, and the retry budget is owned by `httpx`; `auth` must not import `httpx` and vice versa.
Decision: `httpx` declares `TokenRefresher { Authorize(ctx, *http.Request) error; Refresh(ctx) error }`. `*auth.Authorizer` has exactly those methods, so it satisfies the interface with no glue. `Authorize` sets the `Authorization` header itself, so a token never leaves `auth` except into that header. The signature is frozen; changing it is a breaking change.
Consequences: no import edge between the packages. `internal/integration` asserts the satisfaction at compile time. The `Authorizer` does not cache; the daemon owns caching.

## ADR-5: `internal/redact` is a leaf package

Context: the PRD places the redactor in `httpx`, but `output` and `audit` also need it, and `output` must not import `httpx`.
Decision: the redactor lives in `internal/redact`, with `internal/clock` as the other leaf. `output`, `httpx`, `auth` and `audit` import it; it imports nothing in the module.
Consequences: one scrubbing implementation for envelopes, traces, errors and audit lines. It is not public, so consumers pass literal secrets through `output.Options.Secrets` and `audit.WithSecrets`. `httpx.Config.Redactor` cannot be constructed outside the module and is effectively internal. `httpx` additionally scrubs the exact `Authorization` values it sent during a call, because a server that echoed the credential into another header leaked through pattern-only redaction (found by the leak fuzz test).

## ADR-6: Import graph enforced by a test

Decision: the allowed intra-module edges are fixed (`output` to `redact`; `httpx` to `output`, `redact`, `clock`; `auth` to `output`, `redact`, `clock`; `policy` to nothing internal; `audit` to `redact`, `clock`; `selftest` and `docgen` to `output`) and checked by `internal/archtest` along with cycles and the third-party allow-list.
Consequences: `policy` cannot import `output`, so its errors do not implement `output.CategoryError` and the caller maps them. `policy` declares its own one-method `Clock`; `audit` carries the policy decision as a plain string.

## ADR-7: YAML library `github.com/goccy/go-yaml`

Context: the policy must fail closed on unknown or duplicate keys.
Decision: use `goccy/go-yaml` (v1.19.2) with strict decoding. `gopkg.in/yaml.v3` also has `KnownFields(true)` but is archived upstream.
Consequences: one third-party dependency, used only by `policy`. Everything else is the standard library.

## ADR-8: Policy is a guardrail; `dry_run_only` fails safe

Decision: a policy decision is plain data and never claims the server will allow the action. `dry_run_only` decisions have `Allowed=false` and are recognized by `DryRunOnly()`, so a caller checking only `Allowed` never performs a write by mistake. Deny rules win regardless of order; unmatched requests are denied (`default-deny`). Only allowed decisions consume rate budget. The writable-file check warns by default and refuses on request.

## ADR-9: Audit has no body field and a per-Logger failure mode

Decision: `audit.Record` has no field that can hold a body or credential; text is redacted and capped at 512 bytes. The write-failure mode (`warn` or `block`) is a Logger setting because the library cannot classify verbs as reads or writes.
Consequences: the PRD's proposed "block write operations" default is achieved by the tool choosing a mode per call or using two Loggers.

## ADR-10: Retry semantics in `httpx`

Decision: `MaxRetries` is one budget for transient retries and the single 401 refresh. Statuses 429, 502, 503, 504 and network errors become `*RateLimitedError` (exit 8) when the budget is spent or the request is not eligible for retry. `Retry-After` replaces the backoff and is capped by `MaxWait`, without jitter. The vendor error code of a 403 comes only from response headers through `Config.VendorCode`, so the body is never read. Statuses such as 404, 409 and 500 pass through as responses.

## ADR-11: Exit codes are an API contract

Decision: the envelope shape, exit codes and category names are pinned by golden tests (API-8). Failure envelopes carry no data, so a failing selftest matrix is a `general` failure whose message names the failing rows.

## ADR-12: Platform targets

Decision: darwin/arm64, linux/amd64, linux/arm64; no native Windows. Windows users run the Linux build under WSL2, which is Linux. The writable-file check is build-tagged for Unix.

## ADR-13: Deferrals

Decided in the feature spec (2026-10-03):

- Human-mode PKCE login and OS keychain: belongs in `snow-cli`; the `TokenSource` interface only has to allow it.
- Conformance test kit: needs real consumers; revisit after the first release.
- `apidiff` wiring: documented as API-7, not enforced; no earlier release to compare with and blocking-versus-advisory is undecided.
- Release automation (tagging, GitHub Release, SBOM, provenance, signing) and the downstream compatibility build: no consumer code or tag exists, so CI is verify-only.
- The `pkg/client` adapter: see ADR-2.
- Policy file signature check (P2).

## ADR-14: Open items

- Who owns the policy schema across the tools (a generic schema ships; ownership is unassigned).
- First tagged `agent-okta-d` release containing `pkg/client` (gates only the adapter).
- The shared-skill update in the root repository is a manual pull request per release.

## ADR-15: Hosts are allow-listed, redirects checked

Context: auto-review found a bearer token could follow a cross-host redirect or travel over plain http.
Decision: `httpx` pins hosts (`AllowedHosts`, default first request's host), refuses non-loopback plain http unless `AllowInsecureHTTP`, and installs `CheckRedirect` in `NewClient`; the transport also checks every request, so custom clients are covered. Redaction of bare `bearer`/`basic` became context-sensitive and a 40-hex git SHA-1 is preserved. `auth.Token` stores its value in a closure so reflection-based printing cannot reveal it.
Consequences: a legitimate cross-host redirect needs the target in `AllowedHosts`; short all-letter bare credentials outside an `authorization` context are no longer pattern-redacted (register them as literal secrets).

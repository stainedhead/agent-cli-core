# httpx

A retrying HTTP client for CLIs built on `agent-cli-core`: jittered backoff, `Retry-After` handling, idempotency rules, a single token refresh on 401, typed errors that map to exit codes, and redacted tracing (off by default).

```go
import "github.com/stainedhead/agent-cli-core/httpx"

client := httpx.NewClient(httpx.Config{
    Refresher:  authorizer, // anything with Authorize(ctx, *http.Request) and Refresh(ctx)
    VendorCode: func(h http.Header) string { return h.Get("X-Error-Code") },
})
resp, err := client.Do(req)
```

`NewClient` returns a standard `*http.Client`; `NewTransport(base, cfg)` returns just the `http.RoundTripper`. Pass the request context: cancelling it stops waits and retries immediately.

## What is retried

- Statuses 429, 502, 503, 504 and network errors, with exponential backoff (base 500ms, doubling, capped at 30s) varied by plus or minus `Jitter` (default 20%).
- For 429 and 503 a `Retry-After` header (seconds or HTTP date) replaces the backoff. Every wait, `Retry-After` included, is capped at `MaxWait` (default 60s).
- Only idempotent requests (GET, HEAD, OPTIONS, TRACE, PUT, DELETE) are retried. A POST or PATCH is never retried unless you call `httpx.MarkSafeToRetry(req)`, for example because it carries an idempotency key the server honors.
- A request with a body is retried only when its body can be replayed (`req.GetBody` set; `http.NewRequest` does this for strings, bytes and buffers).
- `MaxRetries` (default 3) is one budget for all re-sends, including the 401 refresh. A negative value disables retries.

## Errors

| Situation | Error | Exit code |
|---|---|---|
| Retries exhausted, or a non-retryable request got 429/502/503/504 | `*RateLimitedError` | 8 |
| 401 after one refresh, no `Refresher`, or the refresh failed | `*AuthError` | 3 |
| 403 | `*ForbiddenError` (`VendorCode` from your header or body extractor) | 4 |
| Host not allowed, cross-host redirect, or plain http to a non-loopback host | `*ForbiddenHostError` (code `auth/forbidden-host`) | 4 |

Other statuses (404, 409, 500, ...) are returned as ordinary responses for your code to interpret. The typed errors never contain response bodies. Network errors inside `*RateLimitedError`, and errors from requests that are not retried, are scrubbed: the URL query string and any `user:password@` part are dropped, and the credentials attached to the call are removed from the text. The original error stays reachable with `errors.As`. `http.Client` wraps transport errors in `*url.Error`; use `errors.As`, or `output.ExitOf(err)` for the exit code.

## Authentication

`Config.Refresher` is called to authorize every attempt. On a 401 the transport calls `Refresh` once and resends once; a second 401 is an `*AuthError`. `auth.Authorizer` from this module satisfies the interface, but `httpx` does not import `auth`.

## Vendor code from the response body (v0.2.0)

Some services put the reason for a 403 in the JSON body, not in a header. Set `Config.VendorCodeFromBody`:

```go
cfg := httpx.Config{
	VendorCodeFromBody: func(status int, prefix []byte) string { /* parse prefix, return a code */ },
	VendorBodyLimit:    2048, // optional
}
```

- It applies to 403 only. 401 and 429 keep their typed errors.
- A non-empty code from `Config.VendorCode` (headers) wins, and then the body is not read at all.
- The hook sees at most `VendorBodyLimit` bytes of the body (default `httpx.DefaultVendorBodyLimit`, 4096; values above `httpx.MaxVendorBodyLimit`, 64 KiB, are capped). The rest is drained and the body closed so the connection is reused. If reading fails, the hook is not called.
- Whatever it returns is trimmed, stripped of control characters, scrubbed of secrets and cut to 64 bytes. No part of the body is stored on any error.
- The hook runs on the request path; a panic in it is not caught.

## Tracing

Set `Config.Trace` to an `io.Writer` to get one line per attempt. Authorization, cookies, API keys and similar headers are redacted, the URL is reduced to scheme, host and path, and bodies are never written. Leave it nil in agent mode.

## Testing

Set `Config.Clock` to a fake (for example [`clock.Fake`](clock.md)) whose `Sleep` returns immediately and `Config.Rand` to a fixed function so tests neither wait nor flake. Use `httptest` servers for the other side.

## Troubleshooting

- Exit 8 right away on a POST: the request was not marked safe to retry.
- Exit 8 with `Retry-After` larger than `MaxWait`: the wait is capped, so the server may still be limiting you; raise `MaxWait` or retry later.
- `forbidden host` error: the host (or a redirect target) is not in `Config.AllowedHosts`, or the URL is plain http; add the host or use https.
- Body not resent: build the request with `http.NewRequest` over a replayable reader, or set `GetBody`.

## Host safety

The client only talks to hosts you allow, so a bearer token cannot be sent somewhere else. Set `Config.AllowedHosts` (for example `[]string{"api.example.com"}`; a bare host matches any port, `host:port` pins the port). If you leave it empty, only the host of the first request is allowed. A request or redirect to any other host fails with `*httpx.ForbiddenHostError` (code `auth/forbidden-host`, exit 4) before any credential is attached, and a cross-host redirect is refused, not followed. If a legitimate redirect crosses hosts, add the target host to `AllowedHosts`.

Plain `http` is refused for non-loopback hosts (`localhost` and loopback IPs are fine). Set `Config.AllowInsecureHTTP` only if you must reach a trusted plain-http endpoint.

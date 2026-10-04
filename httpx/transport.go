package httpx

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/stainedhead/agent-cli-core/internal/redact"
)

type safeKey struct{}

// MarkSafeToRetry returns a copy of req marked as safe to retry even though
// its method is not idempotent, for example because it carries an
// idempotency key the server honors. Without the mark, only idempotent
// methods (GET, HEAD, OPTIONS, TRACE, PUT, DELETE) are retried after a
// transient failure.
func MarkSafeToRetry(req *http.Request) *http.Request {
	return req.WithContext(context.WithValue(req.Context(), safeKey{}, true))
}

// IsMarkedSafe reports whether req was marked by MarkSafeToRetry.
func IsMarkedSafe(req *http.Request) bool {
	v, _ := req.Context().Value(safeKey{}).(bool)
	return v
}

func idempotent(method string) bool {
	switch method {
	case "", http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodTrace, http.MethodPut, http.MethodDelete:
		return true
	}
	return false
}

// Transport is a retrying http.RoundTripper. See Config for the behavior.
// It is safe for concurrent use if the base transport and Refresher are.
type Transport struct {
	base http.RoundTripper
	cfg  Config

	mu      sync.Mutex
	allowed []string // lower-cased hosts (host or host:port); pinned on first use when empty
}

// NewTransport wraps base (http.DefaultTransport when nil) with retry,
// Retry-After handling, jitter, token refresh and optional tracing.
//
// Behavior per request: each attempt is authorized through cfg.Refresher. A
// 429, 502, 503 or 504, or a transport error, is retried when the request is
// idempotent or marked with MarkSafeToRetry, its body can be replayed, and
// the shared budget of cfg.MaxRetries remains; otherwise (or once the budget
// is spent) a *RateLimitedError is returned. A 429 or 503 Retry-After is
// honored up to cfg.MaxWait. A 401 triggers one Refresh and one resend, taken
// from the same budget; a second 401 yields *AuthError. A 403 yields
// *ForbiddenError. A request to a host outside Config.AllowedHosts (by
// default, the first request's host), or plain http to a non-loopback host
// without Config.AllowInsecureHTTP, is refused with *ForbiddenHostError before
// it is authorized or sent. Every other response is returned unchanged.
func NewTransport(base http.RoundTripper, cfg Config) *Transport {
	if base == nil {
		base = http.DefaultTransport
	}
	t := &Transport{base: base, cfg: cfg.withDefaults()}
	for _, h := range cfg.AllowedHosts {
		if h = strings.ToLower(strings.TrimSpace(h)); h != "" {
			t.allowed = append(t.allowed, h)
		}
	}
	return t
}

// NewClient returns an http.Client whose Transport is NewTransport over
// http.DefaultTransport and whose CheckRedirect refuses redirects to hosts
// outside Config.AllowedHosts (see ForbiddenHostError). Failures surface as *url.Error values wrapping the
// typed errors of this package; use errors.As or output.CategoryOf.
func NewClient(cfg Config) *http.Client {
	t := NewTransport(nil, cfg)
	return &http.Client{Transport: t, CheckRedirect: t.checkRedirect}
}

// RoundTrip implements http.RoundTripper.
func (t *Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	ctx := req.Context()
	replayable := req.Body == nil || req.Body == http.NoBody || req.GetBody != nil
	retriable := (idempotent(req.Method) || IsMarkedSafe(req)) && replayable
	refreshed := false
	// held collects the credentials this call attached, so a server that
	// echoes one back (in a vendor code or any header) cannot get it into a
	// trace or an error message.
	var held []string

	if err := t.checkRequest(req); err != nil {
		t.trace(0, req, nil, err, nil)
		return nil, err
	}
	for attempt := 1; ; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		out, err := t.prepare(ctx, req, attempt)
		if err != nil {
			return nil, err
		}
		held = holdCredentials(held, out.Header.Get("Authorization"))
		resp, sendErr := t.base.RoundTrip(out)
		t.trace(attempt, out, resp, sendErr, held)
		canRetry := attempt <= t.cfg.MaxRetries

		if sendErr != nil {
			if ctx.Err() != nil || errors.Is(sendErr, context.Canceled) || errors.Is(sendErr, context.DeadlineExceeded) {
				return nil, sendErr
			}
			if !retriable {
				return nil, t.scrubbedSendError(sendErr, held)
			}
			if !canRetry {
				return nil, t.rateLimitedSend(attempt, sendErr, held)
			}
			if err := t.cfg.Clock.Sleep(ctx, t.wait(t.cfg.backoff(attempt-1))); err != nil {
				return nil, err
			}
			continue
		}

		switch resp.StatusCode {
		case http.StatusUnauthorized:
			drain(resp)
			if t.cfg.Refresher == nil || refreshed || !canRetry || !replayable {
				return nil, &AuthError{}
			}
			refreshed = true
			if err := t.cfg.Refresher.Refresh(ctx); err != nil {
				return nil, &AuthError{Err: err}
			}
			continue
		case http.StatusForbidden:
			err := t.forbidden(resp, held)
			drain(resp)
			return nil, err
		case http.StatusTooManyRequests, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
			wait := t.cfg.backoff(attempt - 1)
			if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusServiceUnavailable {
				if d, ok := parseRetryAfter(resp.Header.Get("Retry-After"), t.cfg.Clock.Now()); ok {
					wait = d
				}
			}
			drain(resp)
			if !retriable || !canRetry {
				return nil, &RateLimitedError{Status: resp.StatusCode, Attempts: attempt}
			}
			if err := t.cfg.Clock.Sleep(ctx, t.wait(wait)); err != nil {
				return nil, err
			}
			continue
		}
		return resp, nil
	}
}

// prepare clones req for one attempt, replays the body and authorizes it.
func (t *Transport) prepare(ctx context.Context, req *http.Request, attempt int) (*http.Request, error) {
	out := req.Clone(ctx)
	if attempt > 1 && req.GetBody != nil {
		body, err := req.GetBody()
		if err != nil {
			return nil, err
		}
		out.Body = body
	}
	if t.cfg.Refresher != nil {
		if err := t.cfg.Refresher.Authorize(ctx, out); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// holdCredentials adds the Authorization header value and its credential part
// (the text after the scheme) to held.
func holdCredentials(held []string, header string) []string {
	if header == "" {
		return held
	}
	held = append(held, header)
	if _, cred, ok := strings.Cut(header, " "); ok && cred != "" {
		held = append(held, strings.TrimSpace(cred))
	}
	return held
}

// scrub applies the configured redactor and then removes every credential this
// call attached.
func (t *Transport) scrub(s string, held []string) string {
	s = t.cfg.Redactor.String(s)
	if len(held) == 0 {
		return s
	}
	return redact.New(held...).String(s)
}

func (t *Transport) wait(d time.Duration) time.Duration {
	if d > t.cfg.MaxWait {
		return t.cfg.MaxWait
	}
	if d < 0 {
		return 0
	}
	return d
}

func (t *Transport) forbidden(resp *http.Response, held []string) error {
	e := &ForbiddenError{}
	if t.cfg.VendorCode != nil {
		code := strings.TrimSpace(t.cfg.VendorCode(resp.Header))
		code = t.scrub(code, held)
		if len(code) > maxVendorCode {
			code = code[:maxVendorCode]
		}
		e.VendorCode = code
	}
	if e.VendorCode == "" && t.cfg.VendorCodeFromBody != nil {
		e.VendorCode = t.bodyVendorCode(resp, held)
	}
	return e
}

// bodyVendorCode offers a bounded prefix of the response body to the
// VendorCodeFromBody hook and cleans the result. It returns "" if the body
// cannot be read.
func (t *Transport) bodyVendorCode(resp *http.Response, held []string) string {
	var prefix []byte
	if resp.Body != nil {
		limit := t.cfg.VendorBodyLimit
		switch {
		case limit <= 0:
			limit = DefaultVendorBodyLimit
		case limit > MaxVendorBodyLimit:
			limit = MaxVendorBodyLimit
		}
		var err error
		prefix, err = io.ReadAll(io.LimitReader(resp.Body, int64(limit)))
		if err != nil {
			return ""
		}
	}
	code := strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, t.cfg.VendorCodeFromBody(resp.StatusCode, prefix))
	code = t.scrub(strings.TrimSpace(code), held)
	if len(code) > maxVendorCode {
		cut := maxVendorCode
		for cut > 0 && !utf8.RuneStart(code[cut]) {
			cut--
		}
		code = code[:cut]
	}
	return code
}

// drain discards and closes a response body so the connection can be reused.
func drain(resp *http.Response) {
	if resp.Body == nil {
		return
	}
	_, _ = io.CopyN(io.Discard, resp.Body, 4096)
	_ = resp.Body.Close()
}

// parseRetryAfter parses a Retry-After value: delay-seconds or an HTTP-date
// (resolved against now). A past date or negative number yields zero.
func parseRetryAfter(v string, now time.Time) (time.Duration, bool) {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0, false
	}
	if secs, err := strconv.ParseInt(v, 10, 64); err == nil {
		if secs < 0 {
			secs = 0
		}
		if secs > int64(24*time.Hour/time.Second) {
			secs = int64(24 * time.Hour / time.Second)
		}
		return time.Duration(secs) * time.Second, true
	}
	if at, err := http.ParseTime(v); err == nil {
		if d := at.Sub(now); d > 0 {
			return d, true
		}
		return 0, true
	}
	return 0, false
}

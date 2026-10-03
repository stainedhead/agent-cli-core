package httpx

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

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
// *ForbiddenError. Every other response is returned unchanged.
func NewTransport(base http.RoundTripper, cfg Config) *Transport {
	if base == nil {
		base = http.DefaultTransport
	}
	return &Transport{base: base, cfg: cfg.withDefaults()}
}

// NewClient returns an http.Client whose Transport is NewTransport over
// http.DefaultTransport. Failures surface as *url.Error values wrapping the
// typed errors of this package; use errors.As or output.CategoryOf.
func NewClient(cfg Config) *http.Client {
	return &http.Client{Transport: NewTransport(nil, cfg)}
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
				return nil, sendErr
			}
			if !canRetry {
				return nil, &RateLimitedError{Attempts: attempt, Err: sendErr}
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
			drain(resp)
			return nil, t.forbidden(resp, held)
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
	return e
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

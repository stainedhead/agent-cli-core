package httpx

import (
	"context"
	"io"
	"math"
	"math/rand/v2"
	"net/http"
	"time"

	"github.com/stainedhead/agent-cli-core/internal/clock"
	"github.com/stainedhead/agent-cli-core/internal/redact"
)

// Default retry parameters, applied by NewTransport and NewClient to zero
// Config fields.
const (
	// DefaultMaxRetries is the number of re-sends after the first attempt.
	DefaultMaxRetries = 3
	// DefaultBaseDelay is the first backoff delay before jitter.
	DefaultBaseDelay = 500 * time.Millisecond
	// DefaultMaxDelay caps the exponential backoff delay.
	DefaultMaxDelay = 30 * time.Second
	// DefaultMaxWait is the ceiling on any single wait, Retry-After included.
	DefaultMaxWait = 60 * time.Second
	// DefaultJitter is the default jitter fraction.
	DefaultJitter = 0.2
)

// Clock is the time source for waits. The library's internal clock (real and
// fake) satisfies it, and so does any type with these two methods.
type Clock interface {
	// Now returns the current time (used to resolve HTTP-date Retry-After).
	Now() time.Time
	// Sleep blocks for d or until ctx is done, returning ctx's error then.
	Sleep(ctx context.Context, d time.Duration) error
}

// Config configures a Transport. The zero value is usable: every zero field
// takes its default.
type Config struct {
	// MaxRetries is how many times a request may be re-sent after the first
	// attempt. It is one budget shared by transient-failure retries and the
	// single 401 refresh retry. Zero means DefaultMaxRetries; a negative
	// value disables retries (and the 401 refresh).
	MaxRetries int
	// BaseDelay is the first backoff delay; it doubles per retry up to
	// MaxDelay. Zero means DefaultBaseDelay.
	BaseDelay time.Duration
	// MaxDelay caps the exponential backoff. Zero means DefaultMaxDelay.
	MaxDelay time.Duration
	// MaxWait is the ceiling on any single wait, including Retry-After.
	// Zero means DefaultMaxWait.
	MaxWait time.Duration
	// Jitter is the fraction (0 to 1) by which backoff delays are randomly
	// varied, plus or minus. Zero means DefaultJitter; a negative value
	// disables jitter.
	Jitter float64
	// Clock supplies Now and Sleep. Nil means the real clock.
	Clock Clock
	// Rand returns a value in [0, 1) for jitter. Nil means a random source.
	Rand func() float64
	// Refresher authorizes each attempt and refreshes the token once on a
	// 401. Nil means requests are sent as given and a 401 is an AuthError.
	Refresher TokenRefresher
	// VendorCode extracts a vendor error code from the headers of a 403
	// response. Nil means no code. The response body is never offered.
	VendorCode func(h http.Header) string
	// Trace, when non-nil, receives one redacted line per attempt. Tracing is
	// off by default. Bodies are never traced.
	Trace io.Writer
	// AllowedHosts lists the hosts (host or host:port; a bare host matches any
	// port) the transport may send to and authorize for. Empty means only the
	// host of the first request through the Transport. Any other host,
	// including a redirect target, is refused with *ForbiddenHostError and no
	// credential is attached.
	AllowedHosts []string
	// AllowInsecureHTTP permits plain http to non-loopback hosts. By default
	// only https, or http to localhost or a loopback IP, is allowed.
	AllowInsecureHTTP bool
	// Redactor scrubs traced values. Nil means a default redactor.
	Redactor *redact.Redactor
}

func (c Config) withDefaults() Config {
	switch {
	case c.MaxRetries == 0:
		c.MaxRetries = DefaultMaxRetries
	case c.MaxRetries < 0:
		c.MaxRetries = 0
	}
	if c.BaseDelay <= 0 {
		c.BaseDelay = DefaultBaseDelay
	}
	if c.MaxDelay <= 0 {
		c.MaxDelay = DefaultMaxDelay
	}
	if c.MaxWait <= 0 {
		c.MaxWait = DefaultMaxWait
	}
	switch {
	case c.Jitter == 0:
		c.Jitter = DefaultJitter
	case c.Jitter < 0:
		c.Jitter = 0
	case c.Jitter > 1:
		c.Jitter = 1
	}
	if c.Clock == nil {
		c.Clock = clock.System{}
	}
	if c.Rand == nil {
		c.Rand = rand.Float64
	}
	if c.Redactor == nil {
		c.Redactor = redact.New()
	}
	return c
}

// backoff returns the jittered exponential delay before retry number n
// (zero-based), capped by MaxDelay before jitter.
func (c Config) backoff(n int) time.Duration {
	d := float64(c.BaseDelay) * math.Pow(2, float64(n))
	if d > float64(c.MaxDelay) || math.IsInf(d, 0) {
		d = float64(c.MaxDelay)
	}
	d *= 1 + c.Jitter*(2*c.Rand()-1)
	return time.Duration(d)
}

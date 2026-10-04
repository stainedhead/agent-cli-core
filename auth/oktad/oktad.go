package oktad

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/stainedhead/agent-cli-core/auth"
	"github.com/stainedhead/agent-cli-core/output"
	"github.com/stainedhead/agent-okta-d/pkg/client"
)

// Option configures New.
type Option func(*config)

type config struct {
	socket  string
	timeout time.Duration
}

// WithSocketPath sets the daemon's unix socket path. It overrides the
// AGENT_OKTA_D_SOCKET environment variable and the platform default. An empty
// path is ignored.
func WithSocketPath(path string) Option { return func(c *config) { c.socket = path } }

// WithTimeout sets the per-request timeout. A value of zero or less is
// ignored and the client's default applies.
func WithTimeout(d time.Duration) Option { return func(c *config) { c.timeout = d } }

// Client is an auth.DaemonClient over the credential daemon. It is safe for
// concurrent use. The zero value is not usable: its methods return an error.
type Client struct{ c *client.Client }

var _ auth.DaemonClient = (*Client)(nil)

// New returns a Client. Without WithSocketPath the socket comes from the
// AGENT_OKTA_D_SOCKET environment variable, then the platform default.
func New(opts ...Option) *Client {
	var cfg config
	for _, o := range opts {
		o(&cfg)
	}
	var co []client.Option
	if cfg.socket != "" {
		co = append(co, client.WithSocketPath(cfg.socket))
	}
	if cfg.timeout > 0 {
		co = append(co, client.WithTimeout(cfg.timeout))
	}
	return &Client{c: client.New(co...)}
}

// SocketPath returns the socket path in use, or "" for a zero Client.
func (c *Client) SocketPath() string {
	if c == nil || c.c == nil {
		return ""
	}
	return c.c.SocketPath()
}

// Close releases idle connections. It is idempotent and the Client stays
// usable.
func (c *Client) Close() {
	if c != nil && c.c != nil {
		c.c.Close()
	}
}

// Fetch returns the current token for provider. Failures are mapped as the
// package documentation describes.
func (c *Client) Fetch(ctx context.Context, provider string) (auth.Token, error) {
	return c.get(ctx, provider, func(cl *client.Client) (client.Credential, error) {
		return cl.Credential(ctx, provider)
	})
}

// Refresh forces the daemon to refresh provider's credential and returns the
// new token. Failures are mapped as for Fetch.
func (c *Client) Refresh(ctx context.Context, provider string) (auth.Token, error) {
	return c.get(ctx, provider, func(cl *client.Client) (client.Credential, error) {
		return cl.Refresh(ctx, provider)
	})
}

func (c *Client) get(ctx context.Context, provider string, call func(*client.Client) (client.Credential, error)) (auth.Token, error) {
	if c == nil || c.c == nil {
		return auth.Token{}, errors.New("oktad: Client not created with New")
	}
	if provider == "" {
		return auth.Token{}, errors.New("oktad: provider name is required")
	}
	cred, err := call(c.c)
	if err != nil {
		return auth.Token{}, c.mapError(ctx, provider, err)
	}
	// The one place the token value is read.
	raw := cred.AccessToken.Reveal()
	if raw == "" {
		return auth.Token{}, errors.New("oktad: daemon returned an empty credential")
	}
	return auth.NewToken(raw), nil
}

// mapError classifies err; see the package documentation for the table. A
// context error wins only when ctx is done and err is itself a context error;
// a daemon verdict that arrived before the caller cancelled keeps its class.
func (c *Client) mapError(ctx context.Context, provider string, err error) error {
	switch {
	case ctx.Err() != nil && (errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)):
		return fmt.Errorf("oktad: %w", ctx.Err())
	case errors.Is(err, client.ErrDaemonUnavailable):
		return &auth.UnreachableError{Socket: c.c.SocketPath(), Err: err}
	case errors.Is(err, client.ErrReauthRequired):
		return fmt.Errorf("oktad: %w (%w)", auth.ErrReauthRequired, err)
	case errors.Is(err, client.ErrRevoked):
		return fmt.Errorf("oktad: %w (%w)", auth.ErrRevoked, err)
	case errors.Is(err, client.ErrDegraded):
		return newTransient(err)
	case errors.Is(err, client.ErrNotConfigured), errors.Is(err, client.ErrUnauthorized):
		return &AccessError{Provider: provider, Err: err}
	}
	if _, ok := client.RetryAfter(err); ok {
		return newTransient(err)
	}
	return fmt.Errorf("oktad: %w", err)
}

func newTransient(err error) *TransientError {
	d, _ := client.RetryAfter(err)
	return &TransientError{Wait: d, Err: err}
}

// TransientError means the daemon cannot serve right now but may later: it is
// degraded, or it gave a retry hint. Its category is rate_limited, exit 8.
type TransientError struct {
	// Wait is the daemon's retry hint; zero when it gave none.
	Wait time.Duration
	// Err is the daemon client's error.
	Err error
}

// Error describes the failure; it never contains a token.
func (e *TransientError) Error() string {
	msg := "oktad: credential daemon cannot serve right now"
	if e.Err != nil {
		msg += ": " + e.Err.Error()
	}
	return msg
}

// RetryAfter returns the wait the daemon asked for, or zero if none.
func (e *TransientError) RetryAfter() time.Duration { return e.Wait }

// Hint names the wait, so the failure envelope carries the retry hint.
func (e *TransientError) Hint() string {
	if e.Wait > 0 {
		return "The credential daemon is temporarily unavailable. Retry after " +
			strconv.FormatInt(int64((e.Wait+time.Second-1)/time.Second), 10) + " seconds."
	}
	return "The credential daemon is temporarily unavailable. Retry later."
}

// Unwrap returns the daemon client's error.
func (e *TransientError) Unwrap() error { return e.Err }

// Category returns output.CategoryRateLimited (exit 8).
func (*TransientError) Category() output.Category { return output.CategoryRateLimited }

// AccessError means the daemon knows no usable provider of that name
// (not configured) or does not allow this caller to use it (unauthorized).
// Both are local setup problems for a human to fix, so the category is auth
// (exit 3), not forbidden (exit 4).
type AccessError struct {
	// Provider is the provider name that was requested.
	Provider string
	// Err is the daemon client's error (client.ErrNotConfigured or
	// client.ErrUnauthorized, matched with errors.Is).
	Err error
}

// Error names the provider and the reason.
func (e *AccessError) Error() string {
	if errors.Is(e.Err, client.ErrUnauthorized) {
		return fmt.Sprintf("provider %q: this caller is not authorized to use the credential daemon", e.Provider)
	}
	return fmt.Sprintf("provider %q: not configured in the credential daemon", e.Provider)
}

// Hint says what to check.
func (e *AccessError) Hint() string {
	if errors.Is(e.Err, client.ErrUnauthorized) {
		return "Ask the daemon's administrator to authorize this agent. No fallback credentials are used."
	}
	return "Check that the provider name is right and that the provider is configured and enabled in the credential daemon."
}

// Unwrap returns the daemon client's error.
func (e *AccessError) Unwrap() error { return e.Err }

// Category returns output.CategoryAuth (exit 3).
func (*AccessError) Category() output.Category { return output.CategoryAuth }

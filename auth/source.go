package auth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
)

// TokenSource yields the current bearer token. A tool may supply its own
// implementation (for example a human-mode login source).
type TokenSource interface {
	// Token returns a currently valid token.
	Token(ctx context.Context) (Token, error)
}

// Refresher is optionally implemented by a TokenSource that can force a
// refresh. Authorizer.Refresh needs it.
type Refresher interface {
	// Refresh forces a refresh and returns the new token.
	Refresh(ctx context.Context) (Token, error)
}

// DaemonClient is the credential daemon as this package sees it. Fetch and
// Refresh return ErrReauthRequired, ErrRevoked or *UnreachableError (possibly
// wrapped) for those conditions. It is the adapter point: the module that
// wraps the daemon's own client implements this interface and nothing else in
// this package changes. See the package documentation.
type DaemonClient interface {
	// Fetch returns the current token for provider.
	Fetch(ctx context.Context, provider string) (Token, error)
	// Refresh forces a refresh for provider and returns the new token.
	Refresh(ctx context.Context, provider string) (Token, error)
}

// Option configures NewDaemonTokenSource.
type Option func(*DaemonTokenSource)

// WithRemediation sets the tool-specific instruction added to the generic
// reauth_required and revoked message, for example the exact enroll command.
func WithRemediation(text string) Option {
	return func(s *DaemonTokenSource) { s.remediation = text }
}

// DaemonTokenSource is a TokenSource and Refresher backed by a DaemonClient
// for one named provider. It does no caching (the daemon owns that) and has no
// fallback: any failure is returned.
type DaemonTokenSource struct {
	client      DaemonClient
	provider    string
	remediation string
}

// NewDaemonTokenSource returns a source for provider. The provider name is
// required: the library hard-codes none.
func NewDaemonTokenSource(c DaemonClient, provider string, opts ...Option) (*DaemonTokenSource, error) {
	if c == nil {
		return nil, errors.New("auth: nil DaemonClient")
	}
	if provider == "" {
		return nil, errors.New("auth: provider name is required")
	}
	s := &DaemonTokenSource{client: c, provider: provider}
	for _, o := range opts {
		o(s)
	}
	return s, nil
}

// Token fetches the current token for the provider.
func (s *DaemonTokenSource) Token(ctx context.Context) (Token, error) {
	t, err := s.client.Fetch(ctx, s.provider)
	return s.result(t, err, "fetch")
}

// Refresh forces a daemon refresh for the provider.
func (s *DaemonTokenSource) Refresh(ctx context.Context) (Token, error) {
	t, err := s.client.Refresh(ctx, s.provider)
	return s.result(t, err, "refresh")
}

func (s *DaemonTokenSource) result(t Token, err error, op string) (Token, error) {
	if err == nil {
		if t.IsZero() {
			return Token{}, &TokenError{Provider: s.provider, Op: op, Err: errors.New("daemon returned an empty token")}
		}
		return t, nil
	}
	var ue *UnreachableError
	switch {
	case errors.As(err, &ue):
		return Token{}, err
	case errors.Is(err, ErrReauthRequired):
		return Token{}, &ActionRequiredError{Provider: s.provider, Err: ErrReauthRequired, Remediation: s.remediation}
	case errors.Is(err, ErrRevoked):
		return Token{}, &ActionRequiredError{Provider: s.provider, Err: ErrRevoked, Remediation: s.remediation}
	}
	return Token{}, &TokenError{Provider: s.provider, Op: op, Err: err}
}

// Authorizer authorizes outgoing requests from a TokenSource and can force a
// refresh. It has exactly the methods of httpx.TokenRefresher without
// importing it.
type Authorizer struct{ src TokenSource }

// NewAuthorizer returns an Authorizer over src. A nil src makes every call
// fail.
func NewAuthorizer(src TokenSource) *Authorizer { return &Authorizer{src: src} }

// Authorize sets the Authorization header on req to the current bearer token.
// The token never leaves this package except into that header. On error the
// header is left untouched.
func (a *Authorizer) Authorize(ctx context.Context, req *http.Request) error {
	if req == nil {
		return errors.New("auth: nil request")
	}
	if a.src == nil {
		return errors.New("auth: no token source")
	}
	t, err := a.src.Token(ctx)
	if err != nil {
		return err
	}
	if t.IsZero() {
		return &TokenError{Op: "fetch", Err: errors.New("token source returned an empty token")}
	}
	req.Header.Set("Authorization", "Bearer "+t.reveal())
	return nil
}

// Refresh forces one token refresh. It returns ErrRefreshUnsupported (wrapped)
// when the source does not implement Refresher.
func (a *Authorizer) Refresh(ctx context.Context) error {
	if a.src == nil {
		return errors.New("auth: no token source")
	}
	r, ok := a.src.(Refresher)
	if !ok {
		return fmt.Errorf("auth: %w", ErrRefreshUnsupported)
	}
	_, err := r.Refresh(ctx)
	return err
}

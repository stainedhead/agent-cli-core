package authtest

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/stainedhead/agent-cli-core/auth"
)

// Scenario selects how a Fake behaves.
type Scenario int

// The scenarios.
const (
	// Valid: Fetch returns a token the fake resource server accepts.
	Valid Scenario = iota
	// ExpiredNeedsRefresh: Fetch returns an expired token the resource
	// server rejects with 401 until Refresh is called.
	ExpiredNeedsRefresh
	// ReauthRequired: Fetch and Refresh return auth.ErrReauthRequired.
	ReauthRequired
	// Revoked: Fetch and Refresh return auth.ErrRevoked.
	Revoked
	// Unreachable: Fetch and Refresh return *auth.UnreachableError.
	Unreachable
	// UnauthorizedThenSuccess: the resource server answers the first
	// request with 401 whatever the token, then accepts the refreshed one.
	UnauthorizedThenSuccess
	// UnauthorizedTwice: the resource server answers the first two requests
	// with 401, so a refresh-and-retry still fails.
	UnauthorizedTwice
)

var scenarioNames = [...]string{
	"valid", "expired-needs-refresh", "reauth-required", "revoked",
	"unreachable", "unauthorized-then-success", "unauthorized-twice",
}

// String names the scenario.
func (s Scenario) String() string {
	if s >= 0 && int(s) < len(scenarioNames) {
		return scenarioNames[s]
	}
	return "unknown"
}

// DefaultSocket is the socket path an Unreachable fake reports unless
// WithSocket overrides it.
const DefaultSocket = "/tmp/fake-credential-daemon.sock"

// Option configures New.
type Option func(*Fake)

// WithSocket sets the socket path named by the Unreachable scenario.
func WithSocket(path string) Option { return func(f *Fake) { f.socket = path } }

// Fake is an auth.DaemonClient with scripted behavior, and its Handler is the
// matching fake resource server. It is safe for concurrent use.
type Fake struct {
	mu         sync.Mutex
	scenario   Scenario
	socket     string
	gen        int // generation of the token Fetch returns
	goodFrom   int // resource server accepts tokens of this generation or later
	rejectLeft int
	fetches    int
	refreshes  int
	requests   int
	providers  []string
}

// New returns a Fake for scenario.
func New(scenario Scenario, opts ...Option) *Fake {
	f := &Fake{scenario: scenario, socket: DefaultSocket, gen: 1, goodFrom: 1}
	switch scenario {
	case ExpiredNeedsRefresh:
		f.gen, f.goodFrom = 0, 1
	case UnauthorizedThenSuccess:
		f.rejectLeft = 1
	case UnauthorizedTwice:
		f.rejectLeft = 2
	}
	for _, o := range opts {
		o(f)
	}
	return f
}

func tokenFor(gen int) auth.Token { return auth.NewToken("fake-token-" + strconv.Itoa(gen)) }

func (f *Fake) failure() error {
	switch f.scenario {
	case ReauthRequired:
		return auth.ErrReauthRequired
	case Revoked:
		return auth.ErrRevoked
	case Unreachable:
		return &auth.UnreachableError{Socket: f.socket, Err: fmt.Errorf("dial unix %s: connect: no such file or directory", f.socket)}
	}
	return nil
}

// Fetch implements auth.DaemonClient.
func (f *Fake) Fetch(_ context.Context, provider string) (auth.Token, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.fetches++
	f.providers = append(f.providers, provider)
	if err := f.failure(); err != nil {
		return auth.Token{}, err
	}
	return tokenFor(f.gen), nil
}

// Refresh implements auth.DaemonClient. It issues a newer token.
func (f *Fake) Refresh(_ context.Context, provider string) (auth.Token, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.refreshes++
	f.providers = append(f.providers, provider)
	if err := f.failure(); err != nil {
		return auth.Token{}, err
	}
	f.gen++
	if f.gen > f.goodFrom {
		f.goodFrom = f.gen
	}
	return tokenFor(f.gen), nil
}

// Fetches counts Fetch calls.
func (f *Fake) Fetches() int { f.mu.Lock(); defer f.mu.Unlock(); return f.fetches }

// Refreshes counts Refresh calls.
func (f *Fake) Refreshes() int { f.mu.Lock(); defer f.mu.Unlock(); return f.refreshes }

// Requests counts requests served by Handler.
func (f *Fake) Requests() int { f.mu.Lock(); defer f.mu.Unlock(); return f.requests }

// Providers lists the provider name of every Fetch and Refresh call, in order.
func (f *Fake) Providers() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.providers...)
}

// Handler is the fake resource server: it answers 200 to a request carrying an
// acceptable bearer token and 401 otherwise, per the scenario.
func (f *Fake) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.requests++
		reject := f.rejectLeft > 0
		if reject {
			f.rejectLeft--
		}
		ok := !reject && f.accepts(r.Header.Get("Authorization"))
		f.mu.Unlock()
		if !ok {
			w.Header().Set("WWW-Authenticate", "Bearer")
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
}

func (f *Fake) accepts(header string) bool {
	rest, found := strings.CutPrefix(header, "Bearer fake-token-")
	if !found {
		return false
	}
	gen, err := strconv.Atoi(rest)
	return err == nil && gen >= f.goodFrom
}

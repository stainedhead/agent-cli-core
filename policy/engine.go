package policy

import (
	"sync"
	"time"
)

// Clock is the time source for rate limits. internal/clock's System and Fake
// satisfy it; policy declares its own so that it imports nothing else.
type Clock interface {
	Now() time.Time
}

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now() }

// Engine evaluates requests against a Policy and enforces its rate limits.
// It is safe for concurrent use; counters are mutex-protected. "Per run"
// means since the Engine was created.
type Engine struct {
	policy *Policy
	clock  Clock

	mu     sync.Mutex
	perRun map[string]int
	hits   map[string][]time.Time // sliding hour window per limit key
}

// NewEngine returns an Engine over p. A nil clock uses real time.
func NewEngine(p *Policy, c Clock) *Engine {
	if c == nil {
		c = systemClock{}
	}
	return &Engine{policy: p, clock: c, perRun: map[string]int{}, hits: map[string][]time.Time{}}
}

// Policy returns the policy the engine enforces.
func (e *Engine) Policy() *Policy { return e.policy }

const globalKey = "\x00global"

// Check decides req like Policy.Evaluate and then applies the global and
// per-rule rate limits. Only requests that would actually run (ModeAllow)
// consume rate budget; a request denied by a limit consumes none and the
// Decision carries RetryAfter when the wait is known. A nil Engine denies.
func (e *Engine) Check(req Request) Decision {
	if e == nil {
		return deny(DefaultDenyID, "no policy engine")
	}
	d, rule := e.policy.evaluate(req)
	if d.Mode != ModeAllow || rule == nil {
		return d
	}
	now := e.clock.Now()
	type limit struct {
		key string
		r   Rate
		who string
	}
	limits := []limit{{globalKey, e.policy.RateLimit, "policy"}, {rule.ID, rule.RateLimit, "rule " + rule.ID}}

	e.mu.Lock()
	defer e.mu.Unlock()
	for _, l := range limits {
		if l.r.PerRun > 0 && e.perRun[l.key] >= l.r.PerRun {
			return rateDenied(rule.ID, l.who+" per-run rate limit reached", 0)
		}
		if l.r.PerHour > 0 {
			w := prune(e.hits[l.key], now)
			e.hits[l.key] = w
			if len(w) >= l.r.PerHour {
				return rateDenied(rule.ID, l.who+" hourly rate limit reached", w[0].Add(time.Hour).Sub(now))
			}
		}
	}
	for _, l := range limits {
		e.perRun[l.key]++
		e.hits[l.key] = append(e.hits[l.key], now)
	}
	return d
}

func rateDenied(id, reason string, retry time.Duration) Decision {
	d := deny(id, reason)
	d.RetryAfter = retry
	return d
}

func prune(w []time.Time, now time.Time) []time.Time {
	cut := now.Add(-time.Hour)
	i := 0
	for i < len(w) && !w[i].After(cut) {
		i++
	}
	return w[i:]
}

// Package clock abstracts time so that retry, rate-limit and expiry logic can
// be tested without real sleeping. It is a leaf package: it imports only the
// standard library.
//
// A consuming CLI can implement Clock itself, use System, or use Fake in
// tests, and pass it to audit.WithClock, httpx.Config.Clock or
// policy.NewEngine (which declare structurally identical interfaces).
package clock

import (
	"context"
	"sort"
	"sync"
	"time"
)

// Clock is the source of time and of delays.
type Clock interface {
	// Now returns the current time.
	Now() time.Time
	// Sleep blocks for d or until ctx is done, in which case it returns the
	// context's error. A non-positive d returns immediately (nil, unless ctx
	// is already done).
	Sleep(ctx context.Context, d time.Duration) error
}

// System is the real clock.
type System struct{}

// Now returns time.Now().
func (System) Now() time.Time { return time.Now() }

// Sleep waits for d or for ctx to be done.
func (System) Sleep(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

type waiter struct {
	deadline time.Time
	ch       chan struct{}
}

// Fake is a deterministic Clock for tests. Time moves only when Advance is
// called, or instantly on every Sleep when auto-advance is on. It is safe for
// concurrent use.
type Fake struct {
	mu      sync.Mutex
	now     time.Time
	waiters []*waiter
	auto    bool
	slept   []time.Duration
	changed chan struct{} // closed and replaced whenever the waiter set changes
}

// NewFake returns a Fake whose current time is start.
func NewFake(start time.Time) *Fake {
	return &Fake{now: start, changed: make(chan struct{})}
}

// Now returns the fake time.
func (f *Fake) Now() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.now
}

// SetAutoAdvance makes every Sleep return immediately after advancing the
// fake time by the requested duration and recording it (see Slept).
func (f *Fake) SetAutoAdvance(on bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.auto = on
}

// Slept returns the durations requested by Sleep while auto-advance was on.
func (f *Fake) Slept() []time.Duration {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]time.Duration(nil), f.slept...)
}

// Sleep blocks until Advance moves the fake time past the deadline, or ctx is
// done.
func (f *Fake) Sleep(ctx context.Context, d time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if d <= 0 {
		return nil
	}
	f.mu.Lock()
	if f.auto {
		f.now = f.now.Add(d)
		f.slept = append(f.slept, d)
		f.mu.Unlock()
		return nil
	}
	w := &waiter{deadline: f.now.Add(d), ch: make(chan struct{})}
	f.waiters = append(f.waiters, w)
	f.notifyLocked()
	f.mu.Unlock()

	select {
	case <-w.ch:
		return nil
	case <-ctx.Done():
		f.mu.Lock()
		f.removeLocked(w)
		f.notifyLocked()
		f.mu.Unlock()
		return ctx.Err()
	}
}

// Advance moves the fake time forward by d and wakes every sleeper whose
// deadline has been reached, in deadline order.
func (f *Fake) Advance(d time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.now = f.now.Add(d)
	sort.SliceStable(f.waiters, func(i, j int) bool {
		return f.waiters[i].deadline.Before(f.waiters[j].deadline)
	})
	keep := f.waiters[:0]
	for _, w := range f.waiters {
		if w.deadline.After(f.now) {
			keep = append(keep, w)
			continue
		}
		close(w.ch)
	}
	f.waiters = keep
	f.notifyLocked()
}

// Waiters returns how many goroutines are currently blocked in Sleep.
func (f *Fake) Waiters() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.waiters)
}

// BlockUntil blocks until at least n goroutines are blocked in Sleep. Tests
// use it to avoid racing Advance against a sleeper that has not started yet.
func (f *Fake) BlockUntil(n int) {
	for {
		f.mu.Lock()
		if len(f.waiters) >= n {
			f.mu.Unlock()
			return
		}
		ch := f.changed
		f.mu.Unlock()
		<-ch
	}
}

func (f *Fake) removeLocked(w *waiter) {
	for i, x := range f.waiters {
		if x == w {
			f.waiters = append(f.waiters[:i], f.waiters[i+1:]...)
			return
		}
	}
}

func (f *Fake) notifyLocked() {
	close(f.changed)
	f.changed = make(chan struct{})
}

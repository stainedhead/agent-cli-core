package clock_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stainedhead/agent-cli-core/internal/clock"
)

var start = time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)

func TestSystemNowIsRecent(t *testing.T) {
	var c clock.Clock = clock.System{}
	if d := time.Since(c.Now()); d < 0 || d > time.Minute {
		t.Fatalf("System.Now off by %v", d)
	}
}

func TestSystemSleep(t *testing.T) {
	c := clock.System{}
	if err := c.Sleep(context.Background(), time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if err := c.Sleep(context.Background(), 0); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := c.Sleep(ctx, time.Hour); err == nil {
		t.Fatal("expected context error")
	}
}

func TestFakeNowAndAdvance(t *testing.T) {
	f := clock.NewFake(start)
	if !f.Now().Equal(start) {
		t.Fatal("start mismatch")
	}
	f.Advance(90 * time.Second)
	if want := start.Add(90 * time.Second); !f.Now().Equal(want) {
		t.Fatalf("got %v want %v", f.Now(), want)
	}
}

func TestFakeSleepWakesOnAdvance(t *testing.T) {
	f := clock.NewFake(start)
	done := make(chan error, 1)
	go func() { done <- f.Sleep(context.Background(), time.Minute) }()
	f.BlockUntil(1)
	f.Advance(30 * time.Second)
	select {
	case <-done:
		t.Fatal("woke too early")
	case <-time.After(20 * time.Millisecond):
	}
	f.Advance(30 * time.Second)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if f.Waiters() != 0 {
		t.Fatal("waiter not removed")
	}
}

func TestFakeSleepContextCancel(t *testing.T) {
	f := clock.NewFake(start)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- f.Sleep(ctx, time.Hour) }()
	f.BlockUntil(1)
	cancel()
	if err := <-done; err == nil {
		t.Fatal("expected error")
	}
	if f.Waiters() != 0 {
		t.Fatal("waiter not removed")
	}
}

func TestFakeSleepNonPositiveReturnsImmediately(t *testing.T) {
	f := clock.NewFake(start)
	if err := f.Sleep(context.Background(), 0); err != nil {
		t.Fatal(err)
	}
	if err := f.Sleep(context.Background(), -time.Second); err != nil {
		t.Fatal(err)
	}
}

func TestFakeAutoAdvance(t *testing.T) {
	f := clock.NewFake(start)
	f.SetAutoAdvance(true)
	for _, d := range []time.Duration{time.Second, 2 * time.Second} {
		if err := f.Sleep(context.Background(), d); err != nil {
			t.Fatal(err)
		}
	}
	if want := start.Add(3 * time.Second); !f.Now().Equal(want) {
		t.Fatalf("got %v want %v", f.Now(), want)
	}
	got := f.Slept()
	if len(got) != 2 || got[0] != time.Second || got[1] != 2*time.Second {
		t.Fatalf("Slept = %v", got)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := f.Sleep(ctx, time.Second); err == nil {
		t.Fatal("expected context error")
	}
}

func TestFakeConcurrent(t *testing.T) {
	f := clock.NewFake(start)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _ = f.Sleep(context.Background(), time.Second) }()
	}
	f.BlockUntil(8)
	f.Advance(time.Second)
	wg.Wait()
}

func ExampleFake() {
	f := clock.NewFake(time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC))
	done := make(chan struct{})
	go func() {
		_ = f.Sleep(context.Background(), time.Minute)
		close(done)
	}()
	f.BlockUntil(1) // the goroutine is now sleeping
	f.Advance(time.Minute)
	<-done
	fmt.Println(f.Now().Format(time.RFC3339))
	// Output: 2026-10-03T12:01:00Z
}

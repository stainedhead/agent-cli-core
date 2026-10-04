package policy_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/stainedhead/agent-cli-core/clock"
	"github.com/stainedhead/agent-cli-core/policy"
)

// The public clock types satisfy policy.Clock.
func TestEngineAcceptsPublicClock(t *testing.T) {
	var c policy.Clock = clock.NewFake(time.Unix(5, 0))
	var s policy.Clock = clock.System{}
	if c.Now().Unix() != 5 || s.Now().IsZero() {
		t.Fatal("unexpected clock values")
	}
}

func ExampleNewEngine_fakeClock() {
	p, _ := policy.Parse([]byte("version: 1\nrules:\n - {id: r, effect: allow, verbs: [read], resources: ['*']}\n"))
	fc := clock.NewFake(time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC))
	e := policy.NewEngine(p, fc) // rate-limit windows follow the fake clock
	fmt.Println(e != nil)
	// Output: true
}

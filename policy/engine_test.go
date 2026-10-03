package policy_test

import (
	"sync"
	"testing"
	"time"

	"github.com/stainedhead/agent-cli-core/internal/clock"
	"github.com/stainedhead/agent-cli-core/policy"
)

var t0 = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

const rated = `
version: 1
rate_limit: {per_hour: 5}
rules:
  - {id: slow, effect: allow, verbs: [get], resources: [slow], rate_limit: {per_hour: 2}}
  - {id: run, effect: allow, verbs: [get], resources: [run], rate_limit: {per_run: 1}}
  - {id: any, effect: allow, verbs: [get], resources: ["*"]}
  - {id: dry, effect: allow, mode: dry_run_only, verbs: [put], resources: ["*"], rate_limit: {per_run: 1}}
`

func TestEngineHourlyLimit(t *testing.T) {
	fc := clock.NewFake(t0)
	e := policy.NewEngine(mustParse(t, rated), fc)
	req := policy.Request{Verb: "get", Resource: "slow"}
	if !e.Check(req).Allowed {
		t.Fatal("first")
	}
	fc.Advance(10 * time.Minute)
	if !e.Check(req).Allowed {
		t.Fatal("second")
	}
	d := e.Check(req)
	if d.Allowed || d.RuleID != "slow" || d.RetryAfter != 50*time.Minute {
		t.Fatalf("third: %+v", d)
	}
	fc.Advance(50 * time.Minute)
	if !e.Check(req).Allowed {
		t.Fatal("window should have slid")
	}
}

func TestEngineGlobalLimitAndNoConsumptionOnDenial(t *testing.T) {
	fc := clock.NewFake(t0)
	e := policy.NewEngine(mustParse(t, rated), fc)
	for i := 0; i < 5; i++ {
		if !e.Check(policy.Request{Verb: "get", Resource: "x"}).Allowed {
			t.Fatalf("call %d", i)
		}
	}
	d := e.Check(policy.Request{Verb: "get", Resource: "x"})
	if d.Allowed || d.RetryAfter != time.Hour {
		t.Fatalf("%+v", d)
	}
	// Denied by default: not rate related, and consumes nothing.
	if e.Check(policy.Request{Verb: "zap", Resource: "x"}).RuleID != policy.DefaultDenyID {
		t.Fatal("default deny expected")
	}
}

func TestEnginePerRun(t *testing.T) {
	e := policy.NewEngine(mustParse(t, rated), clock.NewFake(t0))
	req := policy.Request{Verb: "get", Resource: "run"}
	if !e.Check(req).Allowed {
		t.Fatal("first")
	}
	d := e.Check(req)
	if d.Allowed || d.RuleID != "run" || d.RetryAfter != 0 {
		t.Fatalf("%+v", d)
	}
}

func TestEngineDryRunConsumesNothing(t *testing.T) {
	e := policy.NewEngine(mustParse(t, rated), clock.NewFake(t0))
	for i := 0; i < 3; i++ {
		if d := e.Check(policy.Request{Verb: "put", Resource: "x"}); !d.DryRunOnly() {
			t.Fatalf("%+v", d)
		}
	}
}

func TestEngineGlobalPerRun(t *testing.T) {
	e := policy.NewEngine(mustParse(t, "version: 1\nrate_limit: {per_run: 1}\nrules:\n - {id: a, effect: allow, verbs: ['*'], resources: ['*']}\n"), clock.NewFake(t0))
	if !e.Check(policy.Request{Verb: "x", Resource: "y"}).Allowed || e.Check(policy.Request{Verb: "x", Resource: "y"}).Allowed {
		t.Fatal("per-run global limit")
	}
}

func TestEngineNilAndDefaults(t *testing.T) {
	var e *policy.Engine
	if e.Check(policy.Request{}).Allowed {
		t.Fatal("nil engine must deny")
	}
	p := mustParse(t, good)
	e = policy.NewEngine(p, nil) // real clock
	if e.Policy() != p || !e.Check(policy.Request{Verb: "get", Resource: "x"}).Allowed {
		t.Fatal("real clock engine")
	}
}

func TestEngineConcurrent(t *testing.T) {
	e := policy.NewEngine(mustParse(t, "version: 1\nrate_limit: {per_run: 10}\nrules:\n - {id: a, effect: allow, verbs: ['*'], resources: ['*']}\n"), clock.NewFake(t0))
	var wg sync.WaitGroup
	var mu sync.Mutex
	n := 0
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if e.Check(policy.Request{Verb: "x", Resource: "y"}).Allowed {
				mu.Lock()
				n++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if n != 10 {
		t.Fatalf("allowed %d, want 10", n)
	}
}

package policy

import (
	"testing"
	"time"
)

type stepClock struct{ t time.Time }

func (c *stepClock) Now() time.Time { return c.t }

func mustParseInternal(t *testing.T, src string) *Policy {
	t.Helper()
	p, err := Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestEngineNoHitsWithoutHourlyLimit(t *testing.T) {
	p := mustParseInternal(t, "version: 1\nrules:\n  - {id: any, effect: allow, verbs: [get], resources: [\"*\"], rate_limit: {per_run: 100000}}\n")
	e := NewEngine(p, &stepClock{time.Unix(1e9, 0)})
	for i := 0; i < 1000; i++ {
		if !e.Check(Request{Verb: "get", Resource: "x"}).Allowed {
			t.Fatal("denied")
		}
	}
	if len(e.hits) != 0 {
		t.Fatalf("hits tracked without PerHour limit: %d keys", len(e.hits))
	}
}

func TestEngineHitsBoundedUnderHourlyLimit(t *testing.T) {
	p := mustParseInternal(t, "version: 1\nrate_limit: {per_hour: 3}\nrules:\n  - {id: any, effect: allow, verbs: [get], resources: [\"*\"]}\n")
	c := &stepClock{time.Unix(1e9, 0)}
	e := NewEngine(p, c)
	for i := 0; i < 500; i++ {
		c.t = c.t.Add(time.Minute)
		e.Check(Request{Verb: "get", Resource: "x"})
		if n := len(e.hits[globalKey]); n > 3 {
			t.Fatalf("hit window grew to %d", n)
		}
		if _, ok := e.hits["any"]; ok {
			t.Fatal("rule key tracked without its own PerHour limit")
		}
	}
}

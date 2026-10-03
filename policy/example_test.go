package policy_test

import (
	"fmt"
	"time"

	"github.com/stainedhead/agent-cli-core/internal/clock"
	"github.com/stainedhead/agent-cli-core/output"
	"github.com/stainedhead/agent-cli-core/policy"
)

const exampleYAML = `
version: 1
limits: {max_results: 25, max_bytes: 4096}
rate_limit: {per_hour: 100}
rules:
  - id: read
    effect: allow
    verbs: [get, list]
    resources: ["*"]
  - id: create-record
    effect: allow
    mode: dry_run_only
    verbs: [create]
    resources: [record]
    fields: [title, priority]
    constraints:
      priority: {min: 1, max: 3}
  - id: never-delete
    effect: deny
    verbs: [delete]
    resources: ["*"]
`

func ExampleParse() {
	_, err := policy.Parse([]byte("version: 1\nrules: []\nextra: true\n"))
	fmt.Println(err != nil) // unknown key: fail closed
	p, err := policy.Parse([]byte(exampleYAML))
	fmt.Println(err, len(p.Rules))
	// Output:
	// true
	// <nil> 3
}

func ExamplePolicy_Evaluate() {
	p, _ := policy.Parse([]byte(exampleYAML))
	for _, r := range []policy.Request{
		{Verb: "list", Resource: "record"},
		{Verb: "create", Resource: "record", Fields: map[string]any{"title": "x", "priority": 2}},
		{Verb: "create", Resource: "record", Fields: map[string]any{"owner": "x"}},
		{Verb: "delete", Resource: "record"},
	} {
		d := p.Evaluate(r)
		fmt.Println(r.Verb, d.Allowed, d.Mode, d.RuleID)
	}
	// Output:
	// list true allow read
	// create false dry_run_only create-record
	// create false deny create-record
	// delete false deny never-delete
}

func ExampleDecision_Err() {
	p, _ := policy.Parse([]byte(exampleYAML))
	err := p.Evaluate(policy.Request{Verb: "delete", Resource: "record"}).Err()
	// The tool maps a denial to the policy_denied category (exit code 6).
	fmt.Println(output.ExitFor(output.CategoryPolicyDenied), err != nil)
	// Output: 6 true
}

func ExampleEngine_Check() {
	p, _ := policy.Parse([]byte("version: 1\nrate_limit: {per_hour: 2}\nrules:\n - {id: r, effect: allow, verbs: [get], resources: ['*']}\n"))
	fc := clock.NewFake(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	e := policy.NewEngine(p, fc)
	req := policy.Request{Verb: "get", Resource: "x"}
	fmt.Println(e.Check(req).Allowed, e.Check(req).Allowed)
	d := e.Check(req)
	fmt.Println(d.Allowed, d.RetryAfter)
	fc.Advance(time.Hour)
	fmt.Println(e.Check(req).Allowed)
	// Output:
	// true true
	// false 1h0m0s
	// true
}

func ExampleLimits_ClampResults() {
	l := policy.Limits{MaxResults: 25}
	fmt.Println(l.ClampResults(1000), l.ClampResults(10), l.ClampResults(0))
	// Output: 25 10 25
}

func ExampleLoad() {
	_, err := policy.Load("/nonexistent/policy.yaml")
	fmt.Println(err != nil) // a missing policy fails closed
	// Output: true
}

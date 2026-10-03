package policy_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/stainedhead/agent-cli-core/policy"
)

func mustParse(t *testing.T, s string) *policy.Policy {
	t.Helper()
	p, err := policy.Parse([]byte(s))
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestEvaluate(t *testing.T) {
	p := mustParse(t, good)
	tests := []struct {
		name   string
		req    policy.Request
		allow  bool
		mode   policy.Mode
		rule   string
		reason string
	}{
		{"read allowed", policy.Request{Verb: "get", Resource: "anything"}, true, policy.ModeAllow, "read-all", "server still decides"},
		{"default deny", policy.Request{Verb: "update", Resource: "note1"}, false, policy.ModeDeny, policy.DefaultDenyID, "no rule allows"},
		{"explicit deny", policy.Request{Verb: "delete", Resource: "note1"}, false, policy.ModeDeny, "no-delete", "is denied"},
		{"dry run", policy.Request{Verb: "create", Resource: "notes", Fields: map[string]any{"title": "abc", "priority": 2}}, false, policy.ModeDryRunOnly, "write-notes", "dry run"},
		{"field not allowed", policy.Request{Verb: "create", Resource: "note", Fields: map[string]any{"body": "x"}}, false, policy.ModeDeny, "write-notes", `"body" is not in the allowlist`},
		{"too long", policy.Request{Verb: "create", Resource: "note", Fields: map[string]any{"title": "abcdef"}}, false, policy.ModeDeny, "write-notes", "too long"},
		{"bad pattern", policy.Request{Verb: "create", Resource: "note", Fields: map[string]any{"title": "AB"}}, false, policy.ModeDeny, "write-notes", "pattern"},
		{"below min", policy.Request{Verb: "create", Resource: "note", Fields: map[string]any{"priority": 0}}, false, policy.ModeDeny, "write-notes", "minimum"},
		{"above max", policy.Request{Verb: "create", Resource: "note", Fields: map[string]any{"priority": 4}}, false, policy.ModeDeny, "write-notes", "maximum"},
		{"not number", policy.Request{Verb: "create", Resource: "note", Fields: map[string]any{"priority": "x"}}, false, policy.ModeDeny, "write-notes", "is not a number"},
		{"bad enum", policy.Request{Verb: "create", Resource: "note", Fields: map[string]any{"kind": "z"}}, false, policy.ModeDeny, "write-notes", "allowed values"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := p.Evaluate(tc.req)
			if d.Allowed != tc.allow || d.Mode != tc.mode || d.RuleID != tc.rule || !strings.Contains(d.Reason, tc.reason) {
				t.Errorf("got %+v", d)
			}
			if (d.Err() == nil) != d.Allowed {
				t.Errorf("Err()=%v for Allowed=%v", d.Err(), d.Allowed)
			}
			if d.DryRunOnly() != (tc.mode == policy.ModeDryRunOnly) {
				t.Error("DryRunOnly mismatch")
			}
		})
	}
}

func TestEvaluateNil(t *testing.T) {
	var p *policy.Policy
	d := p.Evaluate(policy.Request{Verb: "get"})
	if d.Allowed || d.RuleID != policy.DefaultDenyID {
		t.Errorf("nil policy must deny: %+v", d)
	}
	zero := &policy.Policy{}
	if zero.Evaluate(policy.Request{}).Allowed {
		t.Error("zero policy must deny")
	}
}

func TestDenyWinsRegardlessOfOrder(t *testing.T) {
	p := mustParse(t, `
version: 1
rules:
  - {id: allow-all, effect: allow, verbs: ["*"], resources: ["*"]}
  - {id: deny-secret, effect: deny, verbs: ["*"], resources: ["secret*"]}
`)
	d := p.Evaluate(policy.Request{Verb: "get", Resource: "secrets"})
	if d.Allowed || d.RuleID != "deny-secret" {
		t.Errorf("got %+v", d)
	}
	if !p.Evaluate(policy.Request{Verb: "get", Resource: "public"}).Allowed {
		t.Error("public should be allowed")
	}
}

func TestSecondAllowRuleCanSatisfy(t *testing.T) {
	p := mustParse(t, `
version: 1
rules:
  - {id: narrow, effect: allow, verbs: [set], resources: [r], fields: [a]}
  - {id: wide, effect: allow, verbs: [set], resources: [r], fields: [a, b]}
`)
	d := p.Evaluate(policy.Request{Verb: "set", Resource: "r", Fields: map[string]any{"b": 1}})
	if !d.Allowed || d.RuleID != "wide" {
		t.Errorf("got %+v", d)
	}
}

func TestAllowModeDenyRule(t *testing.T) {
	p := mustParse(t, `
version: 1
rules:
  - {id: ro, effect: allow, mode: deny, verbs: [put], resources: ["*"]}
`)
	d := p.Evaluate(policy.Request{Verb: "put", Resource: "x"})
	if d.Allowed || d.Mode != policy.ModeDeny || d.RuleID != "ro" {
		t.Errorf("got %+v", d)
	}
}

func TestWildcards(t *testing.T) {
	tests := []struct {
		pat, s string
		want   bool
	}{
		{"get", "get", true}, {"get", "gets", false}, {"*", "", true}, {"a*", "abc", true},
		{"a*", "ba", false}, {"*c", "abc", true}, {"*c", "abd", false}, {"a*c", "abbc", true},
		{"a*c", "ac", true}, {"a*b*c", "axbyc", true}, {"a*b*c", "axcyb", false}, {"a*b", "a", false},
	}
	for _, tc := range tests {
		p := mustParse(t, "version: 1\nrules:\n - {id: r, effect: allow, verbs: ['"+tc.pat+"'], resources: ['*']}\n")
		if got := p.Evaluate(policy.Request{Verb: tc.s, Resource: "x"}).Allowed; got != tc.want {
			t.Errorf("%q vs %q: got %v", tc.pat, tc.s, got)
		}
	}
}

func TestNumericTypes(t *testing.T) {
	p := mustParse(t, `
version: 1
rules:
  - {id: r, effect: allow, verbs: [v], resources: [r], constraints: {n: {min: 1, max: 10}}}
`)
	for _, v := range []any{int(5), int8(5), int16(5), int32(5), int64(5), uint(5), uint8(5), uint16(5), uint32(5), uint64(5), float32(5), 5.0, "5"} {
		if !p.Evaluate(policy.Request{Verb: "v", Resource: "r", Fields: map[string]any{"n": v}}).Allowed {
			t.Errorf("%T should pass", v)
		}
	}
	for _, v := range []any{true, nil, []int{1}, "abc"} {
		if p.Evaluate(policy.Request{Verb: "v", Resource: "r", Fields: map[string]any{"n": v}}).Allowed {
			t.Errorf("%T should fail", v)
		}
	}
}

func TestDeniedError(t *testing.T) {
	d := mustParse(t, good).Evaluate(policy.Request{Verb: "delete", Resource: "x"})
	err := d.Err()
	var de *policy.DeniedError
	if !errors.As(err, &de) || de.Decision.RuleID != "no-delete" {
		t.Fatalf("got %v", err)
	}
	if !strings.Contains(err.Error(), "guardrail, not a security control") {
		t.Errorf("wording: %v", err)
	}
}

package policy_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/stainedhead/agent-cli-core/policy"
)

const good = `
version: 1
limits: {max_results: 50, max_bytes: 1024}
rules:
  - id: read-all
    effect: allow
    verbs: [get, list]
    resources: ["*"]
  - id: write-notes
    effect: allow
    mode: dry_run_only
    verbs: [create]
    resources: [note*]
    fields: [title, priority, kind]
    constraints:
      title: {max_len: 5, pattern: "^[a-z]+$"}
      priority: {min: 1, max: 3}
      kind: {enum: [a, b]}
  - id: no-delete
    effect: deny
    verbs: [delete]
    resources: ["*"]
`

func TestParseInvalid(t *testing.T) {
	tests := []struct{ name, in, want string }{
		{"empty", "", "empty"},
		{"blank", "  \n# c\n", ""},
		{"unknown top key", "version: 1\nbogus: 1\nrules: []\n", ""},
		{"unknown rule key", "version: 1\nrules:\n - id: a\n   effect: allow\n   verbs: [a]\n   resources: [b]\n   oops: 1\n", ""},
		{"duplicate key", "version: 1\nversion: 1\nrules: []\n", ""},
		{"bad yaml", "version: [", ""},
		{"wrong type", "version: x\n", ""},
		{"bad version", "version: 2\nrules:\n - {id: a, effect: allow, verbs: [a], resources: [b]}\n", "version must be"},
		{"no rules", "version: 1\n", "at least one rule"},
		{"neg limits", "version: 1\nlimits: {max_bytes: -1}\nrate_limit: {per_hour: -1}\nrules:\n - {id: a, effect: allow, verbs: [a], resources: [b]}\n", "limits must not"},
		{"missing id", "version: 1\nrules:\n - {effect: allow, verbs: [a], resources: [b]}\n", "id is required"},
		{"dup id", "version: 1\nrules:\n - {id: a, effect: allow, verbs: [a], resources: [b]}\n - {id: a, effect: deny, verbs: [a], resources: [b]}\n", "duplicate id"},
		{"bad effect", "version: 1\nrules:\n - {id: a, effect: maybe, verbs: [a], resources: [b]}\n", "effect must be"},
		{"bad mode", "version: 1\nrules:\n - {id: a, effect: allow, mode: x, verbs: [a], resources: [b]}\n", "unknown mode"},
		{"deny with mode", "version: 1\nrules:\n - {id: a, effect: deny, mode: allow, verbs: [a], resources: [b]}\n", "cannot set mode"},
		{"no verbs", "version: 1\nrules:\n - {id: a, effect: allow, resources: [b]}\n", "verbs must"},
		{"empty resource", "version: 1\nrules:\n - {id: a, effect: allow, verbs: [a], resources: ['']}\n", "resources must"},
		{"empty field", "version: 1\nrules:\n - {id: a, effect: allow, verbs: [a], resources: [b], fields: ['']}\n", "fields must"},
		{"neg rate", "version: 1\nrules:\n - {id: a, effect: allow, verbs: [a], resources: [b], rate_limit: {per_run: -1}}\n", "rate_limit must"},
		{"bad pattern", "version: 1\nrules:\n - {id: a, effect: allow, verbs: [a], resources: [b], constraints: {f: {pattern: '('}}}\n", "bad pattern"},
		{"neg maxlen", "version: 1\nrules:\n - {id: a, effect: allow, verbs: [a], resources: [b], constraints: {f: {max_len: -1}}}\n", "max_len"},
		{"min>max", "version: 1\nrules:\n - {id: a, effect: allow, verbs: [a], resources: [b], constraints: {f: {min: 5, max: 1}}}\n", "min exceeds"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p, err := policy.Parse([]byte(tc.in))
			if err == nil || p != nil {
				t.Fatalf("want error, got p=%v err=%v", p, err)
			}
			var ie *policy.InvalidError
			if !errors.As(err, &ie) {
				t.Fatalf("want *InvalidError, got %T", err)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q lacks %q", err, tc.want)
			}
			if tc.name == "bad yaml" && ie.Unwrap() == nil {
				t.Error("Unwrap should expose parse error")
			}
		})
	}
}

func TestParseGood(t *testing.T) {
	p, err := policy.Parse([]byte(good))
	if err != nil {
		t.Fatal(err)
	}
	if p.Limits.MaxResults != 50 || len(p.Rules) != 3 || p.Rules[0].Mode != policy.ModeAllow || p.Rules[2].Mode != policy.ModeDeny {
		t.Errorf("unexpected policy: %+v", p)
	}
	if p.Warnings() != nil {
		t.Error("no warnings expected")
	}
	var np *policy.Policy
	if np.Warnings() != nil {
		t.Error("nil policy warnings")
	}
}

func TestLimitsClamp(t *testing.T) {
	tests := []struct {
		l          policy.Limits
		in, res, b int
	}{
		{policy.Limits{MaxResults: 10, MaxBytes: 100}, 5, 5, 5},
		{policy.Limits{MaxResults: 10, MaxBytes: 100}, 500, 10, 100},
		{policy.Limits{MaxResults: 10, MaxBytes: 100}, 0, 10, 100},
		{policy.Limits{}, 7, 7, 7},
		{policy.Limits{}, 0, 0, 0},
	}
	for _, tc := range tests {
		if g := tc.l.ClampResults(tc.in); g != tc.res {
			t.Errorf("%+v ClampResults(%d)=%d want %d", tc.l, tc.in, g, tc.res)
		}
		if g := tc.l.ClampBytes(tc.in); g != tc.b {
			t.Errorf("%+v ClampBytes(%d)=%d want %d", tc.l, tc.in, g, tc.b)
		}
	}
}

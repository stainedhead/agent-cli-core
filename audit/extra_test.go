package audit_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stainedhead/agent-cli-core/audit"
	"github.com/stainedhead/agent-cli-core/clock"
)

func logOne(t *testing.T, r audit.Record, opts ...audit.Option) (string, error) {
	t.Helper()
	var buf bytes.Buffer
	l := audit.NewLogger(&buf, append([]audit.Option{audit.WithClock(clock.NewFake(t0))}, opts...)...)
	err := l.Log(r)
	return buf.String(), err
}

func TestNewFieldsOmittedWhenUnset(t *testing.T) {
	line, err := logOne(t, sample())
	if err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"rule_id", "target_ref", "extra"} {
		if strings.Contains(line, k) {
			t.Fatalf("%q present in v0.1.0-shaped record: %s", k, line)
		}
	}
}

func TestExtraRuleTargetGoldenAndRoundTrip(t *testing.T) {
	r := sample()
	r.RuleID = "deny-external"
	r.TargetRef = "snow:INC0012345"
	r.Extra = &audit.ExtraFields{"recipient_count": "2", "alias": "ops", "a.b-c_d": "x"}
	line, err := logOne(t, r)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"schema_version":1,"ts":"2026-10-03T12:00:00Z","tool":"demo","agent_id":"agent-1","run_id":"run-1","verb":"read","resource":"incident/INC001","outcome":"ok","http_status":200,"duration":"1.5s","policy_decision":"allow","rule_id":"deny-external","target_ref":"snow:INC0012345","extra":{"a.b-c_d":"x","alias":"ops","recipient_count":"2"}}` + "\n"
	if line != want {
		t.Fatalf("got  %s\nwant %s", line, want)
	}
	var back audit.Record
	if err := json.Unmarshal([]byte(line), &back); err != nil {
		t.Fatal(err)
	}
	if back.RuleID != r.RuleID || back.TargetRef != r.TargetRef || back.Extra == nil || (*back.Extra)["alias"] != "ops" || len(*back.Extra) != 3 {
		t.Fatalf("round trip lost fields: %+v", back)
	}
}

func TestExtraRedactedAndTruncated(t *testing.T) {
	r := sample()
	r.Extra = &audit.ExtraFields{
		"hdr":  "Authorization: Bearer abcdefghijklmnopqrstuvwxyz012345",
		"long": strings.Repeat("é", 300),
		"nl":   "a\nb",
	}
	r.RuleID = "rule Bearer abcdefghijklmnopqrstuvwxyz012345"
	r.TargetRef = "t Bearer abcdefghijklmnopqrstuvwxyz012345"
	line, err := logOne(t, r, audit.WithSecrets("hunter2"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(line, "abcdefghijklmnopqrstuvwxyz012345") {
		t.Fatalf("secret leaked: %s", line)
	}
	if strings.Count(line, "\n") != 1 {
		t.Fatalf("newline in value split the record: %q", line)
	}
	var back audit.Record
	if err := json.Unmarshal([]byte(line), &back); err != nil {
		t.Fatal(err)
	}
	if got := len((*back.Extra)["long"]); got > 256 || got < 254 {
		t.Fatalf("value not truncated to 256 bytes on a rune boundary: %d", got)
	}
	if (*back.Extra)["nl"] != "a\nb" {
		t.Fatalf("newline should survive JSON escaping: %q", (*back.Extra)["nl"])
	}
	r2 := sample()
	r2.Extra = &audit.ExtraFields{"k": "token hunter2 here"}
	line, _ = logOne(t, r2, audit.WithSecrets("hunter2"))
	if strings.Contains(line, "hunter2") {
		t.Fatalf("registered secret leaked: %s", line)
	}
}

func TestExtraInvalidRejected(t *testing.T) {
	big := audit.ExtraFields{}
	for i := 0; i < 17; i++ {
		big[fmt.Sprintf("k%d", i)] = "v"
	}
	cases := map[string]audit.ExtraFields{
		"empty key":   {"": "v"},
		"upper":       {"Key": "v"},
		"space":       {"a b": "v"},
		"too long":    {strings.Repeat("a", 33): "v"},
		"unicode":     {"é": "v"},
		"too many":    big,
		"control key": {"a\nb": "v"},
	}
	for name, extra := range cases {
		t.Run(name, func(t *testing.T) {
			r := sample()
			r.Extra = &extra
			line, err := logOne(t, r)
			if !errors.Is(err, audit.ErrInvalidExtra) || !errors.Is(err, audit.ErrWrite) {
				t.Fatalf("err = %v", err)
			}
			if line != "" {
				t.Fatalf("record written despite error: %s", line)
			}
		})
	}
}

func TestExtraBoundaryAccepted(t *testing.T) {
	r := sample()
	r.Extra = &audit.ExtraFields{}
	for i := 0; i < 16; i++ {
		(*r.Extra)[fmt.Sprintf("k%02d", i)] = "v"
	}
	(*r.Extra)[strings.Repeat("a", 32)] = "v"
	delete(*r.Extra, "k00")
	if _, err := logOne(t, r); err != nil {
		t.Fatal(err)
	}
	r.Extra = &audit.ExtraFields{}
	if line, err := logOne(t, r); err != nil || strings.Contains(line, "extra") {
		t.Fatalf("empty Extra: %v %s", err, line)
	}
}

func TestExtraNotMutated(t *testing.T) {
	m := audit.ExtraFields{"k": "Bearer abcdefghijklmnopqrstuvwxyz012345"}
	r := sample()
	r.Extra = &m
	if _, err := logOne(t, r); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(m["k"], "abcdefgh") {
		t.Fatal("caller's map was modified")
	}
}

func TestInvalidExtraHandleBlock(t *testing.T) {
	var buf bytes.Buffer
	l := audit.NewLogger(&buf, audit.WithFailureMode(audit.Block))
	r := sample()
	r.Extra = &audit.ExtraFields{"BAD": "v"}
	if err := l.Handle(r, nil); !errors.Is(err, audit.ErrInvalidExtra) {
		t.Fatalf("err = %v", err)
	}
}

func TestWithClockNilKeepsSystemClock(t *testing.T) {
	var buf bytes.Buffer
	l := audit.NewLogger(&buf, audit.WithClock(nil))
	if err := l.Log(sample()); err != nil {
		t.Fatal(err)
	}
	var back audit.Record
	if err := json.Unmarshal(buf.Bytes(), &back); err != nil {
		t.Fatal(err)
	}
	if time.Since(back.Timestamp) > time.Minute {
		t.Fatalf("timestamp %v not from the system clock", back.Timestamp)
	}
}

// customClock is a consumer-defined clock: the public Clock interface is
// implementable outside the module.
type customClock struct{}

func (customClock) Now() time.Time                             { return t0 }
func (customClock) Sleep(context.Context, time.Duration) error { return nil }

func TestWithClockAcceptsConsumerClock(t *testing.T) {
	line, err := logOne(t, sample(), audit.WithClock(customClock{}))
	if err != nil || !strings.Contains(line, "2026-10-03T12:00:00Z") {
		t.Fatalf("%v %s", err, line)
	}
}

func ExampleRecord_extra() {
	var buf bytes.Buffer
	l := audit.NewLogger(&buf, audit.WithClock(clock.NewFake(t0)))
	_ = l.Log(audit.Record{
		Tool: "demo", Verb: "write", Resource: "message", Outcome: "ok", PolicyDecision: "allow",
		RuleID: "allow-internal", TargetRef: "msg:42",
		Extra: &audit.ExtraFields{"recipient_count": "2"},
	})
	fmt.Print(buf.String())
	// Output:
	// {"schema_version":1,"ts":"2026-10-03T12:00:00Z","tool":"demo","agent_id":"","run_id":"","verb":"write","resource":"message","outcome":"ok","policy_decision":"allow","rule_id":"allow-internal","target_ref":"msg:42","extra":{"recipient_count":"2"}}
}

func TestRecordStaysComparable(t *testing.T) {
	a, b := sample(), sample()
	if a != b {
		t.Fatal("v0.1.0 records must remain comparable with ==")
	}
}

func TestMarshalDirectOmitsEmptyExtra(t *testing.T) {
	r := sample()
	r.Extra = &audit.ExtraFields{}
	b, err := json.Marshal(r)
	if err != nil || strings.Contains(string(b), "extra") {
		t.Fatalf("%v %s", err, b)
	}
}

func TestExtraKeyRedactionRejected(t *testing.T) {
	const secret = "s3cretvalue0123456789abcdef"
	cases := map[string]audit.ExtraFields{
		"key equals secret":       {secret: "v"},
		"key contains secret":     {"k." + secret: "v"},
		"short key contains part": {"pre" + secret[:20]: "v"},
	}
	for name, extra := range cases {
		t.Run(name, func(t *testing.T) {
			r := sample()
			r.Extra = &extra
			line, err := logOne(t, r, audit.WithSecrets(secret, secret[:20]))
			if !errors.Is(err, audit.ErrInvalidExtra) || !errors.Is(err, audit.ErrWrite) {
				t.Fatalf("err = %v", err)
			}
			if line != "" {
				t.Fatalf("record written: %s", line)
			}
			for k := range extra {
				if strings.Contains(err.Error(), k) || strings.Contains(err.Error(), secret[:20]) {
					t.Fatalf("error leaks key: %v", err)
				}
			}
		})
	}
}

func TestExtraKeySyntaxErrorOmitsKey(t *testing.T) {
	const bad = "SECRETKEY-not-allowed"
	r := sample()
	r.Extra = &audit.ExtraFields{bad: "v"}
	_, err := logOne(t, r)
	if !errors.Is(err, audit.ErrInvalidExtra) || strings.Contains(err.Error(), bad) {
		t.Fatalf("err = %v", err)
	}
}

func TestExtraKeyUnaffectedByUnrelatedSecrets(t *testing.T) {
	r := sample()
	r.Extra = &audit.ExtraFields{"recipient_count": "2"}
	line, err := logOne(t, r, audit.WithSecrets("unrelated-secret-value"))
	if err != nil || !strings.Contains(line, `"recipient_count":"2"`) {
		t.Fatalf("line %q err %v", line, err)
	}
}

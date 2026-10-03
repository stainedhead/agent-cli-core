package audit_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stainedhead/agent-cli-core/audit"
	"github.com/stainedhead/agent-cli-core/internal/clock"
)

var t0 = time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)

func sample() audit.Record {
	return audit.Record{
		Tool: "demo", AgentID: "agent-1", RunID: "run-1",
		Verb: "read", Resource: "incident/INC001",
		Outcome: "ok", HTTPStatus: 200,
		Duration: 1500 * time.Millisecond, PolicyDecision: "allow",
	}
}

func TestGoldenJSONL(t *testing.T) {
	var buf bytes.Buffer
	fc := clock.NewFake(t0)
	l := audit.NewLogger(&buf, audit.WithClock(fc))
	if err := l.Log(sample()); err != nil {
		t.Fatal(err)
	}
	fc.Advance(time.Second)
	r := sample()
	r.Verb, r.Outcome, r.HTTPStatus, r.PolicyDecision = "write", "denied", 0, "deny"
	if err := l.Log(r); err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(filepath.Join("testdata", "golden.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if buf.String() != string(want) {
		t.Fatalf("golden mismatch:\n%s\nwant:\n%s", buf.String(), want)
	}
}

func TestRecordRoundTrip(t *testing.T) {
	r := sample()
	r.SchemaVersion = audit.SchemaVersion
	r.Timestamp = t0
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	var got audit.Record
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if got != r {
		t.Fatalf("got %+v want %+v", got, r)
	}
	if err := json.Unmarshal([]byte(`{"duration":"bogus"}`), &got); err == nil {
		t.Fatal("expected duration error")
	}
	if err := json.Unmarshal([]byte(`{`), &got); err == nil {
		t.Fatal("expected syntax error")
	}
	if err := json.Unmarshal([]byte(`{"ts":"2026-10-03T12:00:00Z"}`), &got); err != nil || got.Duration != 0 {
		t.Fatalf("missing duration: %v %v", err, got.Duration)
	}
}

func TestNoSecretsInRecord(t *testing.T) {
	var buf bytes.Buffer
	l := audit.NewLogger(&buf, audit.WithSecrets("hunter2xyz"))
	r := sample()
	r.Resource = "x?access_token=abc123&q=1 hunter2xyz"
	r.Tool = "Bearer abc.def"
	r.Outcome = strings.Repeat("A", 5000)
	if err := l.Log(r); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, bad := range []string{"abc123", "hunter2xyz", "abc.def"} {
		if strings.Contains(out, bad) {
			t.Fatalf("leak %q in %s", bad, out)
		}
	}
	if len(out) > 4000 {
		t.Fatalf("fields not capped: %d", len(out))
	}
	var m map[string]any
	if err := json.Unmarshal(buf.Bytes(), &m); err != nil {
		t.Fatal(err)
	}
	for k := range m {
		if strings.Contains(k, "body") || strings.Contains(k, "token") {
			t.Fatalf("forbidden field %q", k)
		}
	}
}

func TestFieldCapKeepsRunes(t *testing.T) {
	var buf bytes.Buffer
	l := audit.NewLogger(&buf)
	r := sample()
	r.Resource = strings.Repeat("é", 2000)
	if err := l.Log(r); err != nil {
		t.Fatal(err)
	}
	var got audit.Record
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(got.Resource, "...") || strings.ContainsRune(got.Resource, '�') {
		t.Fatalf("bad truncation: %q", got.Resource)
	}
}

func TestExplicitTimestampKept(t *testing.T) {
	var buf bytes.Buffer
	l := audit.NewLogger(&buf, audit.WithClock(clock.NewFake(t0)))
	r := sample()
	r.Timestamp = t0.Add(-time.Hour)
	if err := l.Log(r); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "2026-10-03T11:00:00Z") {
		t.Fatal(buf.String())
	}
}

type failWriter struct{ err error }

func (f failWriter) Write([]byte) (int, error) { return 0, f.err }

func TestWriteFailureSurfaced(t *testing.T) {
	boom := errors.New("disk full")
	l := audit.NewLogger(failWriter{boom})
	err := l.Log(sample())
	var we *audit.WriteError
	if !errors.As(err, &we) || !errors.Is(err, boom) || !errors.Is(err, audit.ErrWrite) {
		t.Fatalf("err = %v", err)
	}
	if !strings.Contains(err.Error(), "disk full") {
		t.Fatal(err)
	}
}

func TestHandleModes(t *testing.T) {
	boom := errors.New("disk full")
	actionErr := errors.New("action failed")

	var seen error
	warn := audit.NewLogger(failWriter{boom}, audit.WithOnWriteError(func(e error) { seen = e }))
	if err := warn.Handle(sample(), nil); err != nil {
		t.Fatalf("warn mode must not fail the action: %v", err)
	}
	if !errors.Is(seen, boom) {
		t.Fatalf("OnWriteError not called: %v", seen)
	}
	if err := warn.Handle(sample(), actionErr); err != actionErr {
		t.Fatalf("got %v", err)
	}

	block := audit.NewLogger(failWriter{boom}, audit.WithFailureMode(audit.Block))
	if err := block.Handle(sample(), nil); !errors.Is(err, audit.ErrWrite) {
		t.Fatalf("block mode must fail: %v", err)
	}
	err := block.Handle(sample(), actionErr)
	if !errors.Is(err, actionErr) || !errors.Is(err, audit.ErrWrite) {
		t.Fatalf("joined error expected: %v", err)
	}

	var buf bytes.Buffer
	ok := audit.NewLogger(&buf, audit.WithFailureMode(audit.Block))
	if err := ok.Handle(sample(), actionErr); err != actionErr {
		t.Fatalf("got %v", err)
	}
	if buf.Len() == 0 {
		t.Fatal("record not written")
	}
}

func TestFailureModeText(t *testing.T) {
	var m audit.WriteFailureMode
	if err := m.UnmarshalText([]byte("block")); err != nil || m != audit.Block {
		t.Fatal(m, err)
	}
	if err := m.UnmarshalText([]byte("WARN")); err != nil || m != audit.Warn {
		t.Fatal(m, err)
	}
	if err := m.UnmarshalText([]byte("")); err != nil || m != audit.Warn {
		t.Fatal(m, err)
	}
	if err := m.UnmarshalText([]byte("nope")); err == nil {
		t.Fatal("expected error")
	}
	b, _ := audit.Block.MarshalText()
	if string(b) != "block" || audit.Warn.String() != "warn" || audit.WriteFailureMode(9).String() != "unknown" {
		t.Fatal("string forms")
	}
	if _, err := audit.WriteFailureMode(9).MarshalText(); err == nil {
		t.Fatal("expected error")
	}
}

func TestOpenFromConfig(t *testing.T) {
	if _, err := audit.Open(audit.Config{}); !errors.Is(err, audit.ErrNoPath) {
		t.Fatalf("got %v", err)
	}
	p := filepath.Join(t.TempDir(), "sub", "demo.audit.jsonl")
	l, err := audit.Open(audit.Config{Path: p, OnFailure: audit.Block}, audit.WithClock(clock.NewFake(t0)))
	if err != nil {
		t.Fatal(err)
	}
	if err := l.Log(sample()); err != nil {
		t.Fatal(err)
	}
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	if err := l.Log(sample()); !errors.Is(err, audit.ErrWrite) {
		t.Fatalf("write after close: %v", err)
	}
	// Reopen appends.
	l2, err := audit.Open(audit.Config{Path: p}, audit.WithClock(clock.NewFake(t0)))
	if err != nil {
		t.Fatal(err)
	}
	if err := l2.Log(sample()); err != nil {
		t.Fatal(err)
	}
	_ = l2.Close()
	b, _ := os.ReadFile(p)
	if n := strings.Count(string(b), "\n"); n != 2 {
		t.Fatalf("lines = %d", n)
	}
	if runtime.GOOS != "windows" {
		st, _ := os.Stat(p)
		if st.Mode().Perm() != 0o600 {
			t.Fatalf("perm %v", st.Mode().Perm())
		}
	}
}

func TestOpenTightensPerms(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permissions")
	}
	p := filepath.Join(t.TempDir(), "a.jsonl")
	if err := os.WriteFile(p, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	l, err := audit.Open(audit.Config{Path: p})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = l.Close() }()
	st, _ := os.Stat(p)
	if st.Mode().Perm() != 0o600 {
		t.Fatalf("perm %v", st.Mode().Perm())
	}
}

func TestOpenErrors(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "file")
	if err := os.WriteFile(f, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := audit.Open(audit.Config{Path: filepath.Join(f, "x", "a.jsonl")}); err == nil {
		t.Fatal("expected mkdir error")
	}
	if _, err := audit.Open(audit.Config{Path: dir}); err == nil {
		t.Fatal("expected open error on directory")
	}
}

func TestNewLoggerCloseNoCloser(t *testing.T) {
	l := audit.NewLogger(&bytes.Buffer{})
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestConcurrentWritesAtomicLines(t *testing.T) {
	p := filepath.Join(t.TempDir(), "c.jsonl")
	l, err := audit.Open(audit.Config{Path: p})
	if err != nil {
		t.Fatal(err)
	}
	const n = 50
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r := sample()
			r.RunID = fmt.Sprint(i)
			if err := l.Log(r); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	_ = l.Close()
	b, _ := os.ReadFile(p)
	lines := strings.Split(strings.TrimSuffix(string(b), "\n"), "\n")
	if len(lines) != n {
		t.Fatalf("lines = %d", len(lines))
	}
	for _, ln := range lines {
		var r audit.Record
		if err := json.Unmarshal([]byte(ln), &r); err != nil {
			t.Fatalf("corrupt line %q: %v", ln, err)
		}
	}
}

func Example() {
	var buf bytes.Buffer
	l := audit.NewLogger(&buf, audit.WithClock(clock.NewFake(t0)))
	_ = l.Log(audit.Record{
		Tool: "demo", Verb: "read", Resource: "incident/INC001",
		Outcome: "ok", HTTPStatus: 200, Duration: 250 * time.Millisecond,
		PolicyDecision: "allow",
	})
	fmt.Print(buf.String())
	// Output: {"schema_version":1,"ts":"2026-10-03T12:00:00Z","tool":"demo","agent_id":"","run_id":"","verb":"read","resource":"incident/INC001","outcome":"ok","http_status":200,"duration":"250ms","policy_decision":"allow"}
}

func ExampleLogger_Handle() {
	boom := errors.New("disk full")
	l := audit.NewLogger(failWriter{boom}, audit.WithFailureMode(audit.Block))
	err := l.Handle(audit.Record{Tool: "demo", Verb: "write"}, nil)
	fmt.Println(errors.Is(err, audit.ErrWrite))
	// Output: true
}

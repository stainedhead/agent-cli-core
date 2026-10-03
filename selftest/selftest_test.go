package selftest_test

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stainedhead/agent-cli-core/output"
	"github.com/stainedhead/agent-cli-core/selftest"
)

var update = flag.Bool("update", false, "rewrite golden files")

func matrix() []selftest.Row {
	return []selftest.Row{
		{Name: "read-widget", Verb: "read", Resource: "widget", Expect: selftest.Allow, ReadOnly: true},
		{Name: "delete-widget", Verb: "delete", Resource: "widget", Expect: selftest.Deny},
		{Name: "list-gadget", Verb: "list", Resource: "gadget", Expect: selftest.Deny, ReadOnly: true},
	}
}

// fakeProbe allows read and list, denies everything else; no network.
func fakeProbe(calls *[]string) selftest.Probe {
	return func(_ context.Context, r selftest.Row) (selftest.Outcome, error) {
		if calls != nil {
			*calls = append(*calls, r.Name)
		}
		if r.Verb == "read" || r.Verb == "list" {
			return selftest.Allow, nil
		}
		return selftest.Deny, nil
	}
}

func golden(t *testing.T, name string, got []byte) {
	t.Helper()
	p := filepath.Join("testdata", name)
	if *update {
		if err := os.WriteFile(p, got, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("%s mismatch\n got: %s\nwant: %s", name, got, want)
	}
}

func TestRunPerRow(t *testing.T) {
	res, err := selftest.Runner{Rows: matrix(), Probe: fakeProbe(nil)}.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Passed != 2 || res.Failed != 1 || res.Skipped != 0 || res.OK() {
		t.Fatalf("counts: %+v", res)
	}
	want := []selftest.Status{selftest.StatusPass, selftest.StatusPass, selftest.StatusFail}
	for i, r := range res.Rows {
		if r.Status != want[i] {
			t.Errorf("row %d status %s want %s", i, r.Status, want[i])
		}
	}
	if res.Rows[2].Detail != "expected deny, got allow" || res.Rows[2].Actual != selftest.Allow {
		t.Errorf("detail: %+v", res.Rows[2])
	}
}

func TestReadOnlyMode(t *testing.T) {
	var calls []string
	res, err := selftest.Runner{Rows: matrix(), Probe: fakeProbe(&calls), ReadOnly: true}.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(calls) != 2 || calls[0] != "read-widget" || calls[1] != "list-gadget" {
		t.Errorf("probe called for %v", calls)
	}
	if res.Skipped != 1 || res.Rows[1].Status != selftest.StatusSkip || res.Rows[1].Actual != "" {
		t.Errorf("%+v", res)
	}
}

func TestSkipDoesNotFail(t *testing.T) {
	rows := []selftest.Row{{Name: "w", Verb: "write", Resource: "x", Expect: selftest.Allow}}
	res, _ := selftest.Runner{Rows: rows, Probe: fakeProbe(nil), ReadOnly: true}.Run(context.Background())
	if !res.OK() || res.ExitCode() != output.ExitOK {
		t.Errorf("%+v", res)
	}
}

func TestProbeError(t *testing.T) {
	probe := func(context.Context, selftest.Row) (selftest.Outcome, error) {
		return selftest.Allow, errors.New("boom")
	}
	res, err := selftest.Runner{Rows: matrix()[:1], Probe: probe}.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	r := res.Rows[0]
	if r.Status != selftest.StatusFail || r.Detail != "probe error: boom" || r.Actual != "" {
		t.Errorf("%+v", r)
	}
}

func TestEmptyMatrix(t *testing.T) {
	res, err := selftest.Runner{Probe: fakeProbe(nil)}.Run(context.Background())
	if err != nil || !res.OK() || len(res.Rows) != 0 {
		t.Fatalf("%+v %v", res, err)
	}
	if res.ExitCode() != output.ExitOK {
		t.Error("exit")
	}
}

func TestConfigErrors(t *testing.T) {
	_, err := selftest.Runner{Rows: matrix()}.Run(context.Background())
	if output.ExitOf(err) != output.ExitUsage {
		t.Errorf("nil probe: %v", err)
	}
	bad := []selftest.Row{{Name: "x", Expect: "maybe"}}
	var calls []string
	_, err = selftest.Runner{Rows: append(matrix(), bad...), Probe: fakeProbe(&calls)}.Run(context.Background())
	if output.ExitOf(err) != output.ExitUsage || len(calls) != 0 {
		t.Errorf("bad expect: %v calls=%v", err, calls)
	}
	if err.Error() == "" {
		t.Error("empty message")
	}
}

func TestContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	probe := func(context.Context, selftest.Row) (selftest.Outcome, error) {
		cancel()
		return selftest.Allow, nil
	}
	res, err := selftest.Runner{Rows: matrix(), Probe: probe}.Run(ctx)
	if !errors.Is(err, context.Canceled) || len(res.Rows) != 1 {
		t.Errorf("%v %+v", err, res)
	}
}

func TestWriteGoldenPass(t *testing.T) {
	res, _ := selftest.Runner{Rows: matrix()[:2], Probe: fakeProbe(nil), ReadOnly: true}.Run(context.Background())
	var b bytes.Buffer
	code, err := res.Write(&b, output.Options{})
	if err != nil || code != output.ExitOK {
		t.Fatal(code, err)
	}
	golden(t, "pass.json.golden", b.Bytes())
}

func TestWriteGoldenFail(t *testing.T) {
	res, _ := selftest.Runner{Rows: matrix(), Probe: fakeProbe(nil)}.Run(context.Background())
	var b bytes.Buffer
	code, err := res.Write(&b, output.Options{})
	if err != nil || code != output.ExitGeneral {
		t.Fatal(code, err)
	}
	golden(t, "fail.json.golden", b.Bytes())
	b.Reset()
	if _, err := res.Write(&b, output.Options{Format: output.FormatText}); err != nil {
		t.Fatal(err)
	}
	golden(t, "fail.text.golden", b.Bytes())
}

func TestWriteError(t *testing.T) {
	res, _ := selftest.Runner{Rows: matrix(), Probe: fakeProbe(nil)}.Run(context.Background())
	code, err := res.Write(&bytes.Buffer{}, output.Options{Format: "bogus"})
	if err == nil || code != output.ExitUsage {
		t.Errorf("%v %v", code, err)
	}
}

func Example() {
	rows := []selftest.Row{
		{Name: "read-item", Verb: "read", Resource: "item", Expect: selftest.Allow, ReadOnly: true},
		{Name: "delete-item", Verb: "delete", Resource: "item", Expect: selftest.Deny},
	}
	// The tool's probe talks to its own target; this one is a stub.
	probe := func(_ context.Context, r selftest.Row) (selftest.Outcome, error) {
		if r.Verb == "read" {
			return selftest.Allow, nil
		}
		return selftest.Deny, nil
	}
	res, _ := selftest.Runner{Rows: rows, Probe: probe}.Run(context.Background())
	code, _ := res.Write(os.Stdout, output.Options{})
	fmt.Println("exit", code)
	// Output:
	// {"ok":true,"data":{"rows":[{"name":"read-item","verb":"read","resource":"item","expect":"allow","read_only":true,"status":"pass","actual":"allow"},{"name":"delete-item","verb":"delete","resource":"item","expect":"deny","read_only":false,"status":"pass","actual":"deny"}],"passed":2,"failed":0,"skipped":0},"meta":{"truncated":false,"next_offset":null,"count":2}}
	// exit 0
}

func ExampleRunner_readOnly() {
	rows := []selftest.Row{
		{Name: "list", Verb: "list", Resource: "item", Expect: selftest.Allow, ReadOnly: true},
		{Name: "purge", Verb: "purge", Resource: "item", Expect: selftest.Deny},
	}
	probe := func(context.Context, selftest.Row) (selftest.Outcome, error) { return selftest.Allow, nil }
	res, _ := selftest.Runner{Rows: rows, Probe: probe, ReadOnly: true}.Run(context.Background())
	fmt.Println(res.Passed, res.Failed, res.Skipped, res.ExitCode())
	// Output: 1 0 1 0
}

func ExampleResult_Envelope() {
	rows := []selftest.Row{{Name: "x", Verb: "read", Resource: "r", Expect: selftest.Deny}}
	probe := func(context.Context, selftest.Row) (selftest.Outcome, error) { return selftest.Allow, nil }
	res, _ := selftest.Runner{Rows: rows, Probe: probe}.Run(context.Background())
	env := res.Envelope()
	fmt.Println(env.OK, env.ExitCode(), env.Error.Message)
	// Output: false 1 selftest: 1 of 1 rows failed: x (read r): expected deny, got allow
}

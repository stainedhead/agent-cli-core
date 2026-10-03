package selftest

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/stainedhead/agent-cli-core/output"
)

// Outcome is what a probe observed, or what a row expects.
type Outcome string

// The two outcomes a row can expect and a probe can observe.
const (
	// Allow means the action was permitted.
	Allow Outcome = "allow"
	// Deny means the action was refused.
	Deny Outcome = "deny"
)

// Status is the per-row result.
type Status string

// Per-row statuses.
const (
	// StatusPass means the observed outcome matched Row.Expect.
	StatusPass Status = "pass"
	// StatusFail means the outcome differed or the probe returned an error.
	StatusFail Status = "fail"
	// StatusSkip means the row was not run (read-only mode).
	StatusSkip Status = "skip"
)

// Row is one line of the matrix: do Verb on Resource and expect Expect.
// ReadOnly marks rows that cannot change state; only those run in read-only mode.
type Row struct {
	Name     string  `json:"name"`
	Verb     string  `json:"verb"`
	Resource string  `json:"resource"`
	Expect   Outcome `json:"expect"`
	ReadOnly bool    `json:"read_only"`
}

// Probe exercises a row against the tool's own target and reports what it
// observed. A non-nil error means the probe could not determine an outcome;
// the row then fails with the error text as its detail.
type Probe func(ctx context.Context, row Row) (Outcome, error)

// RowResult is the result of one row.
type RowResult struct {
	Row
	Status Status  `json:"status"`
	Actual Outcome `json:"actual,omitempty"`
	Detail string  `json:"detail,omitempty"`
}

// Result is the outcome of a Run.
type Result struct {
	Rows    []RowResult `json:"rows"`
	Passed  int         `json:"passed"`
	Failed  int         `json:"failed"`
	Skipped int         `json:"skipped"`
}

// Runner executes a matrix. The zero value has no rows and no probe.
type Runner struct {
	Rows  []Row
	Probe Probe
	// ReadOnly skips every row whose ReadOnly field is false.
	ReadOnly bool
}

// usageError is a configuration error; it maps to output.CategoryUsage.
type usageError string

func (e usageError) Error() string             { return string(e) }
func (e usageError) Category() output.Category { return output.CategoryUsage }

// Run executes the matrix in order and returns the per-row results. It returns
// a usage-category error (exit 2) if the probe is nil or a row has an Expect
// other than Allow or Deny, before running anything, and the context error if
// ctx is cancelled mid-run. A failing row is not an error: inspect Result.OK.
func (r Runner) Run(ctx context.Context) (Result, error) {
	if r.Probe == nil {
		return Result{}, usageError("selftest: no probe supplied")
	}
	for _, row := range r.Rows {
		if row.Expect != Allow && row.Expect != Deny {
			return Result{}, usageError(fmt.Sprintf("selftest: row %q has invalid expect %q (want allow or deny)", row.Name, row.Expect))
		}
	}
	res := Result{Rows: make([]RowResult, 0, len(r.Rows))}
	for _, row := range r.Rows {
		if err := ctx.Err(); err != nil {
			return res, err
		}
		rr := RowResult{Row: row}
		switch {
		case r.ReadOnly && !row.ReadOnly:
			rr.Status, rr.Detail = StatusSkip, "not a read-only row"
			res.Skipped++
		default:
			got, err := r.Probe(ctx, row)
			rr.Actual = got
			switch {
			case err != nil:
				rr.Status, rr.Actual, rr.Detail = StatusFail, "", "probe error: "+err.Error()
				res.Failed++
			case got != row.Expect:
				rr.Status = StatusFail
				rr.Detail = fmt.Sprintf("expected %s, got %s", row.Expect, got)
				res.Failed++
			default:
				rr.Status = StatusPass
				res.Passed++
			}
		}
		res.Rows = append(res.Rows, rr)
	}
	return res, nil
}

// OK reports whether no row failed. Skipped rows do not count as failures and
// an empty matrix is OK.
func (r Result) OK() bool { return r.Failed == 0 }

// Envelope converts the result to the standard envelope. A passing result is a
// success envelope whose data is the Result. A failing one is a general-category
// failure whose message names each failing row, because the envelope's failure
// form carries no data.
func (r Result) Envelope() output.Envelope {
	if r.OK() {
		return output.Success(r, &output.Meta{Count: len(r.Rows)})
	}
	var parts []string
	for _, row := range r.Rows {
		if row.Status == StatusFail {
			parts = append(parts, fmt.Sprintf("%s (%s %s): %s", row.Name, row.Verb, row.Resource, row.Detail))
		}
	}
	msg := fmt.Sprintf("selftest: %d of %d rows failed: %s", r.Failed, len(r.Rows), strings.Join(parts, "; "))
	return output.Failure(output.CategoryGeneral, msg, "fix the failing rows or the target's configuration, then rerun the selftest command")
}

// ExitCode is output.ExitOK when no row failed and output.ExitGeneral (1) otherwise.
func (r Result) ExitCode() output.ExitCode { return r.Envelope().ExitCode() }

// Write renders the result with output.Write and returns the exit code the
// process should use.
func (r Result) Write(w io.Writer, opts output.Options) (output.ExitCode, error) {
	env := r.Envelope()
	if err := output.Write(w, env, opts); err != nil {
		return output.ExitOf(err), err
	}
	return env.ExitCode(), nil
}

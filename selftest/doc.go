// Package selftest runs a tool-supplied matrix of probes (verb, resource,
// expected outcome) and reports per-row pass, fail or skip in the standard
// output envelope. It knows nothing about any server: the tool supplies the
// rows and the Probe function that exercises them.
//
// A Runner executes each Row through the Probe and compares the observed
// Outcome with Row.Expect. With Runner.ReadOnly set, rows not marked ReadOnly
// are skipped, so a matrix that can mutate state is never run by accident.
// Result.Write renders the outcome with the output package and returns exit
// code 1 (output.ExitGeneral) when any row failed.
//
// The runner executes only when the tool calls Runner.Run, normally from an
// explicit "selftest" command. It never runs at startup, on import or on a
// schedule, and it performs no network access of its own.
package selftest

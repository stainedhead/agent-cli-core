# selftest

A runner for a self-test matrix of probes that your CLI supplies. The runner knows nothing about the server you test: you give it rows (verb, resource, expected outcome) and a probe function; it reports pass, fail or skip per row in the standard output envelope.

## Getting started

```go
rows := []selftest.Row{
    {Name: "read-item", Verb: "read", Resource: "item", Expect: selftest.Allow, ReadOnly: true},
    {Name: "delete-item", Verb: "delete", Resource: "item", Expect: selftest.Deny},
}
probe := func(ctx context.Context, r selftest.Row) (selftest.Outcome, error) {
    // Call your own target here and report selftest.Allow or selftest.Deny.
}
res, err := selftest.Runner{Rows: rows, Probe: probe}.Run(ctx)
if err != nil { /* configuration error: output.ExitOf(err) is 2 */ }
code, _ := res.Write(os.Stdout, output.Options{})
os.Exit(int(code))
```

## Behavior

- Each row passes when the probe's outcome equals `Expect`. A probe error fails the row (detail `probe error: ...`).
- `Runner.ReadOnly = true` runs only rows marked `ReadOnly` and skips the rest. Skipped rows never fail the run. Mark a row `ReadOnly` only if it cannot change state; wire the mode to a flag such as `--read-only`.
- A passing matrix (including an empty one) writes a success envelope whose data holds `rows`, `passed`, `failed`, `skipped`, and exits 0.
- Any failing row writes a failure envelope (`ok:false`, code `general`) whose message names each failing row, and `Write` returns exit code 1.
- A nil probe or a row whose `Expect` is not `allow` or `deny` makes `Run` return a usage error (exit 2) before any probe runs.

## On demand only

The runner executes only when your code calls `Run`, normally from an explicit `selftest` command. It never runs on import, at startup or on a schedule. Do not call it from other commands, and default to read-only mode if your matrix can mutate state.

## Troubleshooting

- Exit 1 with `expected deny, got allow`: the target permitted something the matrix says should be refused; check the target's policy.
- Exit 2: fix the matrix (nil probe or a misspelled `Expect`).
- Probe errors count as failures, so an unreachable target fails every row it touches.

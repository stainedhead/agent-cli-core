# audit

A JSON Lines audit log of what your CLI did: one record per action, never a secret and never a request or response body.

## Getting started

```go
l, err := audit.Open(audit.Config{Path: cfg.AuditPath, OnFailure: audit.Block})
if err != nil { /* configuration error */ }
defer l.Close()

start := time.Now()
actionErr := doTheThing()
err = l.Handle(audit.Record{
    Tool: "mytool", AgentID: agentID, RunID: runID,
    Verb: "read", Resource: "incident/INC001",
    Outcome: "ok", HTTPStatus: 200,
    Duration: time.Since(start), PolicyDecision: "allow",
}, actionErr)
```

`Handle` returns the error your command should act on. `Log` writes a record and returns the write error, if any.

## Record fields

`schema_version, ts, tool, agent_id, run_id, verb, resource, outcome, http_status, duration, policy_decision`, and the optional v0.2.0 fields `rule_id, target_ref, extra` (see below). `ts` is UTC and filled from the clock when you leave it zero. `duration` is a Go duration string such as `1.5s`. There is no field for a body or a credential. Every text field is passed through the redaction pass and capped at 512 bytes. Register known secrets with `audit.WithSecrets(...)`.

## Optional fields (v0.2.0)

| Field | Go field | Meaning |
|---|---|---|
| `rule_id` | `RuleID` | The policy rule that decided, for example `Decision.RuleID` |
| `target_ref` | `TargetRef` | A reference to the object acted on |
| `extra` | `Extra *audit.ExtraFields` | Small string map for tool-specific facts |

All three are left out of the line when empty, the schema version stays `1`, and a record that does not use them is byte-identical to v0.1.0. Readers should ignore keys they do not know. `RuleID` and `TargetRef` are redacted and capped at 512 bytes like the other text fields.

`Extra` is a pointer to a named map type so that `audit.Record` stays comparable with `==`: write `Extra: &audit.ExtraFields{"batch": "7"}`. Limits:

- at most `audit.MaxExtraKeys` (16) keys;
- each key matches `[a-z0-9_.-]{1,32}`;
- each value is redacted and cut to `audit.MaxExtraValueLen` (256) bytes on a character boundary;
- a record that breaks the key rules is not written, and `Log` returns a `*audit.WriteError` that matches both `audit.ErrWrite` and `audit.ErrInvalidExtra` (so `block` mode fails the command);
- your map is never changed, and keys are written in sorted order.

## Configuration

| Setting | Meaning |
|---|---|
| `path` | The log file. Required; the library has no default. Created with mode 0600 (an existing file is tightened to 0600), parent directory 0700. |
| `on_failure` | `warn` (default) or `block`. Both parse from text, so they work in JSON or YAML config. |

## When the write fails

- `warn`: `Handle` calls the `WithOnWriteError` hook (print it on stderr) and returns your action's own result unchanged.
- `block`: `Handle` returns the write error (matches `audit.ErrWrite`, joined with your action's error), so the command fails.

Choose `block` for write operations that must not go unrecorded. If the command already succeeded, the result is not hidden: the write error is surfaced alongside it.

## Notes

The `Logger` is safe for concurrent use; each record is one atomic line. Inject a [clock](clock.md) with `WithClock` in tests (a nil clock keeps the system clock).

## Troubleshooting

- `audit: no log path configured`: set `path`.
- `audit: invalid extra field` (`audit.ErrInvalidExtra`): too many keys or a key outside `[a-z0-9_.-]{1,32}`.
- `audit: open log`: check the directory is writable by the user running the CLI.

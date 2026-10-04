# policy

Client-side guardrail policy loaded from strict YAML: allow/deny rules per verb and resource, field allowlists, value constraints, write modes, rate limits and output caps.

**Read this first.** A policy is a guardrail, not a security control. Passing it does not mean the server will allow the action, and failing it only stops your tool from sending the request. Server-side permissions remain the boundary. Keep the policy file where the agent user cannot edit it (for example a root-owned directory the agent can read but not write).

## Getting started

```go
p, err := policy.Load("/etc/mytool/policy.yaml", policy.WithWritable(policy.WritableRefuse))
if err != nil {
    // Invalid or unreadable policy: fail closed, do not run.
}
e := policy.NewEngine(p, nil) // nil clock means real time

d := e.Check(policy.Request{Verb: "create", Resource: "record",
    Fields: map[string]any{"title": "x", "priority": 2}})
switch {
case d.Allowed:
    // perform the action
case d.DryRunOnly():
    // preview only; never perform the write
default:
    // denied: map d.Err() to output.CategoryPolicyDenied (exit 6)
    // and put d.RuleID and d.Reason in the audit record
}
```

Verb and resource are opaque strings your tool chooses (for example `list` and `record`). Call `Check` before acquiring a token.

## Policy file

```yaml
version: 1                 # required, must be 1
limits:                    # optional output caps; 0 or absent means none
  max_results: 25
  max_bytes: 65536
rate_limit:                # optional, applies to every allowed request
  per_hour: 100            # sliding one-hour window
  per_run: 500             # since the Engine was created
rules:
  - id: read               # required, unique; appears in decisions and audit
    effect: allow          # allow | deny
    verbs: [get, list]     # patterns; * matches any run of characters
    resources: ["*"]
  - id: create-record
    effect: allow
    mode: dry_run_only     # allow (default) | dry_run_only | deny
    verbs: [create]
    resources: [record]
    fields: [title, priority]        # allowlist of request field names
    constraints:                     # per-field value checks
      title: {max_len: 80, pattern: "^[A-Za-z0-9 ]+$"}
      priority: {min: 1, max: 3}     # numeric only
      kind: {enum: [a, b]}
    rate_limit: {per_hour: 10}       # for this rule only
  - id: never-delete
    effect: deny
    verbs: [delete]
    resources: ["*"]
```

Evaluation: a matching deny rule always wins; otherwise the first allow rule whose field allowlist and constraints the request satisfies decides; if only allow rules that the request violates match, the first violation is the reason; if nothing matches, the request is denied (rule id `default-deny`). Omitting `fields` means no field allowlist for that rule.

Use `p.Limits.ClampResults(n)` and `ClampBytes(n)` to feed the caps into your output bounds.

## Rate-limit bookkeeping

The engine only keeps a timestamp window for rules that set `per_hour`. A policy with only `per_run` limits (or none) uses a fixed amount of memory however long the process runs; `per_hour` windows are pruned as hits age out.

## Failing closed

Unknown keys, duplicate keys, an empty file, a wrong `version`, duplicate rule ids, an invalid regular expression, a negative limit or a deny rule with a mode all make `Parse` and `Load` return `*policy.InvalidError`. Treat that as fatal and do not run. Report it with the `validation` category.

## Writable policy file (POSIX only)

`Load` checks whether the current user can write the policy file or its directory. The default adds a warning to `p.Warnings()`; `WithWritable(policy.WritableRefuse)` returns a `*policy.WritableError` instead; `WritableIgnore` skips the check. The check does nothing on non-POSIX platforms.

## Trusted file check (v0.2.0)

`policy.CheckTrustedFile(path, opts...)` checks that a file (a policy, or any config you refuse to trust unless protected) can only be changed by trusted users. It is separate from `Load`, which is unchanged.

```go
err := policy.CheckTrustedFile("/etc/mytool/policy.yaml", policy.WithTrustedUIDs(1001))
```

- Trusted owners are root and any uid given to `WithTrustedUIDs`. The effective uid of the running process is not trusted unless it is root: passing it returns an error, because the agent's own user could rewrite its own file.
- Every directory on the way, including `/`, must be owned by a trusted uid and must not be group or world writable. Symlinks are followed by the check itself and each link must also be owned by a trusted uid (at most 40 hops). A sticky world-writable directory such as `/tmp` is rejected.
- The file itself must be a regular file, owned by a trusted uid, and not group or world writable. It is opened without following links and re-checked on the open descriptor, so a swap between check and use fails.
- It fails closed: a missing file, a directory you cannot read, or a platform without this check (anything but Unix) returns an error.
- The error is a `*policy.TrustError{Path, Reason, Err}` that matches `errors.Is(err, policy.ErrNotTrusted)`, carries a hint, and maps to category `policy_denied` (exit 6).

## Testing your tool

`NewEngine` takes any value with a `Now() time.Time` method, so tests can pass a [`clock.Fake`](clock.md) and step time without sleeping.

## Troubleshooting

- "no rule allows ...": add an allow rule; unmatched requests are denied by design.
- "field ... is not in the allowlist": add the field to `fields` or stop sending it.
- `is not a trusted file: ...` (`policy.TrustError`): the reason says what failed; fix the owner or mode of the file or of a directory above it, or move the file to a root-owned directory.
- Rate-limit denial: `Decision.RetryAfter` says how long until the hourly window admits the request (zero for a per-run limit).

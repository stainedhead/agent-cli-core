// Package policy evaluates a client-side guardrail policy loaded from strict
// YAML. A decision is returned as data (allowed, mode, rule id, reason); the
// caller maps a denial to output exit code 6 and records it in the audit log.
//
// A policy is a guardrail, not a security control: passing it does not mean
// the server will allow the action. Server-side permissions remain the
// boundary.
//
// # Model
//
// A policy file holds an ordered list of rules. Each rule has an id, an
// effect (allow or deny), the verbs and resources it covers (opaque strings
// supplied by the tool; "*" matches any run of characters), and for allow
// rules a write mode (allow, dry_run_only or deny), an optional field
// allowlist, optional value constraints and optional rate limits. Anything no
// rule allows is denied.
//
// Parse and Load are strict: unknown keys, duplicate keys, an empty file or
// any invalid value return an error, and the tool must not run (fail
// closed). Policy.Evaluate is a pure function of a Request; Engine adds the
// stateful rate limits, using an injected clock so tests never sleep.
//
// Load can also warn about, or refuse, a policy file that the current user
// can write (POSIX only), because an agent that can edit its own policy has
// no guardrail.
package policy

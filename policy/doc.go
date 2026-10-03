// Package policy evaluates a client-side guardrail policy loaded from strict
// YAML. A decision is returned as data (allowed, mode, rule id, reason); the
// caller maps a denial to output exit code 6 and records it in the audit log.
//
// A policy is a guardrail, not a security control: passing it does not mean
// the server will allow the action.
//
// This package is a stub until its workstream lands.
package policy

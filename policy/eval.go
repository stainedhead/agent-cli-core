package policy

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// DefaultDenyID is the rule id reported when no rule matches a request.
const DefaultDenyID = "default-deny"

// Request is what the tool asks the policy about. Verb and Resource are
// opaque strings the tool chooses; Fields are the names and values the
// request would send.
type Request struct {
	Verb     string
	Resource string
	Fields   map[string]any
}

// Decision is the outcome of evaluating a Request. It is plain data: the
// tool maps a denial to output exit code 6 and records RuleID and Reason in
// the audit log as the policy decision.
//
// A decision is a client-side guardrail result. Even an allowed decision does
// not mean the server will allow the action.
type Decision struct {
	// Allowed is true only when Mode is ModeAllow.
	Allowed bool
	// Mode is ModeAllow, ModeDryRunOnly or ModeDeny. With ModeDryRunOnly the
	// tool may preview the action but must not perform it, and Allowed is
	// false so that a caller checking only Allowed fails safe.
	Mode Mode
	// RuleID is the id of the deciding rule, or DefaultDenyID.
	RuleID string
	// Reason is a short human-readable explanation.
	Reason string
	// RetryAfter is set when a rate limit denied the request and says how
	// long until the limit would admit it.
	RetryAfter time.Duration
}

// DryRunOnly reports whether the tool may only preview the action.
func (d Decision) DryRunOnly() bool { return d.Mode == ModeDryRunOnly }

// Err returns nil when the action may run (Allowed) and a *DeniedError
// otherwise, including for dry-run-only decisions: a tool that supports
// previews should test DryRunOnly first.
func (d Decision) Err() error {
	if d.Allowed {
		return nil
	}
	return &DeniedError{Decision: d}
}

// DeniedError wraps a refusing Decision. The caller maps it to output exit
// code 6 (category policy_denied).
type DeniedError struct{ Decision Decision }

// Error implements error. The text states that the policy is a guardrail.
func (e *DeniedError) Error() string {
	return fmt.Sprintf("denied by client-side policy rule %q: %s (policy is a guardrail, not a security control)",
		e.Decision.RuleID, e.Decision.Reason)
}

// Evaluate decides req against the rules, without rate limits (see Engine).
// A deny rule that matches the verb and resource always wins; otherwise the
// first allow rule whose field allowlist and constraints the request
// satisfies decides; if there is none, the first matching allow rule's
// violation is reported; if no rule matches the verb and resource, the
// request is denied by default. A nil Policy denies everything.
func (p *Policy) Evaluate(req Request) Decision {
	d, _ := p.evaluate(req)
	return d
}

// evaluate also returns the deciding rule (nil for default deny).
func (p *Policy) evaluate(req Request) (Decision, *Rule) {
	if p == nil {
		return deny(DefaultDenyID, "no policy loaded"), nil
	}
	var firstAllow *Rule
	var firstViolation string
	for i := range p.Rules {
		r := &p.Rules[i]
		if !wildAny(r.Verbs, req.Verb) || !wildAny(r.Resources, req.Resource) {
			continue
		}
		if r.Effect == EffectDeny {
			return deny(r.ID, fmt.Sprintf("%s %s is denied", req.Verb, req.Resource)), r
		}
		if violation := r.check(req); violation != "" {
			if firstAllow == nil {
				firstAllow, firstViolation = r, violation
			}
			continue
		}
		// An allow rule satisfied; a later deny rule may still match.
		for j := i + 1; j < len(p.Rules); j++ {
			q := &p.Rules[j]
			if q.Effect == EffectDeny && wildAny(q.Verbs, req.Verb) && wildAny(q.Resources, req.Resource) {
				return deny(q.ID, fmt.Sprintf("%s %s is denied", req.Verb, req.Resource)), q
			}
		}
		return decide(r), r
	}
	if firstAllow != nil {
		return deny(firstAllow.ID, firstViolation), firstAllow
	}
	return deny(DefaultDenyID, fmt.Sprintf("no rule allows %s %s", req.Verb, req.Resource)), nil
}

func decide(r *Rule) Decision {
	switch r.Mode {
	case ModeDryRunOnly:
		return Decision{Mode: ModeDryRunOnly, RuleID: r.ID, Reason: "allowed for dry run only"}
	case ModeDeny:
		return deny(r.ID, "rule sets write mode deny")
	}
	return Decision{Allowed: true, Mode: ModeAllow, RuleID: r.ID, Reason: "allowed by client-side policy (the server still decides)"}
}

func deny(id, reason string) Decision {
	return Decision{Mode: ModeDeny, RuleID: id, Reason: reason}
}

// check returns "" when req satisfies the rule's field allowlist and
// constraints, else the first violation (fields are visited in sorted order
// so the reason is deterministic).
func (r *Rule) check(req Request) string {
	names := make([]string, 0, len(req.Fields))
	for n := range req.Fields {
		names = append(names, n)
	}
	sort.Strings(names)
	if len(r.Fields) > 0 {
		for _, n := range names {
			if !contains(r.Fields, n) {
				return fmt.Sprintf("field %q is not in the allowlist", n)
			}
		}
	}
	for _, n := range names {
		c, ok := r.Constraints[n]
		if !ok {
			continue
		}
		if v := c.violation(req.Fields[n]); v != "" {
			return fmt.Sprintf("field %q %s", n, v)
		}
	}
	return ""
}

func (c Constraint) violation(v any) string {
	s := fmt.Sprint(v)
	if len(c.Enum) > 0 && !contains(c.Enum, s) {
		return "is not one of the allowed values"
	}
	if c.re != nil && !c.re.MatchString(s) {
		return "does not match the required pattern"
	}
	if c.MaxLen > 0 && utf8.RuneCountInString(s) > c.MaxLen {
		return "is too long"
	}
	if c.Min != nil || c.Max != nil {
		f, ok := number(v)
		if !ok {
			return "is not a number"
		}
		if c.Min != nil && f < *c.Min {
			return "is below the minimum"
		}
		if c.Max != nil && f > *c.Max {
			return "is above the maximum"
		}
	}
	return ""
}

func number(v any) (float64, bool) {
	switch n := v.(type) {
	case int:
		return float64(n), true
	case int8:
		return float64(n), true
	case int16:
		return float64(n), true
	case int32:
		return float64(n), true
	case int64:
		return float64(n), true
	case uint:
		return float64(n), true
	case uint8:
		return float64(n), true
	case uint16:
		return float64(n), true
	case uint32:
		return float64(n), true
	case uint64:
		return float64(n), true
	case float32:
		return float64(n), true
	case float64:
		return n, true
	case string:
		f, err := strconv.ParseFloat(n, 64)
		return f, err == nil
	}
	return 0, false
}

func contains(ss []string, s string) bool {
	for _, x := range ss {
		if x == s {
			return true
		}
	}
	return false
}

func wildAny(patterns []string, s string) bool {
	for _, p := range patterns {
		if wild(p, s) {
			return true
		}
	}
	return false
}

// wild matches s against pattern where "*" matches any run of characters.
func wild(pattern, s string) bool {
	parts := strings.Split(pattern, "*")
	if len(parts) == 1 {
		return pattern == s
	}
	if !strings.HasPrefix(s, parts[0]) {
		return false
	}
	s = s[len(parts[0]):]
	last := parts[len(parts)-1]
	for _, mid := range parts[1 : len(parts)-1] {
		i := strings.Index(s, mid)
		if i < 0 {
			return false
		}
		s = s[i+len(mid):]
	}
	return strings.HasSuffix(s, last)
}

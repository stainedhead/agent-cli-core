package policy

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"

	"github.com/goccy/go-yaml"
)

// Version is the only policy schema version this package understands.
const Version = 1

// Effect says whether a matching rule allows or denies a request.
type Effect string

// The rule effects.
const (
	EffectAllow Effect = "allow"
	EffectDeny  Effect = "deny"
)

// Mode is the write mode of a decision.
type Mode string

// The write modes. ModeAllow lets the action run, ModeDryRunOnly lets the
// tool only preview it, ModeDeny forbids it.
const (
	ModeAllow      Mode = "allow"
	ModeDryRunOnly Mode = "dry_run_only"
	ModeDeny       Mode = "deny"
)

// Rate is a pair of request-count limits. A zero field means no limit.
type Rate struct {
	// PerHour is the maximum number of allowed requests in any sliding hour.
	PerHour int `yaml:"per_hour"`
	// PerRun is the maximum number of allowed requests since the Engine was
	// created.
	PerRun int `yaml:"per_run"`
}

// Limits are the output caps a policy imposes. A zero field means no cap.
type Limits struct {
	// MaxResults caps the number of result items a tool may return.
	MaxResults int `yaml:"max_results"`
	// MaxBytes caps the size in bytes of the data a tool may return.
	MaxBytes int `yaml:"max_bytes"`
}

// ClampResults returns n limited to MaxResults. A non-positive n means "no
// preference" and yields MaxResults (or n unchanged when there is no cap).
func (l Limits) ClampResults(n int) int { return clamp(n, l.MaxResults) }

// ClampBytes returns n limited to MaxBytes, with the same convention as
// ClampResults.
func (l Limits) ClampBytes(n int) int { return clamp(n, l.MaxBytes) }

func clamp(n, limit int) int {
	if limit <= 0 {
		return n
	}
	if n <= 0 || n > limit {
		return limit
	}
	return n
}

// Constraint restricts the value of one request field. All set members must
// hold. Enum and Pattern apply to the value's text form; MaxLen to its length
// in characters; Min and Max only to numeric values (a non-numeric value then
// violates the constraint).
type Constraint struct {
	Enum    []string `yaml:"enum"`
	Pattern string   `yaml:"pattern"`
	MaxLen  int      `yaml:"max_len"`
	Min     *float64 `yaml:"min"`
	Max     *float64 `yaml:"max"`

	re *regexp.Regexp
}

// Rule is one policy rule.
type Rule struct {
	// ID names the rule in decisions and audit records. It must be unique.
	ID     string `yaml:"id"`
	Effect Effect `yaml:"effect"`
	// Verbs and Resources are patterns; "*" matches any run of characters.
	Verbs     []string `yaml:"verbs"`
	Resources []string `yaml:"resources"`
	// Mode is the write mode of an allow rule; empty means ModeAllow.
	Mode Mode `yaml:"mode"`
	// Fields, when non-empty, is the allowlist of request field names.
	Fields []string `yaml:"fields"`
	// Constraints maps a field name to the constraint on its value.
	Constraints map[string]Constraint `yaml:"constraints"`
	// RateLimit applies to requests this rule allows.
	RateLimit Rate `yaml:"rate_limit"`
}

// Policy is a validated policy. Obtain one from Parse or Load; the zero value
// denies everything.
type Policy struct {
	Version   int    `yaml:"version"`
	Limits    Limits `yaml:"limits"`
	RateLimit Rate   `yaml:"rate_limit"`
	Rules     []Rule `yaml:"rules"`

	warnings []string
}

// Warnings returns non-fatal findings from loading, such as a policy file the
// current user can write.
func (p *Policy) Warnings() []string {
	if p == nil {
		return nil
	}
	return append([]string(nil), p.warnings...)
}

// InvalidError reports a policy that could not be parsed or validated. The
// tool must treat it as fatal (fail closed).
type InvalidError struct {
	// Problems lists every validation problem found.
	Problems []string
	// Err is the underlying parse error, if any.
	Err error
}

// Error implements error.
func (e *InvalidError) Error() string {
	if e.Err != nil {
		return "policy invalid: " + e.Err.Error()
	}
	return "policy invalid: " + strings.Join(e.Problems, "; ")
}

// Unwrap returns the underlying parse error.
func (e *InvalidError) Unwrap() error { return e.Err }

// Parse reads a policy from strict YAML. Unknown keys, duplicate keys, an
// empty document and any invalid value are errors of type *InvalidError.
func Parse(data []byte) (*Policy, error) {
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, &InvalidError{Problems: []string{"policy is empty"}}
	}
	var p Policy
	if err := yaml.UnmarshalWithOptions(data, &p, yaml.Strict()); err != nil {
		return nil, &InvalidError{Err: err}
	}
	if probs := p.validate(); len(probs) > 0 {
		return nil, &InvalidError{Problems: probs}
	}
	return &p, nil
}

func (p *Policy) validate() []string {
	var probs []string
	add := func(format string, a ...any) { probs = append(probs, fmt.Sprintf(format, a...)) }
	if p.Version != Version {
		add("version must be %d, got %d", Version, p.Version)
	}
	if p.Limits.MaxResults < 0 || p.Limits.MaxBytes < 0 {
		add("limits must not be negative")
	}
	if p.RateLimit.PerHour < 0 || p.RateLimit.PerRun < 0 {
		add("rate_limit must not be negative")
	}
	if len(p.Rules) == 0 {
		add("rules must contain at least one rule")
	}
	seen := map[string]bool{}
	for i := range p.Rules {
		r := &p.Rules[i]
		where := fmt.Sprintf("rule %d (%q)", i+1, r.ID)
		switch {
		case r.ID == "":
			add("rule %d: id is required", i+1)
		case seen[r.ID]:
			add("%s: duplicate id", where)
		}
		seen[r.ID] = true
		switch r.Effect {
		case EffectAllow:
			switch r.Mode {
			case "":
				r.Mode = ModeAllow
			case ModeAllow, ModeDryRunOnly, ModeDeny:
			default:
				add("%s: unknown mode %q", where, r.Mode)
			}
		case EffectDeny:
			if r.Mode != "" && r.Mode != ModeDeny {
				add("%s: a deny rule cannot set mode %q", where, r.Mode)
			}
			r.Mode = ModeDeny
		default:
			add("%s: effect must be allow or deny, got %q", where, r.Effect)
		}
		if len(r.Verbs) == 0 || hasEmpty(r.Verbs) {
			add("%s: verbs must be non-empty patterns", where)
		}
		if len(r.Resources) == 0 || hasEmpty(r.Resources) {
			add("%s: resources must be non-empty patterns", where)
		}
		if hasEmpty(r.Fields) {
			add("%s: fields must be non-empty names", where)
		}
		if r.RateLimit.PerHour < 0 || r.RateLimit.PerRun < 0 {
			add("%s: rate_limit must not be negative", where)
		}
		for name, c := range r.Constraints {
			if c.Pattern != "" {
				re, err := regexp.Compile(c.Pattern)
				if err != nil {
					add("%s: constraint %q: bad pattern: %v", where, name, err)
				}
				c.re = re
			}
			if c.MaxLen < 0 {
				add("%s: constraint %q: max_len must not be negative", where, name)
			}
			if c.Min != nil && c.Max != nil && *c.Min > *c.Max {
				add("%s: constraint %q: min exceeds max", where, name)
			}
			r.Constraints[name] = c
		}
	}
	return probs
}

func hasEmpty(ss []string) bool {
	for _, s := range ss {
		if s == "" {
			return true
		}
	}
	return false
}

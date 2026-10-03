package policy

import (
	"fmt"
	"os"
	"path/filepath"
)

// WritableMode says what Load does when the current user can write the
// policy file (or the directory holding it).
type WritableMode int

// The writable-file behaviors. POSIX only: on other platforms the check
// finds nothing.
const (
	// WritableWarn loads the policy and records a warning (see
	// Policy.Warnings). It is the default.
	WritableWarn WritableMode = iota
	// WritableRefuse fails Load with a *WritableError.
	WritableRefuse
	// WritableIgnore skips the check.
	WritableIgnore
)

// WritableError reports a policy file that the current user can modify. An
// agent that can edit its own policy has no guardrail; the deployment
// convention is a root-owned directory the agent user can read but not write.
type WritableError struct {
	Path string
	// What is "file" or "directory".
	What string
}

// Error implements error.
func (e *WritableError) Error() string {
	return fmt.Sprintf("policy %s %s is writable by the current user; place the policy where only an administrator can change it", e.What, e.Path)
}

// Option configures Load.
type Option func(*loadConfig)

type loadConfig struct{ writable WritableMode }

// WithWritable sets how Load treats a policy file the current user can write.
func WithWritable(m WritableMode) Option {
	return func(c *loadConfig) { c.writable = m }
}

// Load reads and strictly parses the policy file at path. It fails closed:
// any read or validation error is returned and no Policy is. By default a
// file the current user can write yields a warning; see WithWritable.
func Load(path string, opts ...Option) (*Policy, error) {
	cfg := loadConfig{writable: WritableWarn}
	for _, o := range opts {
		o(&cfg)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("policy: read %s: %w", path, err)
	}
	p, err := Parse(data)
	if err != nil {
		return nil, err
	}
	if cfg.writable == WritableIgnore {
		return p, nil
	}
	for _, t := range []struct{ what, path string }{{"file", path}, {"directory", filepath.Dir(path)}} {
		w, err := writableByMe(t.path)
		if err != nil {
			return nil, fmt.Errorf("policy: check %s: %w", t.path, err)
		}
		if !w {
			continue
		}
		we := &WritableError{Path: t.path, What: t.what}
		if cfg.writable == WritableRefuse {
			return nil, we
		}
		p.warnings = append(p.warnings, we.Error())
	}
	return p, nil
}

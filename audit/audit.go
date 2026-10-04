// Package audit writes a versioned JSON Lines audit log of what a CLI did:
// one record per action, never a secret and never a request or response body.
//
// A Record has no field that can carry a body. Every string field also passes
// through the redaction pass before it is written and is capped in length, so
// a token embedded in a resource name or outcome never reaches the file.
//
// Writes are serialized and each record is written as a single line, so the
// file stays valid JSON Lines under concurrent use. The file is created with
// mode 0600.
package audit

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/stainedhead/agent-cli-core/clock"
	"github.com/stainedhead/agent-cli-core/internal/redact"
)

// SchemaVersion is the version of the Record wire format. It changes only
// under the library's semver rules for the audit schema.
const SchemaVersion = 1

// maxFieldLen caps every string field, in bytes, so free text cannot ride in
// a record.
const maxFieldLen = 512

// Record is one audit entry: what a CLI did, not what it sent or received.
// It deliberately has no body or credential field.
type Record struct {
	// SchemaVersion is set by the Logger to SchemaVersion.
	SchemaVersion int `json:"schema_version"`
	// Timestamp is set by the Logger from its clock when zero.
	Timestamp time.Time `json:"ts"`
	// Tool is the CLI name, for example "demo".
	Tool string `json:"tool"`
	// AgentID identifies the calling agent, if known.
	AgentID string `json:"agent_id"`
	// RunID groups the records of one run.
	RunID string `json:"run_id"`
	// Verb is the action, for example "read" or "write".
	Verb string `json:"verb"`
	// Resource names the target of the action.
	Resource string `json:"resource"`
	// Outcome is a short result label such as "ok", "denied" or "error".
	Outcome string `json:"outcome"`
	// HTTPStatus is the upstream status code, 0 when there was none.
	HTTPStatus int `json:"http_status,omitempty"`
	// Duration is how long the action took.
	Duration time.Duration `json:"-"`
	// PolicyDecision is the policy outcome as a plain string, for example
	// "allow", "dry_run_only" or "deny".
	PolicyDecision string `json:"policy_decision"`
	// RuleID is the id of the policy rule that decided the action, if any.
	RuleID string `json:"rule_id,omitempty"`
	// TargetRef is a reference to the target of the action, such as an
	// object id. It is metadata, never content.
	TargetRef string `json:"target_ref,omitempty"`
	// Extra holds tool-specific metadata columns, for example a recipient
	// count. It is bounded: at most MaxExtraKeys keys, each matching
	// [a-z0-9_.-]{1,32}, each value cut to MaxExtraValueLen bytes. A record
	// that breaks the key rules is not written (see ErrInvalidExtra). Values
	// pass through the redaction pass. Extra must not carry content or
	// credentials.
	//
	// Extra is a pointer so that Record stays comparable with ==, as it was
	// in v0.1.0. A nil or empty Extra is omitted from the output.
	Extra *ExtraFields `json:"extra,omitempty"`
}

// ExtraFields is the key/value set carried by Record.Extra. Build one with a
// composite literal: &audit.ExtraFields{"recipient_count": "2"}.
type ExtraFields map[string]string

// Limits on Record.Extra.
const (
	// MaxExtraKeys is the most keys Record.Extra may hold.
	MaxExtraKeys = 16
	// MaxExtraKeyLen is the longest key, in bytes.
	MaxExtraKeyLen = 32
	// MaxExtraValueLen is the longest value, in bytes; longer values are cut
	// on a rune boundary.
	MaxExtraValueLen = 256
)

// ErrInvalidExtra is matched by errors.Is when Record.Extra breaks the key
// count or key syntax rules. The failed Log also matches ErrWrite.
var ErrInvalidExtra = errors.New("audit: invalid extra field")

// recordJSON is the wire shape; field order here is the column order on disk.
type recordJSON struct {
	SchemaVersion  int       `json:"schema_version"`
	Timestamp      time.Time `json:"ts"`
	Tool           string    `json:"tool"`
	AgentID        string    `json:"agent_id"`
	RunID          string    `json:"run_id"`
	Verb           string    `json:"verb"`
	Resource       string    `json:"resource"`
	Outcome        string    `json:"outcome"`
	HTTPStatus     int       `json:"http_status,omitempty"`
	Duration       string    `json:"duration,omitempty"`
	PolicyDecision string    `json:"policy_decision"`
	RuleID         string    `json:"rule_id,omitempty"`
	TargetRef      string    `json:"target_ref,omitempty"`
	// Extra marshals with sorted keys, so output is deterministic.
	Extra *ExtraFields `json:"extra,omitempty"`
}

// MarshalJSON encodes the record with Duration as a Go duration string.
func (r Record) MarshalJSON() ([]byte, error) {
	w := recordJSON{
		SchemaVersion: r.SchemaVersion, Timestamp: r.Timestamp, Tool: r.Tool,
		AgentID: r.AgentID, RunID: r.RunID, Verb: r.Verb, Resource: r.Resource,
		Outcome: r.Outcome, HTTPStatus: r.HTTPStatus, PolicyDecision: r.PolicyDecision,
		RuleID: r.RuleID, TargetRef: r.TargetRef,
	}
	if r.Extra != nil && len(*r.Extra) > 0 {
		w.Extra = r.Extra
	}
	if r.Duration != 0 {
		w.Duration = r.Duration.String()
	}
	return json.Marshal(w)
}

// UnmarshalJSON decodes a record written by MarshalJSON.
func (r *Record) UnmarshalJSON(b []byte) error {
	var w recordJSON
	if err := json.Unmarshal(b, &w); err != nil {
		return err
	}
	*r = Record{
		SchemaVersion: w.SchemaVersion, Timestamp: w.Timestamp, Tool: w.Tool,
		AgentID: w.AgentID, RunID: w.RunID, Verb: w.Verb, Resource: w.Resource,
		Outcome: w.Outcome, HTTPStatus: w.HTTPStatus, PolicyDecision: w.PolicyDecision,
		RuleID: w.RuleID, TargetRef: w.TargetRef, Extra: w.Extra,
	}
	if w.Duration != "" {
		d, err := time.ParseDuration(w.Duration)
		if err != nil {
			return fmt.Errorf("audit: duration: %w", err)
		}
		r.Duration = d
	}
	return nil
}

// WriteFailureMode says what Handle does when the audit write fails.
type WriteFailureMode int

const (
	// Warn reports a failed write through the OnWriteError hook and lets the
	// action's own result stand. It is the zero value.
	Warn WriteFailureMode = iota
	// Block makes a failed write fail the action.
	Block
)

// String returns "warn", "block" or "unknown".
func (m WriteFailureMode) String() string {
	switch m {
	case Warn:
		return "warn"
	case Block:
		return "block"
	}
	return "unknown"
}

// MarshalText implements encoding.TextMarshaler for config files.
func (m WriteFailureMode) MarshalText() ([]byte, error) {
	if m != Warn && m != Block {
		return nil, fmt.Errorf("audit: invalid write failure mode %d", int(m))
	}
	return []byte(m.String()), nil
}

// UnmarshalText implements encoding.TextUnmarshaler. It accepts "warn" (also
// the empty string) and "block", case-insensitively.
func (m *WriteFailureMode) UnmarshalText(b []byte) error {
	switch strings.ToLower(string(b)) {
	case "", "warn":
		*m = Warn
	case "block":
		*m = Block
	default:
		return fmt.Errorf("audit: unknown write failure mode %q (want warn or block)", b)
	}
	return nil
}

// ErrWrite is matched by errors.Is for every failed audit write.
var ErrWrite = errors.New("audit: write failed")

// ErrNoPath is returned by Open when Config.Path is empty. The library has no
// default path; it comes from the tool's configuration.
var ErrNoPath = errors.New("audit: no log path configured")

// WriteError is the error returned when a record could not be written.
type WriteError struct{ Err error }

// Error describes the failure.
func (e *WriteError) Error() string { return "audit: write failed: " + e.Err.Error() }

// Unwrap returns the underlying cause.
func (e *WriteError) Unwrap() error { return e.Err }

// Is reports whether target is ErrWrite.
func (e *WriteError) Is(target error) bool { return target == ErrWrite }

// Config is the audit section of a tool's configuration.
type Config struct {
	// Path is the JSONL file to append to. Required.
	Path string `json:"path" yaml:"path"`
	// OnFailure selects warn (default) or block.
	OnFailure WriteFailureMode `json:"on_failure" yaml:"on_failure"`
}

// Option customizes a Logger.
type Option func(*Logger)

// WithClock injects the clock used to timestamp records. Any clock.Clock
// works, including a consumer's own implementation or a clock.Fake. A nil
// clock keeps the system clock.
func WithClock(c clock.Clock) Option {
	return func(l *Logger) {
		if c != nil {
			l.clock = c
		}
	}
}

// WithSecrets registers literal secret values that must never be written.
func WithSecrets(secrets ...string) Option {
	return func(l *Logger) { l.red = redact.New(secrets...) }
}

// WithFailureMode sets the behavior of Handle on a failed write.
func WithFailureMode(m WriteFailureMode) Option { return func(l *Logger) { l.mode = m } }

// WithOnWriteError sets the hook Handle calls in Warn mode with a failed
// write's error, so the tool can print it on stderr.
func WithOnWriteError(f func(error)) Option { return func(l *Logger) { l.onErr = f } }

// Logger appends Records to a writer. It is safe for concurrent use.
type Logger struct {
	mu    sync.Mutex
	w     io.Writer
	clock clock.Clock
	red   *redact.Redactor
	mode  WriteFailureMode
	onErr func(error)
}

// NewLogger returns a Logger that writes JSON Lines to w. Use Open for a file.
func NewLogger(w io.Writer, opts ...Option) *Logger {
	l := &Logger{w: w, clock: clock.System{}, red: redact.New()}
	for _, o := range opts {
		o(l)
	}
	return l
}

// Open opens (creating it and its directory if needed) the log file named by
// cfg, with mode 0600, and returns a Logger that appends to it. An existing
// file with wider permissions is tightened to 0600. Close the Logger when done.
func Open(cfg Config, opts ...Option) (*Logger, error) {
	if cfg.Path == "" {
		return nil, ErrNoPath
	}
	if err := os.MkdirAll(filepath.Dir(cfg.Path), 0o700); err != nil {
		return nil, fmt.Errorf("audit: create log directory: %w", err)
	}
	f, err := os.OpenFile(cfg.Path, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o600)
	if err != nil {
		return nil, fmt.Errorf("audit: open log: %w", err)
	}
	if err := f.Chmod(0o600); err != nil && !errors.Is(err, errors.ErrUnsupported) {
		_ = f.Close()
		return nil, fmt.Errorf("audit: set log permissions: %w", err)
	}
	return NewLogger(f, append([]Option{WithFailureMode(cfg.OnFailure)}, opts...)...), nil
}

// Close closes the underlying file if the Logger owns one. It is idempotent.
func (l *Logger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	c, ok := l.w.(io.Closer)
	if !ok {
		return nil
	}
	l.w = closedWriter{}
	return c.Close()
}

type closedWriter struct{}

func (closedWriter) Write([]byte) (int, error) { return 0, os.ErrClosed }

// Log scrubs rec, stamps it and appends it as one line. Any failure is
// returned as a *WriteError (matching ErrWrite); nothing is swallowed. A
// record whose Extra breaks its rules is not written and also matches
// ErrInvalidExtra.
func (l *Logger) Log(rec Record) error {
	if err := checkExtra(rec.Extra); err != nil {
		return &WriteError{Err: err}
	}
	rec = l.sanitize(rec)
	line, err := json.Marshal(rec)
	if err != nil {
		return &WriteError{Err: err}
	}
	line = append(line, '\n')
	l.mu.Lock()
	defer l.mu.Unlock()
	if _, err := l.w.Write(line); err != nil {
		return &WriteError{Err: err}
	}
	return nil
}

// Handle logs rec for an action that finished with actionErr and returns the
// error the tool should act on. If the write succeeds it returns actionErr.
// If it fails, Block mode returns the write error joined with actionErr, so
// the action fails; Warn mode calls the OnWriteError hook and returns
// actionErr unchanged.
func (l *Logger) Handle(rec Record, actionErr error) error {
	err := l.Log(rec)
	if err == nil {
		return actionErr
	}
	if l.mode == Block {
		return errors.Join(actionErr, err)
	}
	if l.onErr != nil {
		l.onErr(err)
	}
	return actionErr
}

func (l *Logger) sanitize(r Record) Record {
	r.SchemaVersion = SchemaVersion
	src := r.Extra
	if r.Timestamp.IsZero() {
		r.Timestamp = l.clock.Now()
	}
	r.Timestamp = r.Timestamp.UTC()
	for _, p := range []*string{&r.Tool, &r.AgentID, &r.RunID, &r.Verb, &r.Resource, &r.Outcome, &r.PolicyDecision, &r.RuleID, &r.TargetRef} {
		*p = capField(l.red.String(*p))
	}
	r.Extra = nil
	if src != nil && len(*src) > 0 {
		m := make(ExtraFields, len(*src))
		for k, v := range *src {
			m[k] = capBytes(l.red.String(v), MaxExtraValueLen)
		}
		r.Extra = &m
	}
	return r
}

// checkExtra validates the count and key syntax of Extra.
func checkExtra(extra *ExtraFields) error {
	if extra == nil {
		return nil
	}
	if len(*extra) > MaxExtraKeys {
		return fmt.Errorf("%w: %d keys (max %d)", ErrInvalidExtra, len(*extra), MaxExtraKeys)
	}
	for k := range *extra {
		if !validExtraKey(k) {
			return fmt.Errorf("%w: key %q must match [a-z0-9_.-]{1,%d}", ErrInvalidExtra, capBytes(k, 40), MaxExtraKeyLen)
		}
	}
	return nil
}

func validExtraKey(k string) bool {
	if k == "" || len(k) > MaxExtraKeyLen {
		return false
	}
	for i := 0; i < len(k); i++ {
		c := k[i]
		if (c < 'a' || c > 'z') && (c < '0' || c > '9') && c != '_' && c != '.' && c != '-' {
			return false
		}
	}
	return true
}

// capBytes cuts s to at most n bytes on a rune boundary.
func capBytes(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}

func capField(s string) string {
	if len(s) <= maxFieldLen {
		return s
	}
	cut := maxFieldLen - 3
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + "..."
}

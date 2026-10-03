package auth

import (
	"fmt"
	"io"
	"log/slog"
)

// redacted is what a Token prints in every format.
const redacted = "[redacted]"

// Token is a bearer token. Its value cannot be read outside this package: it
// prints as "[redacted]" under every fmt verb, JSON, text marshaling and
// log/slog, and there is no accessor. The only way a token reaches the network
// is Authorizer.Authorize writing the Authorization header.
//
// The value is captured in a closure rather than a string or struct field, so
// even when a Token sits in an unexported field (where fmt cannot call its
// methods and walks it by reflection) fmt prints only a function address.
// Limits: code using reflect/unsafe in process, or a memory dump, can still
// read the value.
type Token struct{ get func() string }

// NewToken wraps a token value. Implementers of DaemonClient and TokenSource
// use it to hand a token to this package.
func NewToken(value string) Token {
	if value == "" {
		return Token{}
	}
	return Token{get: func() string { return value }}
}

// IsZero reports whether the token is empty.
func (t Token) IsZero() bool { return t.get == nil }

// String returns "[redacted]".
func (Token) String() string { return redacted }

// GoString returns "[redacted]" so %#v does not leak the value.
func (Token) GoString() string { return redacted }

// Format writes "[redacted]" for every fmt verb.
func (Token) Format(f fmt.State, _ rune) { _, _ = io.WriteString(f, redacted) }

// MarshalText returns "[redacted]".
func (Token) MarshalText() ([]byte, error) { return []byte(redacted), nil }

// MarshalJSON returns the JSON string "[redacted]".
func (Token) MarshalJSON() ([]byte, error) { return []byte(`"` + redacted + `"`), nil }

// LogValue makes log/slog log "[redacted]".
func (Token) LogValue() slog.Value { return slog.StringValue(redacted) }

// reveal returns the value for the Authorization header. It is deliberately
// unexported.
func (t Token) reveal() string {
	if t.get == nil {
		return ""
	}
	return t.get()
}

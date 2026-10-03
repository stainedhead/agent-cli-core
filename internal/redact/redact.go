// Package redact scrubs secrets from strings, headers and bodies before they
// reach an error message, log line, trace or audit record. It is a leaf
// package: it imports only the standard library.
package redact

import (
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strings"
)

// Placeholder replaces every scrubbed value.
const Placeholder = "[redacted]"

// minSecretLen is the shortest registered secret that is scrubbed literally.
// Shorter values would redact innocent text.
const minSecretLen = 6

var (
	// reAuthContext matches a scheme and credential after an "authorization"
	// key; there the credential is redacted whatever it looks like.
	reAuthContext = regexp.MustCompile(`(?i)(\bauthorization\s*[:=]\s*"?)(?:bearer|basic)\s+[A-Za-z0-9._~+/=\-]+`)
	// reAuthScheme matches a bare scheme word and a candidate credential; a
	// match is redacted only if it looks like a credential (see looksLikeCredential).
	reAuthScheme = regexp.MustCompile(`(?i)\b(?:bearer|basic)\s+[A-Za-z0-9._~+/=\-]+`)
	reJWT        = regexp.MustCompile(`\beyJ[A-Za-z0-9_\-]+\.[A-Za-z0-9_\-]+\.[A-Za-z0-9_\-]*`)
	// key=value and "key":"value" forms for sensitive key names.
	reKeyValue = regexp.MustCompile(`(?i)(\b[a-z_\-]*(?:token|secret|password|passwd|api[_\-]?key|authorization)[a-z_\-]*"?\s*[=:]\s*"?)([^\s"&;,}]+)`)
	// Long opaque runs look like keys or tokens. UUIDs (36 chars) stay.
	reOpaque = regexp.MustCompile(`[A-Za-z0-9_\-]{40,}`)
	reSHA1   = regexp.MustCompile(`^[0-9a-f]{40}$`)

	sensitiveHeaderParts = []string{"authorization", "cookie", "token", "secret", "password", "api-key", "apikey"}
)

// OpaqueRunMin is the length at which a run of [A-Za-z0-9_-] is treated as an
// opaque key or token and redacted. UUIDs (36 characters) stay readable. A run
// of exactly 40 lowercase hex digits is a git SHA-1 and is preserved; longer
// hex runs (SHA-256 and up) and any other 40+ character run are redacted.
const OpaqueRunMin = 40

// Thresholds for a credential after a bare "bearer"/"basic" outside an
// authorization context: a token with a digit or symbol needs minMixedCredLen
// characters; a letters-only token needs minLetterCredLen and an uppercase
// letter after the first character (base64-like).
const (
	minMixedCredLen  = 4
	minLetterCredLen = 8
)

// looksLikeCredential reports whether the text after a bare auth-scheme word
// is plausibly a credential and not an ordinary word.
func looksLikeCredential(cred string) bool {
	mixed, upper := false, false
	for i, c := range cred {
		switch {
		case c >= '0' && c <= '9', strings.ContainsRune("._~+/=-", c):
			mixed = true
		case i > 0 && c >= 'A' && c <= 'Z':
			upper = true
		}
	}
	return (mixed && len(cred) >= minMixedCredLen) || (upper && len(cred) >= minLetterCredLen)
}

// Redactor scrubs secrets. The zero value is not useful; use New. A Redactor
// is immutable after construction and safe for concurrent use.
type Redactor struct {
	secrets []string // longest first
}

// New returns a Redactor that, besides the built-in patterns (Authorization
// schemes, JWTs, token/secret/password key-value pairs, opaque runs of
// OpaqueRunMin or more characters except git SHA-1s),
// removes every occurrence of the given literal secrets. Secrets shorter than
// six bytes are ignored.
func New(secrets ...string) *Redactor {
	var keep []string
	for _, s := range secrets {
		if len(s) >= minSecretLen {
			keep = append(keep, s)
		}
	}
	sort.Slice(keep, func(i, j int) bool { return len(keep[i]) > len(keep[j]) })
	return &Redactor{secrets: keep}
}

// String returns s with secrets replaced by Placeholder.
func (r *Redactor) String(s string) string {
	if s == "" {
		return s
	}
	for _, sec := range r.secrets {
		s = strings.ReplaceAll(s, sec, Placeholder)
	}
	s = reAuthContext.ReplaceAllString(s, "${1}"+Placeholder)
	s = reAuthScheme.ReplaceAllStringFunc(s, func(m string) string {
		i := strings.IndexAny(m, " \t\r\n")
		if i < 0 || !looksLikeCredential(strings.TrimSpace(m[i:])) {
			return m
		}
		return Placeholder
	})
	s = reJWT.ReplaceAllString(s, Placeholder)
	s = reKeyValue.ReplaceAllString(s, "${1}"+Placeholder)
	s = reOpaque.ReplaceAllStringFunc(s, func(m string) string {
		if reSHA1.MatchString(m) {
			return m
		}
		return Placeholder
	})
	return s
}

// Header returns a copy of h in which the values of sensitive headers
// (authorization, cookies, anything named like a token, secret, password or
// API key) are replaced and all other values are scrubbed with String. The
// input is not modified. A nil header returns nil.
func (r *Redactor) Header(h http.Header) http.Header {
	if h == nil {
		return nil
	}
	out := make(http.Header, len(h))
	for k, vs := range h {
		nv := make([]string, len(vs))
		sensitive := isSensitiveHeader(k)
		for i, v := range vs {
			if sensitive {
				nv[i] = Placeholder
			} else {
				nv[i] = r.String(v)
			}
		}
		out[k] = nv
	}
	return out
}

func isSensitiveHeader(name string) bool {
	n := strings.ToLower(name)
	for _, p := range sensitiveHeaderParts {
		if strings.Contains(n, p) {
			return true
		}
	}
	return false
}

// Body describes a request or response body without revealing it. Bodies are
// never reproduced: only their size is reported.
func (r *Redactor) Body(b []byte) string {
	return fmt.Sprintf("[body omitted: %d bytes]", len(b))
}

// Error returns an error whose message is err's message scrubbed with String.
// The original error is not wrapped, so a secret held in a wrapped value
// cannot be reached through errors.Unwrap. A nil error returns nil.
func (r *Redactor) Error(err error) error {
	if err == nil {
		return nil
	}
	return errors.New(r.String(err.Error()))
}

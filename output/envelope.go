package output

import (
	"bytes"
	"encoding/json"

	"github.com/stainedhead/agent-cli-core/internal/redact"
)

// Envelope is the single response shape of every command.
//
// A success envelope has OK true, Data and (after Write) Meta. A failure
// envelope has OK false and Error. Marshaling an Envelope with encoding/json
// produces the contract shape; use Write to also apply bounds, format and
// redaction.
type Envelope struct {
	OK    bool
	Data  any
	Meta  *Meta
	Error *Error
}

// Meta describes the data of a success envelope.
type Meta struct {
	// Truncated is true when the data was cut to fit the output bound.
	Truncated bool `json:"truncated"`
	// NextOffset is where to resume (Bounds.Offset) when Truncated, else nil.
	NextOffset *int `json:"next_offset"`
	// Count is the number of items in data when data is an array. Write sets
	// it for arrays; for other data the caller's value is kept.
	Count int `json:"count"`
	// RequestID identifies the request for support; omitted when empty.
	RequestID string `json:"request_id,omitempty"`
}

// Error is the payload of a failure envelope.
type Error struct {
	// Code is the stable machine-readable category.
	Code Category `json:"code"`
	// Message says what happened. It never contains a token or a body.
	Message string `json:"message"`
	// Hint says what to do next. Omitted when empty.
	Hint string `json:"hint,omitempty"`
}

// Success returns a success envelope. meta may be nil.
func Success(data any, meta *Meta) Envelope {
	return Envelope{OK: true, Data: data, Meta: meta}
}

// Failure returns a failure envelope. Message and hint are passed through the
// redactor.
func Failure(c Category, message, hint string) Envelope {
	r := redact.New()
	return Envelope{Error: &Error{Code: c, Message: r.String(message), Hint: r.String(hint)}}
}

// FromError returns the envelope for err: a success envelope with no data for
// nil, otherwise a failure envelope whose code is CategoryOf(err), whose
// message is err.Error() and whose hint comes from a Hinter in the chain, both
// redacted.
func FromError(err error) Envelope {
	if err == nil {
		return Success(nil, nil)
	}
	hint := ""
	if h, ok := findHinter(err); ok {
		hint = h.Hint()
	}
	return Failure(CategoryOf(err), err.Error(), hint)
}

func findHinter(err error) (Hinter, bool) {
	for err != nil {
		if h, ok := err.(Hinter); ok {
			return h, true
		}
		switch u := err.(type) {
		case interface{ Unwrap() error }:
			err = u.Unwrap()
		default:
			return nil, false
		}
	}
	return nil, false
}

// ExitCode returns the process exit code that agrees with the envelope: 0 for
// success, otherwise the code of the error's category.
func (e Envelope) ExitCode() ExitCode {
	if e.OK {
		return ExitOK
	}
	if e.Error == nil {
		return ExitGeneral
	}
	return ExitFor(e.Error.Code)
}

// MarshalJSON emits the contract shape: success is {ok,data,meta}, failure is
// {ok,error}. HTML characters are not escaped.
func (e Envelope) MarshalJSON() ([]byte, error) {
	if e.OK {
		out := struct {
			OK   bool  `json:"ok"`
			Data any   `json:"data"`
			Meta *Meta `json:"meta,omitempty"`
		}{true, e.Data, e.Meta}
		return marshal(out)
	}
	out := struct {
		OK    bool   `json:"ok"`
		Error *Error `json:"error"`
	}{false, e.Error}
	return marshal(out)
}

// marshal is json.Marshal without HTML escaping and without the trailing
// newline.
func marshal(v any) ([]byte, error) {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimRight(b.Bytes(), "\n"), nil
}

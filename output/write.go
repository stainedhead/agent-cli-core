package output

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"unicode/utf8"

	"github.com/stainedhead/agent-cli-core/internal/redact"
)

// DefaultMaxBytes is the output bound used when Bounds.MaxBytes is zero.
const DefaultMaxBytes = 32768

// Format selects how an envelope is rendered.
type Format string

// The output formats. JSON is the default for agents.
const (
	FormatJSON  Format = "json"
	FormatTable Format = "table"
	FormatText  Format = "text"
)

// ParseFormat parses a --format value. The empty string means FormatJSON.
func ParseFormat(s string) (Format, error) {
	switch Format(s) {
	case "", FormatJSON:
		return FormatJSON, nil
	case FormatTable:
		return FormatTable, nil
	case FormatText:
		return FormatText, nil
	}
	return "", fmt.Errorf("unknown output format %q (want json, table or text)", s)
}

// Bounds caps the size of one write.
type Bounds struct {
	// MaxBytes is the largest permitted output in bytes, in the chosen
	// format. Zero means DefaultMaxBytes; negative is invalid.
	MaxBytes int
	// Offset is where to resume after a truncated write: the index of the
	// first item for array data, the byte offset for string data. Use the
	// previous meta.next_offset.
	Offset int
}

// Options controls Write.
type Options struct {
	// Format defaults to FormatJSON.
	Format Format
	// Bounds caps the output size and selects the resume point.
	Bounds Bounds
	// Secrets are literal values (for example a token the process holds) that
	// must never appear in an error message or hint, in addition to the
	// built-in patterns.
	Secrets []string
}

// Errors returned by Write for caller mistakes. They carry CategoryUsage, so ExitOf maps them to exit 2
var (
	// ErrInvalidBounds means MaxBytes or Offset is negative.
	ErrInvalidBounds = categoryErr{CategoryUsage, "invalid output bounds: max-bytes and offset must not be negative"}
	// ErrOffsetOutOfRange means Bounds.Offset is past the end of the data.
	ErrOffsetOutOfRange = categoryErr{CategoryUsage, "offset is past the end of the data"}
	// ErrBoundTooSmall means the smallest unit of data (one array item, one
	// character, or a non-splittable value) does not fit in MaxBytes.
	ErrBoundTooSmall = categoryErr{CategoryUsage, "max-bytes is too small for the smallest unit of output"}
	// ErrUnknownFormat means Options.Format is not json, table or text.
	ErrUnknownFormat = categoryErr{CategoryUsage, "unknown output format"}
)

type categoryErr struct {
	cat Category
	msg string
}

func (e categoryErr) Error() string      { return e.msg }
func (e categoryErr) Category() Category { return e.cat }

// Write renders env in opts.Format, within opts.Bounds, and writes it
// followed by a newline. Success data is marshaled with encoding/json (so
// Untrusted values are marked); error message and hint are redacted. Write
// sets meta (creating it if nil): Count for array data, and Truncated and
// NextOffset when the data had to be cut.
//
// A failure envelope is never truncated. When the data cannot be cut to fit,
// Write returns one of the Err* values and writes nothing.
func Write(w io.Writer, env Envelope, opts Options) error {
	b, err := Render(env, opts)
	if err != nil {
		return err
	}
	_, err = w.Write(b)
	return err
}

// Render is Write without the io.Writer: it returns the bytes that Write
// would write.
func Render(env Envelope, opts Options) ([]byte, error) {
	f := opts.Format
	if f == "" {
		f = FormatJSON
	}
	if f != FormatJSON && f != FormatTable && f != FormatText {
		return nil, ErrUnknownFormat
	}
	if opts.Bounds.MaxBytes < 0 || opts.Bounds.Offset < 0 {
		return nil, ErrInvalidBounds
	}
	max := opts.Bounds.MaxBytes
	if max == 0 {
		max = DefaultMaxBytes
	}

	if !env.OK {
		return renderError(env, f, opts.Secrets)
	}

	v, err := newView(env.Data)
	if err != nil {
		return nil, fmt.Errorf("output: encode data: %w", err)
	}
	meta := Meta{}
	if env.Meta != nil {
		meta = *env.Meta
	}
	meta.Truncated, meta.NextOffset = false, nil
	return fit(v, meta, f, max, opts.Bounds.Offset)
}

func renderError(env Envelope, f Format, secrets []string) ([]byte, error) {
	e := Error{Code: CategoryGeneral, Message: "unspecified error"}
	if env.Error != nil {
		e = *env.Error
	}
	r := redact.New(secrets...)
	e.Message, e.Hint = r.String(e.Message), r.String(e.Hint)
	if f == FormatJSON {
		b, err := Envelope{Error: &e}.MarshalJSON()
		return append(b, '\n'), err
	}
	s := "error: " + string(e.Code) + ": " + e.Message + "\n"
	if e.Hint != "" {
		s += "hint: " + e.Hint + "\n"
	}
	return []byte(s), nil
}

// ---- bounding ----

type viewKind int

const (
	vOther viewKind = iota
	vArray
	vString
)

// view is the data of a success envelope in a form that can be cut.
type view struct {
	kind  viewKind
	items []json.RawMessage // vArray
	str   string            // vString
	raw   json.RawMessage   // vOther
}

func newView(data any) (view, error) {
	raw, err := marshal(data)
	if err != nil {
		return view{}, err
	}
	switch firstByte(raw) {
	case '[':
		var items []json.RawMessage
		if err := json.Unmarshal(raw, &items); err != nil {
			return view{}, err
		}
		return view{kind: vArray, items: items}, nil
	case '"':
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return view{}, err
		}
		return view{kind: vString, str: s}, nil
	}
	return view{kind: vOther, raw: raw}, nil
}

func firstByte(b []byte) byte {
	for _, c := range b {
		if c != ' ' && c != '\n' && c != '\t' && c != '\r' {
			return c
		}
	}
	return 0
}

func (v view) length() int {
	switch v.kind {
	case vArray:
		return len(v.items)
	case vString:
		return len(v.str)
	}
	return 0
}

// slice returns the part of v in [off, off+n).
func (v view) slice(off, n int) view {
	switch v.kind {
	case vArray:
		return view{kind: vArray, items: v.items[off : off+n]}
	case vString:
		return view{kind: vString, str: v.str[off : off+n]}
	}
	return v
}

func (v view) jsonData() []byte {
	switch v.kind {
	case vArray:
		b := []byte{'['}
		for i, it := range v.items {
			if i > 0 {
				b = append(b, ',')
			}
			b = append(b, it...)
		}
		return append(b, ']')
	case vString:
		b, _ := marshal(v.str)
		return b
	}
	if len(v.raw) == 0 {
		return []byte("null")
	}
	return v.raw
}

func (v view) node() (node, error) { return parseNode(v.jsonData()) }

// fit finds the largest prefix of v (from offset) whose rendering is within
// max bytes and renders it with truncation metadata.
func fit(v view, meta Meta, f Format, max, offset int) ([]byte, error) {
	total := v.length()
	if v.kind == vOther {
		if offset != 0 {
			return nil, ErrOffsetOutOfRange
		}
	} else {
		if offset > total {
			return nil, ErrOffsetOutOfRange
		}
		if v.kind == vString {
			for offset < total && !utf8.RuneStart(v.str[offset]) {
				offset++
			}
		}
	}
	remaining := total - offset

	render := func(part view, count int, truncated bool, next int) ([]byte, error) {
		m := meta
		if v.kind == vArray {
			m.Count = count
		}
		if truncated {
			m.Truncated = true
			n := next
			m.NextOffset = &n
		}
		return renderSuccess(part, m, f)
	}

	// Everything fits?
	full, err := render(v.slice(offset, remaining), remaining, false, 0)
	if err != nil {
		return nil, err
	}
	if len(full) <= max {
		return full, nil
	}
	if v.kind == vOther || remaining == 0 {
		return nil, ErrBoundTooSmall
	}

	// Unit boundaries: array items are 1 unit each; strings cut at rune starts.
	fits := func(n int) ([]byte, bool, error) {
		b, err := render(v.slice(offset, n), n, true, offset+n)
		if err != nil {
			return nil, false, err
		}
		return b, len(b) <= max, nil
	}
	var cuts []int // candidate lengths n in (0, remaining), ascending
	if v.kind == vArray {
		for n := 1; n < remaining; n++ {
			cuts = append(cuts, n)
		}
	} else {
		for n := 1; n < remaining; n++ {
			if utf8.RuneStart(v.str[offset+n]) {
				cuts = append(cuts, n)
			}
		}
	}
	// Largest candidate that fits (rendered size is non-decreasing in n).
	idx := sort.Search(len(cuts), func(i int) bool {
		_, ok, err := fits(cuts[i])
		return err != nil || !ok
	}) - 1
	if idx < 0 {
		return nil, ErrBoundTooSmall
	}
	b, ok, err := fits(cuts[idx])
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrBoundTooSmall
	}
	return b, nil
}

func renderSuccess(part view, m Meta, f Format) ([]byte, error) {
	switch f {
	case FormatJSON:
		out := struct {
			OK   bool            `json:"ok"`
			Data json.RawMessage `json:"data"`
			Meta Meta            `json:"meta"`
		}{true, part.jsonData(), m}
		b, err := marshal(out)
		if err != nil {
			return nil, err
		}
		return append(b, '\n'), nil
	default:
		n, err := part.node()
		if err != nil {
			return nil, err
		}
		return renderPlain(n, part.kind == vArray, m, f), nil
	}
}

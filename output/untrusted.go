package output

import (
	"strconv"
	"strings"
	"time"
)

// Untrusted wraps a free-text field written by someone else (a ticket
// description, a comment, an email body, a chat message). Place it in the
// envelope data wherever the tool knows a field is free text; the library
// marks it, the tool decides which fields.
//
// In JSON it marshals as
//
//	{"untrusted":true,"value":"...","author":"...","timestamp":"RFC 3339"}
//
// (author and timestamp are omitted when empty). In table and text output it
// is wrapped in delimiters that carry the author and timestamp. The marking
// is a mitigation against prompt injection, not a guarantee.
type Untrusted struct {
	Value     string
	Author    string
	Timestamp time.Time
}

const (
	untrustedOpen  = "<<<UNTRUSTED"
	untrustedClose = "<<<END UNTRUSTED>>>"
)

// MarshalJSON implements json.Marshaler.
func (u Untrusted) MarshalJSON() ([]byte, error) {
	out := struct {
		Untrusted bool   `json:"untrusted"`
		Value     string `json:"value"`
		Author    string `json:"author,omitempty"`
		Timestamp string `json:"timestamp,omitempty"`
	}{Untrusted: true, Value: u.Value, Author: u.Author}
	if !u.Timestamp.IsZero() {
		out.Timestamp = u.Timestamp.UTC().Format(time.RFC3339)
	}
	return marshal(out)
}

// String returns the text-format rendering, so an Untrusted value is marked
// even when printed with fmt.
func (u Untrusted) String() string {
	ts := ""
	if !u.Timestamp.IsZero() {
		ts = u.Timestamp.UTC().Format(time.RFC3339)
	}
	return untrustedBlock(u.Value, u.Author, ts, false)
}

// untrustedBlock renders the delimiters. Inline mode keeps everything on one
// line (for table cells). Delimiter look-alikes inside the body are broken
// so the content cannot close its own block.
func untrustedBlock(value, author, timestamp string, inline bool) string {
	value = strings.ReplaceAll(value, "<<<", "<< <")
	var b strings.Builder
	b.WriteString(untrustedOpen)
	b.WriteString(" author=" + strconv.Quote(author))
	b.WriteString(" timestamp=" + strconv.Quote(timestamp))
	b.WriteString(">>>")
	if inline {
		b.WriteString(" " + oneLine(value) + " ")
	} else {
		b.WriteString("\n" + value + "\n")
	}
	b.WriteString(untrustedClose)
	return b.String()
}

// oneLine escapes line breaks and tabs so a value fits in one table cell.
func oneLine(s string) string {
	r := strings.NewReplacer("\r\n", `\n`, "\n", `\n`, "\r", `\r`, "\t", `\t`)
	return r.Replace(s)
}

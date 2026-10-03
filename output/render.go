package output

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// renderPlain renders data for the table and text formats, followed by a
// meta footer line.
func renderPlain(n node, isArray bool, m Meta, f Format) []byte {
	var b strings.Builder
	if f == FormatTable {
		renderTable(&b, n)
	} else {
		renderText(&b, n, 0)
	}
	b.WriteString(metaLine(m, isArray))
	b.WriteByte('\n')
	return []byte(b.String())
}

func metaLine(m Meta, isArray bool) string {
	parts := []string{}
	if isArray {
		parts = append(parts, fmt.Sprintf("count=%d", m.Count))
	}
	parts = append(parts, fmt.Sprintf("truncated=%t", m.Truncated))
	if m.NextOffset != nil {
		parts = append(parts, fmt.Sprintf("next_offset=%d", *m.NextOffset))
	}
	if m.RequestID != "" {
		parts = append(parts, "request_id="+oneLine(m.RequestID))
	}
	return "-- " + strings.Join(parts, " ")
}

func isScalar(n node) bool {
	if _, _, _, ok := n.untrusted(); ok {
		return false
	}
	return n.kind != nArray && n.kind != nObject
}

func untrustedText(n node, inline bool) (string, bool) {
	v, a, ts, ok := n.untrusted()
	if !ok {
		return "", false
	}
	return untrustedBlock(v, a, ts, inline), true
}

// ---- text ----

func renderText(b *strings.Builder, n node, indent int) {
	pad := strings.Repeat(" ", indent)
	if blk, ok := untrustedText(n, false); ok {
		writeLines(b, blk, pad)
		return
	}
	switch n.kind {
	case nArray:
		for i, c := range n.items {
			if i > 0 && !isScalar(c) {
				b.WriteByte('\n')
			}
			if isScalar(c) {
				writeLines(b, "- "+c.compact(), pad)
				continue
			}
			b.WriteString(pad + "-\n")
			renderText(b, c, indent+2)
		}
	case nObject:
		for i, c := range n.items {
			key := n.keys[i]
			if isScalar(c) && !strings.Contains(c.s, "\n") {
				b.WriteString(pad + key + ": " + c.compact() + "\n")
				continue
			}
			b.WriteString(pad + key + ":\n")
			if isScalar(c) {
				writeLines(b, c.s, pad+"  ")
				continue
			}
			renderText(b, c, indent+2)
		}
	case nNull:
		// null data prints nothing; the meta footer still follows.
	default:
		writeLines(b, n.compact(), pad)
	}
}

func writeLines(b *strings.Builder, s, pad string) {
	for _, line := range strings.Split(s, "\n") {
		b.WriteString(pad + line + "\n")
	}
}

// ---- table ----

func renderTable(b *strings.Builder, n node) {
	switch {
	case n.kind == nArray:
		rows := n.items
		if len(rows) == 0 {
			return
		}
		var cols []string
		seen := map[string]bool{}
		allObjects := true
		for _, r := range rows {
			if _, _, _, ok := r.untrusted(); ok || r.kind != nObject {
				allObjects = false
				break
			}
			for _, k := range r.keys {
				if !seen[k] {
					seen[k] = true
					cols = append(cols, k)
				}
			}
		}
		if !allObjects {
			cells := [][]string{{"value"}}
			for _, r := range rows {
				cells = append(cells, []string{cell(r)})
			}
			writeGrid(b, cells)
			return
		}
		cells := [][]string{cols}
		for _, r := range rows {
			line := make([]string, len(cols))
			for i, k := range cols {
				if v, ok := r.get(k); ok {
					line[i] = cell(v)
				}
			}
			cells = append(cells, line)
		}
		writeGrid(b, cells)
	case n.kind == nObject && !isUntrustedNode(n):
		cells := [][]string{{"key", "value"}}
		for i, k := range n.keys {
			cells = append(cells, []string{oneLine(k), cell(n.items[i])})
		}
		writeGrid(b, cells)
	case n.kind == nNull:
	default:
		b.WriteString(cell(n) + "\n")
	}
}

func isUntrustedNode(n node) bool {
	_, _, _, ok := n.untrusted()
	return ok
}

func cell(n node) string {
	if blk, ok := untrustedText(n, true); ok {
		return blk
	}
	return oneLine(n.compact())
}

// writeGrid writes rows aligned in columns with a separator under the first
// row. Widths are measured in runes.
func writeGrid(b *strings.Builder, rows [][]string) {
	widths := make([]int, len(rows[0]))
	for _, r := range rows {
		for i, c := range r {
			if w := utf8.RuneCountInString(c); w > widths[i] {
				widths[i] = w
			}
		}
	}
	line := func(r []string) {
		var sb strings.Builder
		for i, c := range r {
			sb.WriteString(c)
			if i < len(r)-1 {
				sb.WriteString(strings.Repeat(" ", widths[i]-utf8.RuneCountInString(c)+2))
			}
		}
		b.WriteString(strings.TrimRight(sb.String(), " ") + "\n")
	}
	line(rows[0])
	sep := make([]string, len(widths))
	for i, w := range widths {
		sep[i] = strings.Repeat("-", w)
	}
	line(sep)
	for _, r := range rows[1:] {
		line(r)
	}
}

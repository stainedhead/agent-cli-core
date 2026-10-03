package output

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

type nodeKind int

const (
	nNull nodeKind = iota
	nBool
	nNumber
	nString
	nArray
	nObject
)

// node is an order-preserving JSON tree used by the table and text writers.
type node struct {
	kind  nodeKind
	s     string // string value, or literal text for numbers and booleans
	items []node // array elements, or object values
	keys  []string
}

func parseNode(raw []byte) (node, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	n, err := decodeNode(dec)
	if err != nil {
		return node{}, err
	}
	return n, nil
}

func decodeNode(dec *json.Decoder) (node, error) {
	tok, err := dec.Token()
	if err != nil {
		return node{}, err
	}
	switch t := tok.(type) {
	case nil:
		return node{kind: nNull, s: "null"}, nil
	case bool:
		return node{kind: nBool, s: fmt.Sprint(t)}, nil
	case json.Number:
		return node{kind: nNumber, s: t.String()}, nil
	case string:
		return node{kind: nString, s: t}, nil
	case json.Delim:
		if t == '[' {
			n := node{kind: nArray}
			for dec.More() {
				c, err := decodeNode(dec)
				if err != nil {
					return node{}, err
				}
				n.items = append(n.items, c)
			}
			if _, err := dec.Token(); err != nil {
				return node{}, err
			}
			return n, nil
		}
		n := node{kind: nObject}
		for dec.More() {
			k, err := dec.Token()
			if err != nil {
				return node{}, err
			}
			c, err := decodeNode(dec)
			if err != nil {
				return node{}, err
			}
			n.keys = append(n.keys, k.(string))
			n.items = append(n.items, c)
		}
		if _, err := dec.Token(); err != nil {
			return node{}, err
		}
		return n, nil
	}
	return node{}, io.ErrUnexpectedEOF
}

func (n node) get(key string) (node, bool) {
	for i, k := range n.keys {
		if k == key {
			return n.items[i], true
		}
	}
	return node{}, false
}

// untrusted reports whether n is a marked untrusted value and returns its
// parts.
func (n node) untrusted() (value, author, timestamp string, ok bool) {
	if n.kind != nObject {
		return
	}
	flag, has := n.get("untrusted")
	if !has || flag.kind != nBool || flag.s != "true" {
		return
	}
	v, has := n.get("value")
	if !has || v.kind != nString {
		return
	}
	value = v.s
	if a, has := n.get("author"); has && a.kind == nString {
		author = a.s
	}
	if t, has := n.get("timestamp"); has && t.kind == nString {
		timestamp = t.s
	}
	return value, author, timestamp, true
}

// compact renders n as one-line JSON-ish text for table cells.
func (n node) compact() string {
	switch n.kind {
	case nNull, nBool, nNumber:
		return n.s
	case nString:
		return n.s
	}
	var b bytes.Buffer
	writeCompact(&b, n)
	return b.String()
}

func writeCompact(b *bytes.Buffer, n node) {
	switch n.kind {
	case nString:
		q, _ := marshal(n.s)
		b.Write(q)
	case nArray:
		b.WriteByte('[')
		for i, c := range n.items {
			if i > 0 {
				b.WriteByte(',')
			}
			writeCompact(b, c)
		}
		b.WriteByte(']')
	case nObject:
		b.WriteByte('{')
		for i, c := range n.items {
			if i > 0 {
				b.WriteByte(',')
			}
			k, _ := marshal(n.keys[i])
			b.Write(k)
			b.WriteByte(':')
			writeCompact(b, c)
		}
		b.WriteByte('}')
	default:
		b.WriteString(n.s)
	}
}

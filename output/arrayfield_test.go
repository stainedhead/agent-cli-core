package output_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/stainedhead/agent-cli-core/output"
)

type page struct {
	Items []item `json:"items"`
	Total int    `json:"total"`
	Note  string `json:"note"`
}

func renderField(t *testing.T, data any, meta *output.Meta, f output.Format, max, off int) ([]byte, error) {
	t.Helper()
	return output.Render(output.Success(data, meta), output.Options{
		Format: f,
		Bounds: output.Bounds{MaxBytes: max, Offset: off, ArrayField: "items"},
	})
}

// R1: next_page_token is emitted after request_id, only when set.
func TestMetaNextPageToken(t *testing.T) {
	b, err := json.Marshal(output.Meta{Count: 1, NextPageToken: "tok/1=="})
	if err != nil {
		t.Fatal(err)
	}
	want := `{"truncated":false,"next_offset":null,"count":1,"next_page_token":"tok/1=="}`
	if string(b) != want {
		t.Errorf("got %s", b)
	}
	b, _ = json.Marshal(output.Meta{RequestID: "r", NextPageToken: "t"})
	if !strings.HasSuffix(string(b), `"request_id":"r","next_page_token":"t"}`) {
		t.Errorf("order: %s", b)
	}
	b, _ = json.Marshal(output.Meta{})
	if strings.Contains(string(b), "next_page_token") {
		t.Errorf("empty token must be omitted: %s", b)
	}
}

func TestNextPageTokenRendering(t *testing.T) {
	m := &output.Meta{NextPageToken: "abc\ndef"}
	for _, f := range []output.Format{output.FormatJSON, output.FormatTable, output.FormatText} {
		b := mustRender(t, output.Success(items(2), m), output.Options{Format: f})
		if !strings.Contains(string(b), "abc") {
			t.Errorf("%s: token missing: %s", f, b)
		}
		if f != output.FormatJSON && !strings.Contains(string(b), "next_page_token=abc\\ndef") {
			t.Errorf("%s: footer: %s", f, b)
		}
	}
	// No token: footer unchanged.
	b := mustRender(t, output.Success(items(2), nil), output.Options{Format: output.FormatText})
	if strings.Contains(string(b), "next_page_token") {
		t.Errorf("unexpected token: %s", b)
	}
}

func TestNextPageTokenSurvivesTruncationAndCountsInBudget(t *testing.T) {
	tok := strings.Repeat("T", 200)
	data := page{Items: make([]item, 30), Total: 30}
	for i := range data.Items {
		data.Items[i] = item{ID: fmt.Sprint(i), Title: "t", State: "s"}
	}
	b, err := renderField(t, data, &output.Meta{NextPageToken: tok}, output.FormatJSON, 600, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(b) > 600 {
		t.Errorf("over budget: %d", len(b))
	}
	var p struct {
		Meta output.Meta `json:"meta"`
	}
	if err := json.Unmarshal(b, &p); err != nil {
		t.Fatal(err)
	}
	if p.Meta.NextPageToken != tok || !p.Meta.Truncated {
		t.Errorf("meta: %+v", p.Meta)
	}
}

func TestArrayFieldFitsUnchanged(t *testing.T) {
	data := page{Items: items(2), Total: 2, Note: "<ok>"}
	b, err := renderField(t, data, nil, output.FormatJSON, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	plain := mustRender(t, output.Success(data, nil), output.Options{})
	// Same data bytes; only meta.count differs (set to the item count).
	var a, c struct {
		Data json.RawMessage `json:"data"`
		Meta output.Meta     `json:"meta"`
	}
	_ = json.Unmarshal(b, &a)
	_ = json.Unmarshal(plain, &c)
	if string(a.Data) != string(c.Data) {
		t.Errorf("data changed:\n%s\n%s", a.Data, c.Data)
	}
	if a.Meta.Truncated || a.Meta.NextOffset != nil || a.Meta.Count != 2 {
		t.Errorf("meta %+v", a.Meta)
	}
}

func TestArrayFieldKeyOrderPreserved(t *testing.T) {
	data := map[string]any{"z": 1, "items": []int{1, 2, 3}, "a": 2}
	// encoding/json sorts map keys: a, items, z. Struct order is kept below.
	b, err := renderField(t, data, nil, output.FormatJSON, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"data":{"a":2,"items":[1,2,3],"z":1}`) {
		t.Errorf("%s", b)
	}
	b, _ = renderField(t, page{Items: items(0), Total: 7, Note: "n"}, nil, output.FormatJSON, 0, 0)
	if !strings.Contains(string(b), `"data":{"items":[],"total":7,"note":"n"}`) {
		t.Errorf("%s", b)
	}
}

func TestArrayFieldTruncates(t *testing.T) {
	data := page{Items: items(40), Total: 40, Note: "n"}
	b, err := renderField(t, data, nil, output.FormatJSON, 700, 0)
	if err != nil {
		t.Fatal(err)
	}
	checkPage(t, b, 700)
	var p struct {
		Data page        `json:"data"`
		Meta output.Meta `json:"meta"`
	}
	if err := json.Unmarshal(b, &p); err != nil {
		t.Fatal(err)
	}
	k := len(p.Data.Items)
	if !p.Meta.Truncated || p.Meta.NextOffset == nil || *p.Meta.NextOffset != k || p.Meta.Count != k {
		t.Fatalf("meta %+v kept %d", p.Meta, k)
	}
	if p.Data.Total != 40 || p.Data.Note != "n" || k == 0 || k >= 40 {
		t.Errorf("data %+v", p.Data)
	}
	// Maximal: one more item would not fit.
	one, _ := json.Marshal(data.Items[k])
	if len(b)+len(one)+1 <= 700 {
		t.Errorf("not maximal: k=%d len=%d", k, len(b))
	}
}

// Paging with ArrayField reassembles the original in every format.
func TestArrayFieldPaging(t *testing.T) {
	data := page{Items: items(57), Total: 57}
	for _, max := range []int{300, 450, 1000} {
		off, seen := 0, 0
		for {
			b, err := renderField(t, data, nil, output.FormatJSON, max, off)
			if err != nil {
				t.Fatal(err)
			}
			checkPage(t, b, max)
			var p struct {
				Data page        `json:"data"`
				Meta output.Meta `json:"meta"`
			}
			if err := json.Unmarshal(b, &p); err != nil {
				t.Fatal(err)
			}
			for i, it := range p.Data.Items {
				if it != data.Items[off+i] {
					t.Fatalf("mismatch at %d", off+i)
				}
			}
			seen += len(p.Data.Items)
			if !p.Meta.Truncated {
				break
			}
			if *p.Meta.NextOffset <= off {
				t.Fatal("no progress")
			}
			off = *p.Meta.NextOffset
		}
		if seen != 57 {
			t.Errorf("max %d: saw %d", max, seen)
		}
	}
	for _, f := range []output.Format{output.FormatTable, output.FormatText} {
		b, err := renderField(t, data, nil, f, 400, 0)
		if err != nil {
			t.Fatal(err)
		}
		checkPage(t, b, 400)
		if !strings.Contains(string(b), "truncated=true next_offset=") || !strings.Contains(string(b), "count=") {
			t.Errorf("%s footer: %s", f, b)
		}
	}
}

func TestArrayFieldOffsetAndEdges(t *testing.T) {
	data := page{Items: items(5), Total: 5}
	b, err := renderField(t, data, nil, output.FormatJSON, 0, 5) // offset == len: empty page
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"items":[]`) || strings.Contains(string(b), `"truncated":true`) {
		t.Errorf("%s", b)
	}
	if _, err = renderField(t, data, nil, output.FormatJSON, 0, 6); !errors.Is(err, output.ErrOffsetOutOfRange) {
		t.Errorf("offset past end: %v", err)
	}
	// null array: treated as empty, left as null.
	b, err = renderField(t, page{}, nil, output.FormatJSON, 0, 0)
	if err != nil || !strings.Contains(string(b), `"items":null`) {
		t.Errorf("null: %v %s", err, b)
	}
	if _, err = renderField(t, page{}, nil, output.FormatJSON, 0, 1); !errors.Is(err, output.ErrOffsetOutOfRange) {
		t.Errorf("null offset: %v", err)
	}
}

func TestArrayFieldErrors(t *testing.T) {
	cases := map[string]any{
		"missing":      map[string]any{"other": []int{1}},
		"not array":    map[string]any{"items": "x"},
		"array data":   []int{1, 2},
		"string data":  "hello",
		"nil data":     nil,
		"object field": map[string]any{"items": map[string]any{}},
	}
	for name, d := range cases {
		_, err := renderField(t, d, nil, output.FormatJSON, 0, 0)
		if !errors.Is(err, output.ErrArrayField) {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if output.ExitOf(err) != output.ExitUsage {
			t.Errorf("%s: exit %d", name, output.ExitOf(err))
		}
	}
	if output.ErrArrayField.Error() == "" {
		t.Error("empty message")
	}
	// Failure envelopes ignore ArrayField.
	if _, err := renderField(t, nil, nil, output.FormatJSON, 0, 0); err == nil {
		t.Fatal("expected error")
	}
	b, err := output.Render(output.Failure(output.CategoryGeneral, "m", ""), output.Options{Bounds: output.Bounds{ArrayField: "items"}})
	if err != nil || !strings.Contains(string(b), `"ok":false`) {
		t.Errorf("failure: %v %s", err, b)
	}
}

func TestArrayFieldTooSmall(t *testing.T) {
	data := page{Items: items(3), Total: 3}
	if _, err := renderField(t, data, nil, output.FormatJSON, 100, 0); !errors.Is(err, output.ErrBoundTooSmall) {
		t.Errorf("got %v", err)
	}
	// Zero items would fit but one does not: still too small as an item must fit.
	empty := page{Items: items(0), Total: 3}
	b, _ := renderField(t, empty, nil, output.FormatJSON, 0, 0)
	if _, err := renderField(t, data, nil, output.FormatJSON, len(b)+5, 0); !errors.Is(err, output.ErrBoundTooSmall) {
		t.Errorf("got %v", err)
	}
}

func TestArrayFieldUnicodeItems(t *testing.T) {
	d := map[string]any{"items": []string{"é世🙂", "é世🙂", "é世🙂", "é世🙂"}}
	b, err := renderField(t, d, nil, output.FormatJSON, 110, 0)
	if err != nil {
		t.Fatal(err)
	}
	checkPage(t, b, 110)
	if !strings.Contains(string(b), "é世🙂") {
		t.Errorf("%s", b)
	}
}

// Unset ArrayField leaves object data exactly as in v0.1.0.
func TestArrayFieldUnsetUnchanged(t *testing.T) {
	data := page{Items: items(1000), Total: 1000}
	if _, err := output.Render(output.Success(data, nil), output.Options{}); !errors.Is(err, output.ErrBoundTooSmall) {
		t.Errorf("v0.1.0 behaviour changed: %v", err)
	}
}

func ExampleBounds_arrayField() {
	data := map[string]any{"items": []string{"alpha", "bravo", "charlie", "delta"}, "total": 4}
	err := output.Write(os.Stdout, output.Success(data, &output.Meta{NextPageToken: "tok-2"}), output.Options{
		Bounds: output.Bounds{MaxBytes: 150, ArrayField: "items"},
	})
	fmt.Println(err)
	// Output:
	// {"ok":true,"data":{"items":["alpha","bravo","charlie"],"total":4},"meta":{"truncated":true,"next_offset":3,"count":3,"next_page_token":"tok-2"}}
	// <nil>
}

func ExampleMeta_nextPageToken() {
	env := output.Success([]string{"a", "b"}, &output.Meta{NextPageToken: "opaque-cursor"})
	_ = output.Write(os.Stdout, env, output.Options{})
	// Output:
	// {"ok":true,"data":["a","b"],"meta":{"truncated":false,"next_offset":null,"count":2,"next_page_token":"opaque-cursor"}}
}

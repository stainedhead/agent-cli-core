package docgen_test

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/stainedhead/agent-cli-core/docgen"
	"github.com/stainedhead/agent-cli-core/output"
)

func nestedTree() docgen.CommandTree {
	return docgen.CommandTree{
		Name:        "outlook",
		Description: "Read and send mail.",
		Commands: []docgen.Command{
			{
				Name:        "mail",
				Description: "Mail commands.",
				Usage:       "outlook mail <subcommand>",
				Subcommands: []docgen.Command{
					{Name: "send", Description: "Send a message.", Usage: "outlook mail send --to <addr>", Forbidden: []string{"never send without approval"}},
					{Name: "list", Usage: "outlook mail list", Examples: []string{"outlook mail list --folder inbox"},
						Subcommands: []docgen.Command{{Name: "unread", Usage: "outlook mail list unread"}}},
				},
			},
			{Name: "folder", Subcommands: []docgen.Command{{Name: "list"}}},
			{Name: "whoami", Usage: "outlook whoami"},
		},
	}
}

func TestNestedGolden(t *testing.T) {
	got, err := docgen.Generate(nestedTree())
	if err != nil {
		t.Fatal(err)
	}
	const path = "testdata/SKILL.nested.golden.md"
	if *update {
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("differs from %s (run go test ./docgen -update)\n%s", path, got)
	}
}

func TestNestedHeadingsAndOrder(t *testing.T) {
	out, err := docgen.Generate(nestedTree())
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	order := []string{"### folder\n", "#### folder list\n", "### mail\n", "#### mail list\n", "##### mail list unread\n", "#### mail send\n", "### whoami\n"}
	last := -1
	for _, h := range order {
		i := strings.Index(s, h)
		if i < 0 || i < last {
			t.Fatalf("heading %q missing or out of order (%d after %d)\n%s", h, i, last, s)
		}
		last = i
	}
	// A parent's own usage renders as well as its children.
	if !strings.Contains(s, "outlook mail <subcommand>") {
		t.Error("parent usage missing")
	}
}

func TestNestedOrderIndependent(t *testing.T) {
	a := nestedTree()
	b := nestedTree()
	m := b.Commands[0].Subcommands
	m[0], m[1] = m[1], m[0]
	b.Commands[0], b.Commands[2] = b.Commands[2], b.Commands[0]
	x, _ := docgen.Generate(a)
	y, _ := docgen.Generate(b)
	if !bytes.Equal(x, y) {
		t.Error("input order changes output")
	}
}

func TestNestedDoesNotMutateInput(t *testing.T) {
	tr := nestedTree()
	_, _ = docgen.Generate(tr)
	if tr.Commands[0].Subcommands[0].Name != "send" {
		t.Error("input subcommands were reordered")
	}
}

func TestNestedEmptySubcommandsIsLeaf(t *testing.T) {
	a, _ := docgen.Generate(docgen.CommandTree{Name: "t", Commands: []docgen.Command{{Name: "a", Usage: "t a", Subcommands: []docgen.Command{}}}})
	b, _ := docgen.Generate(docgen.CommandTree{Name: "t", Commands: []docgen.Command{{Name: "a", Usage: "t a"}}})
	if !bytes.Equal(a, b) {
		t.Error("empty Subcommands differs from leaf")
	}
}

func TestNestedValidation(t *testing.T) {
	deep := docgen.Command{Name: "l5"}
	for _, n := range []string{"l4", "l3", "l2", "l1"} {
		deep = docgen.Command{Name: n, Subcommands: []docgen.Command{deep}}
	}
	ok4 := docgen.Command{Name: "l4"}
	for _, n := range []string{"l3", "l2", "l1"} {
		ok4 = docgen.Command{Name: n, Subcommands: []docgen.Command{ok4}}
	}
	cases := map[string]docgen.CommandTree{
		"duplicate siblings": {Name: "t", Commands: []docgen.Command{{Name: "a", Subcommands: []docgen.Command{{Name: "x"}, {Name: " x "}}}}},
		"empty name":         {Name: "t", Commands: []docgen.Command{{Name: "a", Subcommands: []docgen.Command{{}}}}},
		"control char":       {Name: "t", Commands: []docgen.Command{{Name: "a", Subcommands: []docgen.Command{{Name: "b\nc"}}}}},
		"too deep":           {Name: "t", Commands: []docgen.Command{deep}},
	}
	for name, tr := range cases {
		_, err := docgen.Generate(tr)
		var ce output.CategoryError
		if err == nil || !errors.As(err, &ce) || ce.Category() != output.CategoryValidation {
			t.Errorf("%s: %v", name, err)
		}
	}
	// Same name under different parents is fine; depth 4 is the limit.
	if _, err := docgen.Generate(nestedTree()); err != nil {
		t.Error(err)
	}
	if _, err := docgen.Generate(docgen.CommandTree{Name: "t", Commands: []docgen.Command{ok4}}); err != nil {
		t.Errorf("depth 4: %v", err)
	}
}

// Subcommands are rendered as nested sections with the full command path.
func ExampleCommand_subcommands() {
	out, _ := docgen.Generate(docgen.CommandTree{
		Name: "outlook",
		Commands: []docgen.Command{{
			Name:        "mail",
			Subcommands: []docgen.Command{{Name: "send", Usage: "outlook mail send --to <addr>"}},
		}},
	})
	for _, l := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(l, "###") {
			fmt.Println(l)
		}
	}
	// Output:
	// ### mail
	// #### mail send
}

func TestSiblingOrderUsesTrimmedNames(t *testing.T) {
	gen := func(cmds []docgen.Command, nested bool) string {
		tr := docgen.CommandTree{Name: "t", Commands: cmds}
		if nested {
			tr.Commands = []docgen.Command{{Name: "p", Subcommands: cmds}}
		}
		out, err := docgen.Generate(tr)
		if err != nil {
			t.Fatal(err)
		}
		return string(out)
	}
	for _, nested := range []bool{false, true} {
		x := gen([]docgen.Command{{Name: " b"}, {Name: "a"}}, nested)
		y := gen([]docgen.Command{{Name: "a"}, {Name: " b"}}, nested)
		if x != y {
			t.Fatalf("output depends on input order (nested=%v)", nested)
		}
		ia, ib := strings.Index(x, " a\n"), strings.Index(x, " b\n")
		if ia < 0 || ib < 0 || ia > ib {
			t.Fatalf("a must precede b (nested=%v):\n%s", nested, x)
		}
	}
}

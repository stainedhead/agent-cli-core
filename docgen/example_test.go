package docgen_test

import (
	"fmt"
	"strings"

	"github.com/stainedhead/agent-cli-core/docgen"
)

// Generate a SKILL.md for a small tool and show its headline.
func ExampleGenerate() {
	out, err := docgen.Generate(docgen.CommandTree{
		Name:        "tracker",
		Description: "Read items in an issue tracker.",
		Commands: []docgen.Command{
			{Name: "get", Usage: "tracker get <id>"},
		},
	})
	if err != nil {
		fmt.Println(err)
		return
	}
	lines := strings.Split(string(out), "\n")
	fmt.Println(lines[1])
	fmt.Println(lines[5])
	// Output:
	// name: tracker
	// # tracker
}

// A command can list actions the agent must never take; they appear in the
// generated document.
func ExampleCommand_forbidden() {
	out, _ := docgen.Generate(docgen.CommandTree{
		Name: "tracker",
		Commands: []docgen.Command{
			{Name: "close", Forbidden: []string{"never bulk close"}},
		},
	})
	fmt.Println(strings.Contains(string(out), "- never bulk close"))
	// Output: true
}

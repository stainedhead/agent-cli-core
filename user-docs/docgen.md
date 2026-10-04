# docgen

Generate a deterministic SKILL.md for your CLI from a command tree. The document teaches an agent harness how to call your tool and how to read its output.

## Getting started

```go
tree := docgen.CommandTree{
	Name:        "tracker",
	Description: "Read and update items in an issue tracker.",
	Commands: []docgen.Command{
		{
			Name:     "get",
			Usage:    "tracker get <id>",
			Examples: []string{"tracker get ITEM-42"},
		},
		{
			Name:      "close",
			Usage:     "tracker close <id> --reason <text>",
			Forbidden: []string{"never bulk close"},
		},
	},
}
out, err := docgen.Generate(tree)
if err != nil {
	return err // validation category, exit 9
}
return os.WriteFile("SKILL.md", out, 0o644)
```

A good pattern is a hidden `docs` command that prints `Generate` output, plus a test that compares it with the checked-in SKILL.md.

## What you get

- Front matter with `name` and `description`.
- One section per command, sorted by name: description, usage, examples and a "Never" list from `Forbidden`. Top-level commands are `###` headings; nested commands appear depth-first under their parent with one more `#` per level and the full path as the title (see below).
- Always included, you cannot turn them off: the untrusted content rule, the envelope shape, the exit code table, and a pointer to the shared `agent-cli-core` skill (`docgen.SharedSkill`) for policy, credentials and output bounds.

The envelope examples and exit codes come from package `output` itself, so they stay correct when the library changes. Output has no timestamps and does not depend on the order of `Commands`.

## Nested commands (v0.2.0)

Give a command `Subcommands` to describe `mytool mail send` as a child of `mail`:

```go
docgen.Command{
	Name: "mail", Description: "Work with mail.",
	Subcommands: []docgen.Command{
		{Name: "send", Usage: "mytool mail send --to <addr>"},
		{Name: "list", Usage: "mytool mail list"},
	},
}
```

The parent renders its own text first, then its children sorted by name, as `### mail`, `#### mail list`, `#### mail send`. Nesting goes at most `docgen.MaxDepth` (4) levels deep. Sibling names must be unique after trimming spaces; the same name under different parents is fine. Names must be non-empty and free of control characters. `Usage` and `Examples` are your text, so write the full invocation in them. A command with no `Subcommands` renders exactly as before.

## Reference

| Field | Meaning |
|---|---|
| `CommandTree.Name` | Required. Letters, digits, `.`, `_`, `-`. |
| `CommandTree.Description` | Optional. Collapsed to one line. |
| `Command.Name` | Required, unique, single line. |
| `Command.Description` | Optional. Collapsed to one line. |
| `Command.Usage` | Synopsis, shown in a code block. |
| `Command.Examples` | Example invocations, order kept. |
| `Command.Forbidden` | Actions the agent must never take; sorted. |
| `Command.Subcommands` | Child commands, up to `docgen.MaxDepth` (4) levels. Names unique among siblings. |

## Troubleshooting

- `docgen: duplicate command "x"` (exit 9): command names must be unique among siblings; for nested commands the message shows the full path such as `mail send`.
- A nesting-depth error (exit 9): the tree is deeper than `docgen.MaxDepth`.
- Golden or checked-in file differs: regenerate it. For this library's own golden file run `go test ./docgen -update`, then review the diff.
- Do not put vendor-specific policy text in descriptions that belongs in the shared skill; link to it instead.

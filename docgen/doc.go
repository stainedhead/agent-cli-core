// Package docgen generates a deterministic harness skill document (SKILL.md)
// from a tool-supplied command tree. The output always includes the untrusted
// content rule, the envelope and the exit codes.
//
// The tool describes its commands with a CommandTree and calls Generate. The
// result is plain Markdown with a small front matter block: the same tree
// always yields the same bytes (commands are sorted by name, there are no
// timestamps), so the file can be checked in and compared in CI.
//
// The envelope example and the exit code table are built from the real
// definitions in package output, so the generated text cannot drift from the
// behavior. The document links to the shared conventions skill (SharedSkill)
// instead of repeating its policy, credential and output-bound guidance.
package docgen

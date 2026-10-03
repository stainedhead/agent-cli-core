# Implementation Notes - agent-cli-core
Date: 2026-10-03
Purpose: record decisions and surprises during implementation. Update after each task.

## Technical Decisions

### docgen
- `Generate` returns `*docgen.Error` (validation category) for an invalid tree, so `output.ExitOf` gives exit 9.
- Exit code meanings live in a map keyed by `output.Category`; a test asserts every `output.Categories()` entry has a row, so adding a category to `output` cannot silently drift.
- Envelope and untrusted examples are marshaled from `output` at generation time, not hand-written.
- `Command` has an extra optional `Description` field beyond the data dictionary; the dictionary fields are unchanged.
- User text is single-lined or fenced with a fence longer than any backtick run, to block heading/front-matter injection.
## Edge Cases and Solutions
## Deviations from Plan
## Lessons Learned

# Implementation Notes - agent-cli-core
Date: 2026-10-03
Purpose: record decisions and surprises during implementation. Update after each task.

## Technical Decisions
## Edge Cases and Solutions
## Deviations from Plan
## Lessons Learned

### audit
- Record carries `duration` as a Go duration string; encoding via explicit wire struct fixes column order for the golden test.
- Write mode is a Logger setting (warn|block), not per-verb: the library has no read/write classification. Tools wanting the PRD's "block write operations" default use two Loggers or choose per call. Assumption.
- Fields capped at 512 bytes and redacted so free text cannot smuggle bodies or tokens.
- Existing log files wider than 0600 are chmod-ed to 0600 on Open.

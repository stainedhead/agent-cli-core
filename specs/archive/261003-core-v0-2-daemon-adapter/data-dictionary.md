# core-v0-2-daemon-adapter - Data Dictionary

Purpose: new exported types and fields.

## Entities / Interfaces
- `oktad.Client`, `oktad.Option`, `oktad.TransientError{RetryAfter time.Duration; Err error}` (implements `output.CategoryError`, category rate_limited)
- `clock.Clock`, `clock.System`, `clock.Fake`
- `policy.TrustError{Path, Reason}`, `policy.TrustOption`

## Fields added
- `output.Meta.NextPageToken string` (`next_page_token`, omitempty)
- `output.Bounds.ArrayField string`
- `docgen.Command.Subcommands []Command`
- `audit.Record.Extra map[string]string`, `RuleID`, `TargetRef` (omitempty)
- `httpx.Config.VendorCodeFromBody func(status int, prefix []byte) string` (+ optional body-limit knob)

## Enumerations / API types
None new (no new categories or exit codes).

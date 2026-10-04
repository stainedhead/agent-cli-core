# ADR fragment: WS-C (clock, audit extension fields, trusted-file check, httpx body vendor code)

Status: proposed. Decisions D-C1..D-C6 of the core v0.2 spec, as implemented.

## R3: public clock (FR-007)
- New leaf package `clock` (Clock, System, Fake, NewFake) is the single implementation. `internal/clock` keeps its names as type aliases and a `NewFake` wrapper, so the types are identical and every internal import compiles unchanged.
- `audit` now imports the public `clock` directly so `audit.WithClock(c clock.Clock)` shows an importable type in docs. `policy.Clock` and `httpx.Clock` stay declared as they were; public types satisfy them structurally. Consumers (outlook 6/14, teams 7) can implement or reuse the clock.
- `audit.WithClock(nil)` keeps the system clock (previously a nil-pointer panic at the first Log).
- archtest: `clock` added as a leaf; `internal/clock` may import `clock`; `audit` may import `clock`.

## R5: audit Extra, RuleID, TargetRef (FR-009; D-C1, D-C2, D-C4)
- `Record` gains `RuleID`, `TargetRef` (omitempty strings) and `Extra *ExtraFields` (`type ExtraFields map[string]string`), written after `policy_decision` as `rule_id`, `target_ref`, `extra`.
- D-C4 (RB-1) resolved: a plain `map[string]string` field would make `Record` non-comparable, which breaks `==` for any caller and an existing v0.1.0 test already does that (`got != r`). The v0.2 release is additive-only, so `Extra` is a pointer to a named map type: `Record` stays comparable and callers write `Extra: &audit.ExtraFields{"k": "v"}`. A nil or empty Extra is omitted. Alternative rejected: a plain map field (incompatible per apidiff).
- D-C1: more than 16 keys (`MaxExtraKeys`) or a key outside `[a-z0-9_.-]{1,32}` is rejected: Log writes nothing and returns a `*WriteError` that matches both `ErrWrite` and the new `ErrInvalidExtra`. Values go through the redactor and are cut to 256 bytes (`MaxExtraValueLen`) on a rune boundary. `RuleID` and `TargetRef` use the existing redact+512-byte field cap. The caller's map is never mutated. Keys serialise sorted (encoding/json map order).
- D-C2: `SchemaVersion` stays 1 (additions are optional and omitempty; v0.1.0 output is byte-identical when unset). Readers must tolerate unknown keys.

## R6: standalone trusted-file check (FR-010; D-C5)
- `policy.CheckTrustedFile(path, ...TrustOption) error`, `policy.WithTrustedUIDs(uids ...uint32)`, `*policy.TrustError{Path, Reason, Err}`, `policy.ErrNotTrusted`. `TrustError` implements `output.CategoryError` (policy_denied, exit 6) and `Hinter`. This adds the `policy -> output` edge to the archtest allowed list (output does not import policy; no cycle).
- Trusted owners: root always, plus configured uids. The effective uid is never accepted unless it is root; passing it returns a `*TrustError` (the agent's own user can rewrite its own file). This satisfies outlook 15 and teams 2 ("root or a configured trusted uid, never the effective uid").
- Walk: the path is made absolute and walked component by component with `lstat`; symlinks are resolved by the check itself (owner of each link must be trusted, up to 40 hops, loops fail) and every directory on the real route, "/" included, must be owned by a trusted uid and not group/world writable (`mode&022`). D-C5 strict: a sticky world-writable ancestor such as `/tmp` is rejected.
- Final check of record: `open(O_RDONLY|O_NOFOLLOW|O_NONBLOCK)` then `fstat` on the descriptor (regular file, trusted owner, not group/world writable), plus `os.SameFile` against the earlier `lstat` so a swap between check and open fails. Non-regular files are rejected before opening (no FIFO hang).
- Everything fails closed: missing file, EACCES on an ancestor, non-unix platform (`trust_other.go`, build tag `!unix`, always returns a `*TrustError`). `Load` and `WritableMode` are untouched.
- Testability without root: the filesystem is behind a small unexported interface with a fake (arbitrary owners/modes, covers every branch), and the effective-uid lookup is a test seam so a non-root test user can play "trusted owner" on real files under `$HOME` (macOS and Linux). Real-filesystem tests skip if the home directory chain is not clean. No test needs chown or root.

## R7: httpx vendor code from body (FR-011; D-C3, D-C6)
- `Config.VendorCodeFromBody func(status int, prefix []byte) string` and `Config.VendorBodyLimit int` (default `DefaultVendorBodyLimit` 4096, cap `MaxVendorBodyLimit` 64 KiB). Applies to 403 only (D-C6); 401 and 429 keep their v0.1.0 typed errors. Precedence: a non-empty header `VendorCode` wins and the body is not read; otherwise the body prefix is read with `io.LimitReader`, the hook is called (skipped on a read error), then the body is drained and closed so connections are reused.
- The result is trimmed, control characters removed, scrubbed (redactor plus held credentials) and cut to 64 bytes on a rune boundary. D-C3: no body prefix is stored on any error type; only the hook sees the body. Hook panics are not recovered (documented). Zero-value Config behaviour is unchanged.
- Covers outlook 1/16/19 and teams 4. The extra "expose Status on typed errors" idea in those requests is already met (RateLimitedError.Status exists; AuthError/Forbidden carry their status by type).

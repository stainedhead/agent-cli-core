# API compatibility: v0.2.0 against v0.1.0

Result: **additions only; the only source-level caveat is unkeyed composite literals of the four extended structs (`output.Meta`, `output.Bounds`, `docgen.Command`, `audit.Record`), none in this repo or in snow-cli, outlook-cli and teams-cli at the time of writing (checked 2026-10-04 by grep for unkeyed literals of those types in the three consumers and in this repo).** One symbol is reported as "incompatible" by `apidiff` (audit.WithClock); it is a false positive explained below. Verified on 2026-10-03 against branch `feat/core-v0.2`.

## Method

A throwaway detached checkout of the `v0.1.0` tag (removed afterwards) and the v0.2 worktree, with `apidiff` from `golang.org/x/exp` (installed with `go install golang.org/x/exp/cmd/apidiff@latest`; go1.27.1). For each public package:

```
# in the v0.1.0 checkout
apidiff -w old-<pkg>.api ./<pkg>
# in the v0.2 worktree
apidiff -w new-<pkg>.api ./<pkg>
apidiff -incompatible old-<pkg>.api new-<pkg>.api
apidiff old-<pkg>.api new-<pkg>.api
```

Packages compared: audit, auth, auth/authtest, docgen, examples/sampletool, httpx, output, policy, selftest. New in v0.2 (no baseline, nothing to break): `clock`, `auth/oktad`. `internal/...` is not public API.

## Result per package

| Package | Incompatible | Added |
|---|---|---|
| audit | `WithClock` parameter type (false positive, below) | `ErrInvalidExtra`, `ExtraFields`, `MaxExtraKeyLen`, `MaxExtraKeys`, `MaxExtraValueLen`, `Record.Extra`, `Record.RuleID`, `Record.TargetRef` |
| auth | none | none |
| auth/authtest | none | none |
| docgen | none | `Command.Subcommands`, `MaxDepth` |
| examples/sampletool | none | none |
| httpx | none | `Config.VendorBodyLimit`, `Config.VendorCodeFromBody`, `DefaultVendorBodyLimit`, `MaxVendorBodyLimit` |
| output | none | `Bounds.ArrayField`, `ErrArrayField`, `Meta.NextPageToken` |
| policy | none | `CheckTrustedFile`, `ErrNotTrusted`, `TrustError`, `TrustOption`, `WithTrustedUIDs` |
| selftest | none | none |
| clock (new) | n/a | whole package: `Clock`, `System`, `Fake` and constructors |
| auth/oktad (new) | n/a | whole package: daemon adapter implementing `auth.DaemonClient` |

## The one flagged item: audit.WithClock

`apidiff` reports: `WithClock: changed from func(internal/clock.Clock) Option to func(clock.Clock) Option`.

In v0.1.0 the parameter type was declared in `internal/clock`, which no code outside the module can import or name. In v0.2 `internal/clock.Clock` is a type alias of the public `clock.Clock` (`type Clock = clock.Clock`), and both interfaces have the identical method set (`Now() time.Time`, `Sleep(ctx, d) error`). So every argument expression that compiled against v0.1.0 (any value with those two methods, or `clock.System`/`Fake` equivalents) still compiles. `apidiff` cannot see through the internal package boundary, hence the false positive. The downstream builds below confirm it. Note `record` comparability is kept too: `Record.Extra` is a pointer (`*ExtraFields`) so `Record` still supports `==`.

## Downstream verification

Copies of the `main` branch of each consumer (`git archive main`, no `.git`) in `$CLAUDE_JOB_DIR/tmp/downstream-<name>`, each pinned to core v0.1.0 in `go.mod`, with `replace github.com/stainedhead/agent-cli-core => <v0.2 worktree>` appended, then `go mod tidy`, `go build ./...`, `go vet ./...`, `go test ./...`. The originals were not touched.

| Consumer | build | vet | test |
|---|---|---|---|
| snow-cli | pass | pass | all pass except `internal/repocheck TestGoModNoReplaceNoPseudoVersion` |
| outlook-cli | pass | pass | all pass |
| teams-cli | pass | pass | all pass except `internal/archtest TestRepoGoMod` |

The two failures are guard tests that forbid a `replace` directive in `go.mod`; they fail only because this harness adds one. They are caused by the test method, not by the change: with those tests skipped (`go test -skip ...`) both packages pass, and the rest of snow-cli also passes under `-race`. There is no pre-existing failure and no failure caused by the core change. The CI `downstream` job (`.github/workflows/ci.yml`) uses a `go.work` file (`use ./core ./consumer`) instead of a `replace` directive, so the consumer's `go.mod` is untouched and these guard tests run and pass; verified locally on 2026-10-04 for all three consumers (build, vet, `go test -race`, no failures).

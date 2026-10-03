# user-docs

This directory holds only files that help a user adopt, configure and use `agent-cli-core`. The users are developers building a CLI on the library:

- how to add the dependency
- getting started
- per-package usage (`auth`, `policy`, `output`, `audit`, `httpx`, `selftest`, `docgen`)
- configuration reference
- usage examples
- troubleshooting

It is NOT for design, requirements, spec or process material, and it must not link into `specs/`. Those belong in `docs/`, `specs/` or the PRD.

## Status

Nothing is released yet: the library is a Draft PRD with no code and no tagged version, so there is no dependency to add and no package to use today. When the first release exists, a consumer will add it to `go.mod` at a released semver tag, for example:

```
go get github.com/stainedhead/agent-cli-core@vX.Y.Z
```

(`vX.Y.Z` is a placeholder; no version exists yet.) Per-package guides will be added here once there is something to use.

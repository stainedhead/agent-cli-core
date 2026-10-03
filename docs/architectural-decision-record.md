# Architectural decision record

Record architecture decisions here (context, decision, consequences), one entry per decision.

## Decisions made before code (from the PRD, section 2)

- The shared CLI core is its own repository, `agent-cli-core`, Go module `github.com/stainedhead/agent-cli-core`.
- It is a library: no binary, no container image.
- `snow`, `outlook` and `teams` remain separate binaries in separate repositories, each importing this module.
- Its `auth` package wraps `pkg/client` from `agent-okta-d`.
- It contains no vendor clients (no ServiceNow, Graph or Teams code).

Open items to record here once decided: where the human-mode PKCE and keychain token source lives, whether the module must compile for windows/amd64, ownership of the policy schema, and the conformance test kit (PRD section 13).

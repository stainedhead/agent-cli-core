# Intent

## Purpose
`agent-cli-core` is the shared Go library behind the agent-facing command-line tools of the
agentic-teams set: `snow` (ServiceNow), `outlook` (mail) and `teams` (Teams messaging). It holds the
behavior all three must share: getting a short-lived token from the `agent-okta-d` daemon, a client-side
policy engine, one response envelope with stable exit codes and untrusted-content marking, an audit
log, HTTP retry and redaction, a self-test runner and skill-document generation. It is a library: no
binary, no vendor clients.

**Status: implemented (PRD draft v0.1); no tagged release yet.**

### The wider context
The agentic-teammate project aims to let AI agents work as real teammates. Each agent runs inside
Hermes, or inside a CLI harness we provide, in a container built from the images in
`agentic-team-w-paperclip`. The Go tools in this set exist so that **Okta secures the agent's access
to the key tooling** it needs: AWS, GitHub, ServiceNow, Microsoft 365 and Atlassian.

This library's role is to keep the three CLIs consistent and thin. Without it each CLI would
re-implement token handling, retries, output shapes and the "never print a token" rule, and drift.

```
agent-okta-d (pkg/client)  <-  agent-cli-core  <-  snow, outlook, teams (separate binaries)
```

## Where this fits
Root map: [stainedhead/agentic-teams](https://github.com/stainedhead/agentic-teams).

| Repository | What it is | Relation to `agent-cli-core` |
|---|---|---|
| [agentic-team-w-paperclip](https://github.com/stainedhead/agentic-team-w-paperclip) | Container images with the Hermes, OMP and OpenCode harnesses, plus Paperclip | Hosts the agents that run the CLIs built on this library |
| [agent-okta-d](https://github.com/stainedhead/agent-okta-d) | Credential daemon; Okta OIDC is the identity root | Upstream dependency: this library wraps its `pkg/client` |
| [agent-cli-core](https://github.com/stainedhead/agent-cli-core) | This repository | Shared CLI core (library) |
| [snow-cli](https://github.com/stainedhead/snow-cli) | ServiceNow CLI (`snow`) | Consumer |
| [outlook-cli](https://github.com/stainedhead/outlook-cli) | Mail as the agent's own Entra user (`outlook`) | Consumer |
| [teams-cli](https://github.com/stainedhead/teams-cli) | Teams messaging as the agent's own Entra user (`teams`) | Consumer |

## Goals
- **One audited path from daemon to token** for every CLI, with the token never printed or logged.
- **One envelope and one set of exit codes (0-9)** so a harness skill describes every tool once.
- **Mark other people's free text as untrusted** the same way everywhere, and keep output bounded.
- **Provide guardrail layers**: client-side policy and an audit log.
- **Stay vendor-free**: the library knows nothing of ServiceNow, Graph or Teams.
- **Be adoptable safely**: semver tags, a small public API, breaking changes visible in the version.

## Non-goals
- **Vendor clients or command surfaces.** Those live in the tool repositories.
- **Being the security control.** Server-side permissions are the boundary; the policy engine is a
  guardrail.
- **A binary, a daemon or a container image.** The daemon is `agent-okta-d`.
- **Printing or exposing tokens** in any form.
- **A general-purpose CLI framework.**
- **Deploying or operating the agent fleet.**

## Status and caution
The library is implemented and tested, but **nothing is released anywhere**. It does not import
`agent-okta-d`: `auth` defines its own daemon interface, and an adapter over `pkg/client` is deferred
until `agent-okta-d` publishes a tagged release containing it.

Okta's reach differs by system. Okta directly gates AWS and ServiceNow, which accept its tokens.
GitHub and Microsoft 365 use the agent's own user account, gated by that account's state plus the
daemon's custody of its credential, and the Atlassian key is not bound to Okta. This library does not
change those guarantees. Items the PRD marks unconfirmed (the PRD's warning-sign items) are
assumptions to validate, not facts.

## Scope boundary in one line
> This library is the shared plumbing around a vendor call (token, retry, policy, output, audit),
> not the vendor client, the command surface or the lock on the door.

## How this file is used
INTENT.md captures *why* this library exists and where it sits in the set. The *how* lives in
[agent-cli-core-PRD.md](specs/archive/261003-agent-cli-core/agent-cli-core-PRD.md), and contributor rules in [AGENTS.md](AGENTS.md).
Update this file when goals, direction or scope shift, or when the set around it changes, not when
implementation details change. The PRD's evidence markers are authoritative; this file does not
upgrade any of them.

# Product summary

`agent-cli-core` is the shared Go library behind the agent-facing CLIs `snow`, `outlook` and `teams`. It provides the behavior those tools must share so that each stays thin: getting a short-lived token from the `agent-okta-d` daemon, client-side policy, one response envelope with stable exit codes and untrusted-content marking, an audit log, HTTP retry and redaction, a self-test runner and skill-document generation. It contains no vendor clients and no binary.

Source: [agent-cli-core-PRD.md](../agent-cli-core-PRD.md) (Draft v0.1, section 1).

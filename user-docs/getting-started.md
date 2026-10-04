# Getting started

This guide builds one command of a CLI the way the library intends: policy first, then token, HTTP, output and audit. Every identifier below exists in the library today.

## 1. Add the dependency

```
go get github.com/stainedhead/agent-cli-core@vX.Y.Z
```

`vX.Y.Z` is a placeholder: no version is tagged yet (see the [index](README.md)). The module needs Go 1.27 and pulls in two third-party modules, `github.com/goccy/go-yaml` (for `policy`) and, from v0.2.0, `github.com/stainedhead/agent-okta-d` (for `auth/oktad`).

## 2. Order of every command

1. `policy` decides whether the verb and resource are allowed. A denial never reaches the network and exits 6.
2. `auth` supplies the token; `httpx` uses it and refreshes once on a 401.
3. `output` writes one envelope.
4. `audit` records what happened.
5. The process exits with the envelope's code.

## 3. A complete command

```go
package main

import (
	"context"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/stainedhead/agent-cli-core/audit"
	"github.com/stainedhead/agent-cli-core/auth"
	"github.com/stainedhead/agent-cli-core/httpx"
	"github.com/stainedhead/agent-cli-core/output"
	"github.com/stainedhead/agent-cli-core/policy"
)

// denied gives a policy refusal the policy_denied category (exit 6).
type denied struct{ err error }

func (d denied) Error() string             { return d.err.Error() }
func (d denied) Unwrap() error             { return d.err }
func (d denied) Category() output.Category { return output.CategoryPolicyDenied }

func run(ctx context.Context, daemon auth.DaemonClient) output.ExitCode {
	out := os.Stdout

	// Invalid policy is fatal: report it as a validation error and stop.
	pol, err := policy.Load("/etc/mytool/policy.yaml", policy.WithWritable(policy.WritableRefuse))
	if err != nil {
		return emit(out, output.Failure(output.CategoryValidation, err.Error(), "fix the policy file"))
	}
	engine := policy.NewEngine(pol, nil)

	logger, err := audit.Open(audit.Config{Path: "/var/log/mytool/audit.jsonl", OnFailure: audit.Block})
	if err != nil {
		return emit(out, output.FromError(err))
	}
	defer logger.Close()

	src, err := auth.NewDaemonTokenSource(daemon, "my-provider",
		auth.WithRemediation("run `mytool enroll my-provider`"))
	if err != nil {
		return emit(out, output.FromError(err))
	}
	client := httpx.NewClient(httpx.Config{Refresher: auth.NewAuthorizer(src)})

	// 1. policy
	d := engine.Check(policy.Request{Verb: "get", Resource: "item"})
	if !d.Allowed {
		err := logger.Handle(audit.Record{Tool: "mytool", Verb: "get", Resource: "item/42",
			Outcome: "denied", PolicyDecision: string(d.Mode)}, denied{d.Err()})
		return emit(out, output.FromError(err))
	}

	// 2. HTTP with auth, retries and a single refresh on 401
	start := time.Now()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.example.test/items/42", nil)
	resp, err := client.Do(req)
	status := 0
	var data any
	if err == nil {
		defer resp.Body.Close()
		status = resp.StatusCode
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		// Free text written by other people is marked untrusted.
		data = map[string]any{"note": output.Untrusted{Value: string(body), Author: "upstream"}}
	}

	// 3. audit, then 4. output
	outcome := "ok"
	if err != nil {
		outcome = "error"
	}
	err = logger.Handle(audit.Record{Tool: "mytool", Verb: "get", Resource: "item/42",
		Outcome: outcome, HTTPStatus: status, Duration: time.Since(start),
		PolicyDecision: string(d.Mode)}, err)
	if err != nil {
		return emit(out, output.FromError(err))
	}
	return emit(out, output.Success(data, nil))
}

func emit(w io.Writer, env output.Envelope) output.ExitCode {
	if err := output.Write(w, env, output.Options{}); err != nil {
		env = output.FromError(err)
		_ = output.Write(w, env, output.Options{})
	}
	return env.ExitCode()
}

func main() { os.Exit(int(run(context.Background(), myDaemonAdapter{}))) }
```

`myDaemonAdapter` stands for the daemon client: use `oktad.New()` from `auth/oktad` (see [oktad](oktad.md)) or your own type implementing `auth.DaemonClient` (`Fetch` and `Refresh`); see [auth](auth.md). A runnable, vendor-neutral version of this flow, with fakes, is in the library repository under `examples/sampletool`.

## 4. Things to remember

- Most policy errors do not carry an output category (only `*policy.TrustError` does); you map a refusal to `policy_denied` as above, and an invalid policy to `validation`.
- A 404 or 409 comes back from `httpx` as an ordinary response. Map it yourself to `output.CategoryNotFound` or `output.CategoryConflict` with a small error type implementing `Category()`.
- Wrap free text written by other people in `output.Untrusted`; the library cannot guess which fields they are.
- Expose paging by adding a flag such as `--offset` that fills `output.Options.Bounds.Offset`, and `--max-bytes` for `Bounds.MaxBytes`. The library defines no flags.
- Never print a token. There is no helper that returns one.

## Next

[Configuration reference](configuration.md), [usage examples](examples.md) and the per-package guides linked from the [index](README.md).

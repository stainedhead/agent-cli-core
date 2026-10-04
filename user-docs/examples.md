# Usage examples

Short recipes. Each package guide has more detail; [getting started](getting-started.md) shows them combined.

## Return a result and exit with the right code

```go
env := output.Success(items, &output.Meta{RequestID: reqID})
if err != nil {
	env = output.FromError(err)
}
_ = output.Write(os.Stdout, env, output.Options{Format: output.FormatJSON})
os.Exit(int(env.ExitCode()))
```

## Give your own error an exit code and a hint

```go
type notFound struct{ id string }

func (e notFound) Error() string              { return "item " + e.id + " was not found" }
func (e notFound) Category() output.Category { return output.CategoryNotFound }
func (e notFound) Hint() string               { return "check the identifier" }
```

`output.ExitOf(err)` returns 5 for this error even when it is wrapped.

## Page through large output

```go
opts := output.Options{Bounds: output.Bounds{MaxBytes: maxBytes, Offset: offsetFlag}}
// The envelope's meta.next_offset is the value to pass as --offset next time;
// meta.truncated is false on the last page.
```

## Mark free text as untrusted

```go
data := map[string]any{
	"number":  "T-1001",
	"comment": output.Untrusted{Value: body, Author: "carol", Timestamp: when},
}
```

## Check policy before acting

```go
p, err := policy.Load(path, policy.WithWritable(policy.WritableRefuse))
if err != nil { /* fatal: exit 9 */ }
e := policy.NewEngine(p, nil)
d := e.Check(policy.Request{Verb: "create", Resource: "record", Fields: map[string]any{"title": "x"}})
switch {
case d.Allowed:      // perform the action
case d.DryRunOnly(): // preview only
default:             // d.Err() is a *policy.DeniedError: report exit 6; d.RuleID and d.Reason go to the audit record
}
```

A minimal policy:

```yaml
version: 1
rules:
  - id: read
    effect: allow
    verbs: [get, list]
    resources: ["*"]
  - id: create-record
    effect: allow
    mode: dry_run_only
    verbs: [create]
    resources: [record]
    fields: [title]
```

## Get a token source from the credential daemon

```go
c := oktad.New() // socket from AGENT_OKTA_D_SOCKET or the platform default
defer c.Close()
src, err := auth.NewDaemonTokenSource(c, "my-provider",
	auth.WithRemediation("run `mytool enroll my-provider`"))
authz := auth.NewAuthorizer(src)
client := httpx.NewClient(httpx.Config{Refresher: authz})
```

## Test your CLI without a daemon

```go
f := authtest.New(authtest.UnauthorizedThenSuccess)
srv := httptest.NewServer(f.Handler())
defer srv.Close()
authz := auth.NewAuthorizer(mustSource(auth.NewDaemonTokenSource(f, "test")))
client := httpx.NewClient(httpx.Config{Refresher: authz, Clock: instantClock{}})
// f.Refreshes() == 1 after one 401 followed by success.
```

where `instantClock` has `Now()` and a `Sleep` that returns at once.

## Retry a POST that carries an idempotency key

```go
req = httpx.MarkSafeToRetry(req)
resp, err := client.Do(req)
```

## Audit an action

```go
l, _ := audit.Open(audit.Config{Path: path, OnFailure: audit.Block})
defer l.Close()
err = l.Handle(audit.Record{Tool: "mytool", Verb: "read", Resource: "item/1",
	Outcome: "ok", HTTPStatus: 200, PolicyDecision: "allow"}, actionErr)
```

## Run a self-test

```go
res, err := selftest.Runner{Rows: rows, Probe: probe, ReadOnly: readOnlyFlag}.Run(ctx)
if err != nil { os.Exit(int(output.ExitOf(err))) } // 2: bad matrix
code, _ := res.Write(os.Stdout, output.Options{})
os.Exit(int(code)) // 0 pass, 1 any failing row
```

## Generate and check SKILL.md

```go
out, err := docgen.Generate(docgen.CommandTree{
	Name:        "mytool",
	Description: "Read items.",
	Commands:    []docgen.Command{{Name: "get", Usage: "mytool get <id>", Examples: []string{"mytool get 42"}}},
})
```

Compare `out` with your checked-in file in a test so the document never drifts.

## Read a retry hint from the daemon adapter

```go
_, err := c.Fetch(ctx, "my-provider")
var te *oktad.TransientError
if errors.As(err, &te) {
	time.Sleep(te.RetryAfter()) // exit 8; zero means the daemon gave no hint
}
```

## Page a list that sits inside an object

```go
data := struct {
	Items []Item `json:"items"`
	Total int    `json:"total"`
}{items, total}
env := output.Success(data, &output.Meta{NextPageToken: vendorToken}) // token set by your tool
err := output.Write(os.Stdout, env, output.Options{
	Bounds: output.Bounds{MaxBytes: 8192, Offset: offset, ArrayField: "items"},
})
// "total" is untouched; meta.count is the items kept; meta.next_offset resumes.
```

## Nested commands in SKILL.md

```go
tree := docgen.CommandTree{Name: "mytool", Commands: []docgen.Command{{
	Name: "mail", Description: "Work with mail.",
	Subcommands: []docgen.Command{
		{Name: "send", Usage: "mytool mail send --to <addr>", Examples: []string{"mytool mail send --to a@example.com"}},
		{Name: "list", Usage: "mytool mail list"},
	},
}}}
```

## Audit with a rule id and extra fields

```go
err := l.Log(audit.Record{
	Tool: "mytool", Verb: "create", Resource: "item/9", Outcome: "ok",
	RuleID: decision.RuleID, TargetRef: "item/9",
	Extra: &audit.ExtraFields{"batch": "7", "dry_run": "false"},
})
```

## Refuse an untrusted policy file

```go
if err := policy.CheckTrustedFile("/etc/mytool/policy.yaml"); err != nil {
	fmt.Fprintln(os.Stderr, output.FromError(err).Error.Message)
	os.Exit(int(output.ExitOf(err))) // 6: policy_denied
}
p, err := policy.Load("/etc/mytool/policy.yaml")
```

## Derive a vendor code from a 403 body

```go
cfg := httpx.Config{
	VendorCodeFromBody: func(status int, prefix []byte) string {
		var v struct{ Code string `json:"code"` }
		if json.Unmarshal(prefix, &v) != nil {
			return ""
		}
		return v.Code
	},
}
```

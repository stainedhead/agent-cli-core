package sampletool

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/stainedhead/agent-cli-core/audit"
	"github.com/stainedhead/agent-cli-core/httpx"
	"github.com/stainedhead/agent-cli-core/output"
	"github.com/stainedhead/agent-cli-core/policy"
	"github.com/stainedhead/agent-cli-core/selftest"
)

// maxBody bounds how much of a response the tool reads.
const maxBody = 1 << 20

// Config wires a Tool. Everything with behavior is injected so tests can use
// fakes; a real main builds the same Config from flags and files.
type Config struct {
	// Name is the executable name, used in audit records.
	Name string
	// BaseURL is the upstream service root.
	BaseURL string
	// Policy decides every action before anything else happens.
	Policy *policy.Engine
	// Authorizer authorizes requests and refreshes once on a 401. An
	// *auth.Authorizer satisfies it without an adapter.
	Authorizer httpx.TokenRefresher
	// HTTP configures retries, clock and tracing. Refresher is set by New.
	HTTP httpx.Config
	// Audit receives one record per action. Nil disables auditing.
	Audit *audit.Logger
	// Out receives the envelope.
	Out io.Writer
	// Format is the output format; empty means JSON.
	Format output.Format
	// Secrets are literal values to scrub from error text.
	Secrets []string
	// RunID groups the audit records of one run.
	RunID string
	// ExtraSelftestRows adds a row that is expected to fail, so tests can
	// check that a failing matrix exits 1.
	ExtraSelftestRows bool
}

// Tool is the sample command runner.
type Tool struct {
	cfg    Config
	client *http.Client
}

// New returns a Tool. It sets the HTTP layer's TokenRefresher from
// cfg.Authorizer.
func New(cfg Config) *Tool {
	h := cfg.HTTP
	h.Refresher = cfg.Authorizer
	return &Tool{cfg: cfg, client: httpx.NewClient(h)}
}

// statusError is a failure the tool derives from an upstream status.
type statusError struct {
	cat    output.Category
	status int
}

func (e *statusError) Error() string { return fmt.Sprintf("upstream answered status %d", e.status) }

func (e *statusError) Category() output.Category { return e.cat }

// categorized attaches a category to an error from a package that does not
// import output (policy).
type categorized struct {
	cat output.Category
	err error
}

func (e *categorized) Error() string { return e.err.Error() }

func (e *categorized) Unwrap() error { return e.err }

func (e *categorized) Category() output.Category { return e.cat }

// Run executes one command and returns the process exit code. The envelope is
// already written to Config.Out.
func (t *Tool) Run(ctx context.Context, args []string) output.ExitCode {
	if len(args) == 0 {
		return t.fail(&categorized{output.CategoryUsage, errors.New("usage: " + t.cfg.Name + " get|create|selftest")})
	}
	switch args[0] {
	case "get":
		return t.get(ctx, args[1:])
	case "create":
		return t.create(ctx, args[1:])
	case "delete":
		return t.deleteItem(args[1:])
	case "selftest":
		return t.selftest(ctx, args[1:])
	default:
		return t.fail(&categorized{output.CategoryUsage, fmt.Errorf("unknown command %q", args[0])})
	}
}

func (t *Tool) emit(env output.Envelope) output.ExitCode {
	_ = output.Write(t.cfg.Out, env, output.Options{Format: t.cfg.Format, Secrets: t.cfg.Secrets})
	return env.ExitCode()
}

func (t *Tool) fail(err error) output.ExitCode { return t.emit(output.FromError(err)) }

// record writes the audit record and returns the error the command should
// act on (a failed write in block mode joins the action's error).
func (t *Tool) record(verb, resource, outcome, decision string, status int, err error) error {
	if t.cfg.Audit == nil {
		return err
	}
	return t.cfg.Audit.Handle(audit.Record{
		Tool: t.cfg.Name, RunID: t.cfg.RunID, Verb: verb, Resource: resource,
		Outcome: outcome, HTTPStatus: status, PolicyDecision: decision,
	}, err)
}

// check runs the policy step. A refusal is a policy_denied error.
func (t *Tool) check(verb string, fields map[string]any) (policy.Decision, error) {
	d := t.cfg.Policy.Check(policy.Request{Verb: verb, Resource: "item", Fields: fields})
	if d.Allowed || d.Mode == policy.ModeDryRunOnly {
		return d, nil
	}
	return d, &categorized{output.CategoryPolicyDenied, &policy.DeniedError{Decision: d}}
}

func (t *Tool) deleteItem(args []string) output.ExitCode {
	if len(args) != 1 {
		return t.fail(&categorized{output.CategoryUsage, errors.New("usage: delete <id>")})
	}
	d, err := t.check("delete", nil)
	err = t.record("delete", "item/"+args[0], "denied", string(d.Mode), 0, err)
	return t.fail(err)
}

func (t *Tool) get(ctx context.Context, args []string) output.ExitCode {
	if len(args) != 1 {
		return t.fail(&categorized{output.CategoryUsage, errors.New("usage: get <id>")})
	}
	id := args[0]
	d, err := t.check("get", nil)
	if err != nil {
		return t.fail(t.record("get", "item/"+id, "denied", string(d.Mode), 0, err))
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, t.cfg.BaseURL+"/items/"+url.PathEscape(id), nil)
	if err != nil {
		return t.fail(err)
	}
	data, status, err := t.do(req)
	outcome := "ok"
	if err != nil {
		outcome = "error"
	}
	err = t.record("get", "item/"+id, outcome, string(d.Mode), status, err)
	if err != nil {
		return t.fail(err)
	}
	return t.emit(output.Success(data, nil))
}

func (t *Tool) create(ctx context.Context, args []string) output.ExitCode {
	fs := flag.NewFlagSet("create", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	priority := fs.Int("priority", 1, "priority 1..3")
	if len(args) == 0 {
		return t.fail(&categorized{output.CategoryUsage, errors.New("usage: create <title> [--priority N]")})
	}
	title := args[0]
	if err := fs.Parse(args[1:]); err != nil {
		return t.fail(&categorized{output.CategoryUsage, err})
	}
	fields := map[string]any{"title": title, "priority": *priority}
	d, err := t.check("create", fields)
	if err != nil {
		return t.fail(t.record("create", "item", "denied", string(d.Mode), 0, err))
	}
	if d.Mode == policy.ModeDryRunOnly {
		err = t.record("create", "item", "dry_run", string(d.Mode), 0, nil)
		if err != nil {
			return t.fail(err)
		}
		return t.emit(output.Success(map[string]any{"dry_run": true, "would_create": fields}, nil))
	}
	body, _ := json.Marshal(fields)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.cfg.BaseURL+"/items", bytes.NewReader(body))
	if err != nil {
		return t.fail(err)
	}
	req.Header.Set("Content-Type", "application/json")
	data, status, err := t.do(req)
	outcome := "ok"
	if err != nil {
		outcome = "error"
	}
	if err = t.record("create", "item", outcome, string(d.Mode), status, err); err != nil {
		return t.fail(err)
	}
	return t.emit(output.Success(data, nil))
}

// item is the sample upstream record. Description and Title are free text
// written by other people, so they are marked untrusted.
type item struct {
	ID          string           `json:"id"`
	Title       output.Untrusted `json:"title"`
	Description output.Untrusted `json:"description"`
}

// do sends req through httpx and maps the answer to data or a categorized
// error. httpx's own typed errors (auth, forbidden, rate limited) pass through.
func (t *Tool) do(req *http.Request) (any, int, error) {
	resp, err := t.client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = resp.Body.Close() }()
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return nil, resp.StatusCode, &statusError{output.CategoryNotFound, resp.StatusCode}
	case resp.StatusCode == http.StatusConflict:
		return nil, resp.StatusCode, &statusError{output.CategoryConflict, resp.StatusCode}
	case resp.StatusCode >= 400:
		return nil, resp.StatusCode, &statusError{output.CategoryGeneral, resp.StatusCode}
	}
	var raw struct {
		ID          string `json:"id"`
		Title       string `json:"title"`
		Description string `json:"description"`
		Author      string `json:"author"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxBody)).Decode(&raw); err != nil {
		return nil, resp.StatusCode, &statusError{output.CategoryGeneral, resp.StatusCode}
	}
	return item{
		ID:          raw.ID,
		Title:       output.Untrusted{Value: raw.Title, Author: raw.Author},
		Description: output.Untrusted{Value: raw.Description, Author: raw.Author},
	}, resp.StatusCode, nil
}

// selfTestRows is the sample matrix: what the policy must allow and deny.
func (t *Tool) selfTestRows() []selftest.Row {
	rows := []selftest.Row{
		{Name: "read item", Verb: "get", Resource: "item", Expect: selftest.Allow, ReadOnly: true},
		{Name: "delete is refused", Verb: "delete", Resource: "item", Expect: selftest.Deny, ReadOnly: true},
		{Name: "create item", Verb: "create", Resource: "item", Expect: selftest.Allow},
		{Name: "unknown resource is refused", Verb: "get", Resource: "other", Expect: selftest.Deny, ReadOnly: true},
	}
	if t.cfg.ExtraSelftestRows {
		rows = append(rows, selftest.Row{Name: "deliberate mismatch", Verb: "delete", Resource: "item", Expect: selftest.Allow, ReadOnly: true})
	}
	return rows
}

func (t *Tool) selftest(ctx context.Context, args []string) output.ExitCode {
	readOnly := false
	for _, a := range args {
		if a == "--read-only" {
			readOnly = true
		}
	}
	pol := t.cfg.Policy.Policy()
	probe := func(_ context.Context, row selftest.Row) (selftest.Outcome, error) {
		fields := map[string]any(nil)
		if row.Verb == "create" {
			fields = map[string]any{"title": "t", "priority": 1}
		}
		// Evaluate is pure, so the matrix consumes no rate budget.
		if pol.Evaluate(policy.Request{Verb: row.Verb, Resource: row.Resource, Fields: fields}).Mode == policy.ModeDeny {
			return selftest.Deny, nil
		}
		return selftest.Allow, nil
	}
	res, err := selftest.Runner{Rows: t.selfTestRows(), Probe: probe, ReadOnly: readOnly}.Run(ctx)
	if err != nil {
		return t.fail(err)
	}
	return t.emit(res.Envelope())
}

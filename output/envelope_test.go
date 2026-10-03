package output_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/stainedhead/agent-cli-core/audit"
	"github.com/stainedhead/agent-cli-core/output"
)

type hintErr struct{}

func (hintErr) Error() string             { return "denied by rule r1" }
func (hintErr) Category() output.Category { return output.CategoryPolicyDenied }
func (hintErr) Hint() string              { return "ask a human to approve" }

func TestSuccessAndFailureShapes(t *testing.T) {
	s := output.Success(map[string]int{"a": 1}, &output.Meta{Count: 1, RequestID: "r-1"})
	if !s.OK || s.ExitCode() != output.ExitOK {
		t.Fatalf("%+v", s)
	}
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"ok":true,"data":{"a":1},"meta":{"truncated":false,"next_offset":null,"count":1,"request_id":"r-1"}}`
	if string(b) != want {
		t.Fatalf("got %s", b)
	}
	f := output.Failure(output.CategoryNotFound, "item not found", "check the id")
	b, _ = json.Marshal(f)
	want = `{"ok":false,"error":{"code":"not_found","message":"item not found","hint":"check the id"}}`
	if string(b) != want {
		t.Fatalf("got %s", b)
	}
	if f.ExitCode() != output.ExitNotFound {
		t.Fatalf("exit %d", f.ExitCode())
	}
}

func TestExitCodeOfMalformedEnvelope(t *testing.T) {
	if got := (output.Envelope{}).ExitCode(); got != output.ExitGeneral {
		t.Fatalf("got %d", got)
	}
}

func TestRenderDoesNotEscapeHTML(t *testing.T) {
	b, err := output.Render(output.Success("a<b>&c", nil), output.Options{})
	if err != nil || !strings.Contains(string(b), `"a<b>&c"`) {
		t.Fatalf("got %s", b)
	}
}

func TestFromError(t *testing.T) {
	if e := output.FromError(nil); !e.OK {
		t.Fatal("nil error must be success")
	}
	e := output.FromError(fmt.Errorf("wrap: %w", hintErr{}))
	if e.OK || e.Error.Code != output.CategoryPolicyDenied || e.Error.Hint != "ask a human to approve" {
		t.Fatalf("%+v", e.Error)
	}
	if e.ExitCode() != output.ExitPolicyDenied {
		t.Fatal("exit mismatch")
	}
	g := output.FromError(errors.New("plain"))
	if g.Error.Code != output.CategoryGeneral || g.Error.Hint != "" {
		t.Fatalf("%+v", g.Error)
	}
}

func TestFromErrorRedactsMessageAndHint(t *testing.T) {
	e := output.Failure(output.CategoryAuth, "refresh failed: Bearer abc123def", "retry with token=hunter2 later")
	if strings.Contains(e.Error.Message, "abc123def") || strings.Contains(e.Error.Hint, "hunter2") {
		t.Fatalf("leak: %+v", e.Error)
	}
	e = output.FromError(errors.New("boom client_secret=s3cr3t"))
	if strings.Contains(e.Error.Message, "s3cr3t") {
		t.Fatalf("leak: %+v", e.Error)
	}
}

func ExampleFromError() {
	env := output.FromError(hintErr{})
	b, _ := json.Marshal(env)
	fmt.Println(string(b))
	fmt.Println(env.ExitCode())
	// Output:
	// {"ok":false,"error":{"code":"policy_denied","message":"denied by rule r1","hint":"ask a human to approve"}}
	// 6
}

func TestFailureWithSecretsScrubsLiteral(t *testing.T) {
	env := output.FailureWithSecrets(output.CategoryAuth, "bad key abc-123-xyz", "retry with abc-123-xyz", "abc-123-xyz")
	if strings.Contains(env.Error.Message, "abc-123-xyz") || strings.Contains(env.Error.Hint, "abc-123-xyz") {
		t.Fatalf("leak: %+v", env.Error)
	}
	// Plain Failure cannot know the literal; Write with Secrets is the second line.
	plain := output.Failure(output.CategoryAuth, "bad key abc-123-xyz", "")
	b := mustRender(t, plain, output.Options{Secrets: []string{"abc-123-xyz"}})
	if strings.Contains(string(b), "abc-123-xyz") {
		t.Fatalf("Write did not rescrub: %s", b)
	}
	b = mustRender(t, env, output.Options{})
	if strings.Contains(string(b), "abc-123-xyz") {
		t.Fatalf("leak on constructed path: %s", b)
	}
}

func TestFromErrorWithSecretsScrubsLiteralAndKeepsHint(t *testing.T) {
	err := fmt.Errorf("call failed for abc-123-xyz: %w", hinted{})
	env := output.FromErrorWithSecrets(err, "abc-123-xyz")
	if strings.Contains(env.Error.Message, "abc-123-xyz") || env.Error.Hint != "try again" {
		t.Fatalf("env: %+v", env.Error)
	}
	if got := output.FromErrorWithSecrets(nil, "abc-123-xyz"); !got.OK {
		t.Fatal("nil must be success")
	}
}

type hinted struct{}

func (hinted) Error() string { return "hinted" }
func (hinted) Hint() string  { return "try again" }

func TestHintFoundThroughJoinedErrors(t *testing.T) {
	werr := &audit.WriteError{Err: errors.New("disk full")}
	cases := map[string]error{
		"join action then audit": errors.Join(hintErr{}, werr),
		"join audit then action": errors.Join(werr, hintErr{}),
		"wrapped join":           fmt.Errorf("run: %w", errors.Join(werr, fmt.Errorf("x: %w", hintErr{}))),
	}
	for name, err := range cases {
		env := output.FromError(err)
		if env.Error.Hint != "ask a human to approve" {
			t.Errorf("%s: hint = %q", name, env.Error.Hint)
		}
	}
	if h := output.FromError(errors.Join(werr)).Error.Hint; h != "" {
		t.Errorf("no hinter: hint = %q", h)
	}
}

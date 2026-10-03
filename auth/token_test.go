package auth_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"github.com/stainedhead/agent-cli-core/auth"
)

const secret = "s3cr3t-value-123456"

func TestTokenNeverPrintsValue(t *testing.T) {
	tok := auth.NewToken(secret)
	for _, verb := range []string{"%v", "%+v", "%#v", "%s", "%q", "%x", "%d", "%T%v"} {
		for _, arg := range []any{tok, &tok, []auth.Token{tok}, struct{ T auth.Token }{tok}, map[string]auth.Token{"k": tok}} {
			if got := fmt.Sprintf(verb, arg); strings.Contains(got, secret) {
				t.Fatalf("verb %s leaked: %s", verb, got)
			}
		}
	}
	if got := fmt.Sprintf("%v", tok); got != "[redacted]" {
		t.Fatalf("got %q", got)
	}
	if tok.String() != "[redacted]" || tok.GoString() != "[redacted]" {
		t.Fatal("String/GoString must redact")
	}
	if fmt.Sprint(tok) != "[redacted]" || fmt.Sprintf("%+v", tok) != "[redacted]" {
		t.Fatal("Sprint and plus-v must redact")
	}
}

func TestTokenMarshalRedacts(t *testing.T) {
	tok := auth.NewToken(secret)
	b, err := json.Marshal(struct{ T auth.Token }{tok})
	if err != nil || strings.Contains(string(b), secret) || !strings.Contains(string(b), "[redacted]") {
		t.Fatalf("json: %s %v", b, err)
	}
	txt, err := tok.MarshalText()
	if err != nil || string(txt) != "[redacted]" {
		t.Fatalf("text: %s %v", txt, err)
	}
	j, err := tok.MarshalJSON()
	if err != nil || string(j) != `"[redacted]"` {
		t.Fatalf("json: %s %v", j, err)
	}
}

func TestTokenSlogRedacts(t *testing.T) {
	var buf bytes.Buffer
	l := slog.New(slog.NewTextHandler(&buf, nil))
	tok := auth.NewToken(secret)
	l.Info("x", "tok", tok, slog.Any("t2", &tok))
	jl := slog.New(slog.NewJSONHandler(&buf, nil))
	jl.Info("x", "tok", tok)
	if strings.Contains(buf.String(), secret) || !strings.Contains(buf.String(), "[redacted]") {
		t.Fatalf("log: %s", buf.String())
	}
}

func TestTokenIsZero(t *testing.T) {
	var z auth.Token
	if !z.IsZero() || auth.NewToken("").IsZero() == false || auth.NewToken("a").IsZero() {
		t.Fatal("IsZero")
	}
	if z.String() != "[redacted]" {
		t.Fatal("zero token must still redact")
	}
}

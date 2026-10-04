package oktad

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stainedhead/agent-cli-core/auth"
	"github.com/stainedhead/agent-cli-core/output"
	"github.com/stainedhead/agent-okta-d/pkg/client"
)

func TestMapErrorDaemonVerdictBeatsLateCancel(t *testing.T) {
	c := New(WithSocketPath("/tmp/none"))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := c.mapError(ctx, "graph", client.ErrRevoked)
	if !errors.Is(err, auth.ErrRevoked) || output.CategoryOf(err) != output.CategoryAuth {
		t.Fatalf("got %v", err)
	}
	// A context error with a cancelled context is still a context error.
	err = c.mapError(ctx, "graph", context.Canceled)
	if !errors.Is(err, context.Canceled) || output.CategoryOf(err) != output.CategoryGeneral {
		t.Fatalf("got %v", err)
	}
	dctx, dcancel := context.WithTimeout(context.Background(), 0)
	defer dcancel()
	err = c.mapError(dctx, "graph", context.DeadlineExceeded)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("got %v", err)
	}
}

func TestGetRejectsEmptyCredential(t *testing.T) {
	c := New(WithSocketPath("/tmp/none"))
	_, err := c.get(context.Background(), "graph", func(*client.Client) (client.Credential, error) {
		return client.Credential{}, nil
	})
	if err == nil || output.CategoryOf(err) != output.CategoryGeneral ||
		!strings.Contains(err.Error(), "oktad: daemon returned an empty credential") {
		t.Fatalf("got %v", err)
	}
}

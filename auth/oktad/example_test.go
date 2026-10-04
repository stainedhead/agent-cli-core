package oktad_test

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/stainedhead/agent-cli-core/auth"
	"github.com/stainedhead/agent-cli-core/auth/oktad"
	"github.com/stainedhead/agent-cli-core/output"
	"github.com/stainedhead/agent-okta-d/pkg/client/clienttest"
)

// exampleT adapts examples (which have no *testing.T) to clienttest.New.
type exampleT struct{}

func (exampleT) Helper()                   {}
func (exampleT) Fatalf(f string, a ...any) { panic(fmt.Sprintf(f, a...)) }
func (exampleT) Cleanup(func())            {}

func ExampleNew() {
	srv := clienttest.New(exampleT{})
	defer srv.Close()
	srv.SetCredential("graph", clienttest.Credential{TokenType: "Bearer", AccessToken: "tok", ExpiresAt: time.Now().Add(time.Hour)})

	// Real code calls oktad.New() with no options: the socket then comes from
	// AGENT_OKTA_D_SOCKET or the platform default.
	c := oktad.New(oktad.WithSocketPath(srv.SocketPath()))
	defer c.Close()

	src, err := auth.NewDaemonTokenSource(c, "graph")
	if err != nil {
		fmt.Println(err)
		return
	}
	tok, err := src.Token(context.Background())
	fmt.Println(tok, err)
	// Output: [redacted] <nil>
}

func ExampleTransientError() {
	srv := clienttest.New(exampleT{})
	defer srv.Close()
	srv.SetProviderError("graph", clienttest.Error{Code: clienttest.CodeDegraded, RetryAfter: 30 * time.Second})
	c := oktad.New(oktad.WithSocketPath(srv.SocketPath()))
	defer c.Close()

	_, err := c.Fetch(context.Background(), "graph")
	var te *oktad.TransientError
	if errors.As(err, &te) {
		fmt.Println(output.CategoryOf(err), output.ExitFor(output.CategoryOf(err)), te.RetryAfter())
	}
	// Output: rate_limited 8 30s
}

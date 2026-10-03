package auth_test

import (
	"context"
	"fmt"
	"net/http"

	"github.com/stainedhead/agent-cli-core/auth"
	"github.com/stainedhead/agent-cli-core/auth/authtest"
	"github.com/stainedhead/agent-cli-core/output"
)

func ExampleToken() {
	tok := auth.NewToken("not-a-real-token")
	fmt.Println(tok)
	fmt.Printf("%v %+v %#v\n", tok, tok, tok)
	// Output:
	// [redacted]
	// [redacted] [redacted] [redacted]
}

func ExampleAuthorizer() {
	// In a real CLI the DaemonClient is an adapter over the credential
	// daemon's client; here the fake stands in. The provider name comes from
	// the tool, never from this library.
	daemon := authtest.New(authtest.Valid)
	src, _ := auth.NewDaemonTokenSource(daemon, "my-provider")
	authz := auth.NewAuthorizer(src)

	req, _ := http.NewRequest(http.MethodGet, "https://example.invalid/items", nil)
	if err := authz.Authorize(context.Background(), req); err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(req.Header.Get("Authorization") != "")
	// Output: true
}

func ExampleUnreachableError() {
	daemon := authtest.New(authtest.Unreachable, authtest.WithSocket("/run/example/daemon.sock"))
	src, _ := auth.NewDaemonTokenSource(daemon, "my-provider")
	_, err := src.Token(context.Background())
	fmt.Println(err)
	fmt.Println("exit:", output.ExitOf(err))
	// Output:
	// credential daemon unreachable at socket "/run/example/daemon.sock": the agent-okta-d service may not be running: dial unix /run/example/daemon.sock: connect: no such file or directory
	// exit: 3
}

func ExampleWithRemediation() {
	daemon := authtest.New(authtest.ReauthRequired)
	src, _ := auth.NewDaemonTokenSource(daemon, "my-provider",
		auth.WithRemediation("run `mytool enroll my-provider` as the signed-in user"))
	_, err := src.Token(context.Background())
	env := output.FromError(err)
	fmt.Println(env.Error.Code)
	fmt.Println(env.Error.Hint)
	// Output:
	// auth
	// A human action is needed: the credential must be re-enrolled. run `mytool enroll my-provider` as the signed-in user
}

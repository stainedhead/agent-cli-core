// Package sampletool is a vendor-neutral sample of a CLI built on
// agent-cli-core. It is documentation that compiles and is tested: it shows
// the order every command follows and how the packages meet.
//
// The flow of one command is policy -> auth -> httpx -> output -> audit:
//
//  1. policy.Engine.Check decides whether the verb and resource are allowed.
//     A denial never reaches the network and exits 6.
//  2. auth.Authorizer is handed to httpx as its TokenRefresher. The bridge is
//     structural: *auth.Authorizer has exactly the methods of
//     httpx.TokenRefresher, so no adapter code is needed. The token goes only
//     into the Authorization header.
//  3. httpx retries transient failures, honors Retry-After and refreshes the
//     token once on a 401.
//  4. output writes one envelope; free text written by others is wrapped in
//     output.Untrusted and the process exit code agrees with the envelope.
//  5. audit.Logger.Handle records what happened, never a body or a token.
//
// The tool itself never imports an internal package; tests inject a fake
// clock, a fake credential daemon and an httptest server.
package sampletool

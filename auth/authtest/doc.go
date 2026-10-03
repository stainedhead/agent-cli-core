// Package authtest provides a fake auth.DaemonClient for tests of CLIs built
// on agent-cli-core, plus a fake resource server that returns 401 as the
// scenario dictates. It is test support only and is never needed in a
// production binary. It performs no real network I/O except through the
// httptest servers the caller starts, and holds no real credentials.
package authtest

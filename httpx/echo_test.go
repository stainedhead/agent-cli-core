package httpx_test

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stainedhead/agent-cli-core/httpx"
)

type fixedAuth struct{ value string }

func (f fixedAuth) Authorize(_ context.Context, r *http.Request) error {
	r.Header.Set("Authorization", "Bearer "+f.value)
	return nil
}
func (f fixedAuth) Refresh(context.Context) error { return nil }

// A server that echoes the credential it was sent in headers the redactor does
// not treat as sensitive must not get it into a trace or an error.
func TestEchoedCredentialNeverTracedOrReported(t *testing.T) {
	const cred = "zq9xk2mw7plain" // short and unpatterned: only the value match can catch it
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		echo := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		w.Header().Set("X-Vendor-Code", echo)
		w.Header().Set("X-Request-Id", echo)
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	var trace bytes.Buffer
	c := httpx.NewClient(httpx.Config{
		Refresher:  fixedAuth{cred},
		Trace:      &trace,
		VendorCode: func(h http.Header) string { return h.Get("X-Vendor-Code") },
	})
	_, err := c.Get(srv.URL)
	var fe *httpx.ForbiddenError
	if !errors.As(err, &fe) {
		t.Fatalf("err = %v", err)
	}
	if strings.Contains(err.Error(), cred) || strings.Contains(trace.String(), cred) {
		t.Fatalf("credential leaked:\nerr=%v\ntrace=%s", err, trace.String())
	}
}

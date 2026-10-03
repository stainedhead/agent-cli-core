package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stainedhead/agent-cli-core/audit"
	"github.com/stainedhead/agent-cli-core/auth"
	"github.com/stainedhead/agent-cli-core/examples/sampletool"
	"github.com/stainedhead/agent-cli-core/httpx"
	"github.com/stainedhead/agent-cli-core/internal/clock"
	"github.com/stainedhead/agent-cli-core/policy"
)

const testPolicy = `
version: 1
limits: {max_results: 25, max_bytes: 4096}
rules:
  - id: read
    effect: allow
    verbs: [get]
    resources: [item]
  - id: write-item
    effect: allow
    verbs: [create]
    resources: [item]
    fields: [title, priority]
    constraints:
      priority: {min: 1, max: 3}
  - id: never-delete
    effect: deny
    verbs: [delete]
    resources: ["*"]
`

// rig is one wired sample tool over fakes: fake clock, httptest server, fake
// daemon, in-memory audit and trace buffers.
type rig struct {
	tool  *sampletool.Tool
	out   *bytes.Buffer
	audit *bytes.Buffer
	trace *bytes.Buffer
	clk   *clock.Fake
	srv   *httptest.Server
	hits  *serverState
}

type serverState struct {
	mu    sync.Mutex
	paths []string
}

func (s *serverState) add(p string) { s.mu.Lock(); s.paths = append(s.paths, p); s.mu.Unlock() }
func (s *serverState) count() int   { s.mu.Lock(); defer s.mu.Unlock(); return len(s.paths) }

// newRig builds the tool. handler serves the fake upstream; daemon is the fake
// credential daemon.
func newRig(t *testing.T, daemon auth.DaemonClient, handler http.Handler, opts ...func(*sampletool.Config)) *rig {
	t.Helper()
	st := &serverState{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		st.add(r.Method + " " + r.URL.Path)
		handler.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)

	pol, err := policy.Parse([]byte(testPolicy))
	if err != nil {
		t.Fatal(err)
	}
	clk := clock.NewFake(time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC))
	clk.SetAutoAdvance(true)

	src, err := auth.NewDaemonTokenSource(daemon, "sample-provider",
		auth.WithRemediation("run `sample enroll` as the signed-in user"))
	if err != nil {
		t.Fatal(err)
	}
	r := &rig{out: &bytes.Buffer{}, audit: &bytes.Buffer{}, trace: &bytes.Buffer{}, clk: clk, srv: srv, hits: st}
	cfg := sampletool.Config{
		Name:       "sample",
		BaseURL:    srv.URL,
		Policy:     policy.NewEngine(pol, clk),
		Authorizer: auth.NewAuthorizer(src),
		HTTP:       httpx.Config{Clock: clk, Rand: func() float64 { return 0.5 }, Trace: r.trace},
		Audit:      audit.NewLogger(r.audit, audit.WithClock(clk)),
		Out:        r.out,
		RunID:      "run-1",
	}
	for _, o := range opts {
		o(&cfg)
	}
	r.tool = sampletool.New(cfg)
	return r
}

func (r *rig) run(args ...string) (int, map[string]any) {
	r.out.Reset()
	code := r.tool.Run(context.Background(), args)
	var env map[string]any
	if err := json.Unmarshal(r.out.Bytes(), &env); err != nil {
		panic("output is not one JSON envelope: " + err.Error() + ": " + r.out.String())
	}
	return int(code), env
}

func (r *rig) auditLines() []string {
	s := strings.TrimSpace(r.audit.String())
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

func clockStart() time.Time { return time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC) }

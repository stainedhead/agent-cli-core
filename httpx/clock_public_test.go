package httpx_test

import (
	"testing"

	"github.com/stainedhead/agent-cli-core/clock"
	"github.com/stainedhead/agent-cli-core/httpx"
)

// The public clock types satisfy httpx.Clock.
func TestConfigAcceptsPublicClock(t *testing.T) {
	cfg := httpx.Config{Clock: clock.System{}}
	cfg.Clock = clock.NewFake(fakeClock().Now())
	if cfg.Clock == nil {
		t.Fatal("nil")
	}
}

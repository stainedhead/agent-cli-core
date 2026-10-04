package clock_test

import (
	"testing"
	"time"

	pub "github.com/stainedhead/agent-cli-core/clock"
	internal "github.com/stainedhead/agent-cli-core/internal/clock"
)

func pubNow(c pub.Clock) time.Time            { return c.Now() }
func internalNow(c internal.Clock) time.Time  { return c.Now() }
func pubFake(f *pub.Fake) time.Time           { return f.Now() }
func internalFake(f *internal.Fake) time.Time { return f.Now() }

// The internal names must be the same types as the public ones, so a value
// built through either path is accepted by the other (compile-time check).
func TestAliasesAreIdentical(t *testing.T) {
	at := time.Unix(7, 0)
	if !pubFake(internal.NewFake(at)).Equal(at) || !internalFake(pub.NewFake(at)).Equal(at) {
		t.Fatal("fake mismatch")
	}
	if pubNow(internal.System{}).IsZero() || internalNow(pub.System{}).IsZero() {
		t.Fatal("system mismatch")
	}
}

// Package clock is the internal name of the library's clock. The types now
// live in the public package github.com/stainedhead/agent-cli-core/clock;
// these aliases keep every internal import compiling with identical types.
package clock

import (
	"time"

	"github.com/stainedhead/agent-cli-core/clock"
)

// Clock is an alias of the public clock.Clock.
type Clock = clock.Clock

// System is an alias of the public clock.System.
type System = clock.System

// Fake is an alias of the public clock.Fake.
type Fake = clock.Fake

// NewFake returns a Fake whose current time is start.
func NewFake(start time.Time) *Fake { return clock.NewFake(start) }

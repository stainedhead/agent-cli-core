//go:build !unix

package policy_test

import (
	"errors"
	"testing"

	"github.com/stainedhead/agent-cli-core/policy"
)

func TestUnsupportedPlatformFailsClosed(t *testing.T) {
	if err := policy.CheckTrustedFile("anything"); !errors.Is(err, policy.ErrNotTrusted) {
		t.Fatalf("%v", err)
	}
}

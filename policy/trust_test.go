package policy_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stainedhead/agent-cli-core/output"
	"github.com/stainedhead/agent-cli-core/policy"
)

func TestTrustErrorContract(t *testing.T) {
	err := policy.CheckTrustedFile("")
	var te *policy.TrustError
	if !errors.As(err, &te) || !errors.Is(err, policy.ErrNotTrusted) {
		t.Fatalf("%v", err)
	}
	if output.CategoryOf(err) != output.CategoryPolicyDenied {
		t.Fatalf("category = %v", output.CategoryOf(err))
	}
	var h output.Hinter = te
	if h.Hint() == "" || te.Error() == "" {
		t.Fatal("empty hint or message")
	}
}

func TestMissingFileFailsClosed(t *testing.T) {
	err := policy.CheckTrustedFile(filepath.Join(t.TempDir(), "absent.yaml"))
	if !errors.Is(err, policy.ErrNotTrusted) {
		t.Fatalf("%v", err)
	}
}

func TestOwnEffectiveUIDIsRejectedAsTrusted(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() <= 0 {
		t.Skip("needs a non-root POSIX user")
	}
	err := policy.CheckTrustedFile("/etc/hosts", policy.WithTrustedUIDs(uint32(os.Geteuid())))
	var te *policy.TrustError
	if !errors.As(err, &te) {
		t.Fatalf("%v", err)
	}
}

func TestNilOptionIgnored(t *testing.T) {
	if err := policy.CheckTrustedFile("", nil, policy.WithTrustedUIDs()); err == nil {
		t.Fatal("expected error")
	}
}

func ExampleCheckTrustedFile() {
	// A policy file must be owned by root (or a configured trusted uid) and
	// sit in directories only they can change.
	err := policy.CheckTrustedFile("/nonexistent/policy.yaml", policy.WithTrustedUIDs(0))
	fmt.Println(errors.Is(err, policy.ErrNotTrusted))
	// Output: true
}

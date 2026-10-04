//go:build unix

package policy_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stainedhead/agent-cli-core/policy"
)

// realTrustDir makes a scratch directory under the home directory, outside
// the sticky, world-writable system temp dir that the check rejects.
func realTrustDir(t *testing.T) string {
	t.Helper()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home dir")
	}
	d, err := os.MkdirTemp(home, ".trusttest-*")
	if err != nil {
		t.Skipf("cannot create scratch dir under home: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(d) })
	return d
}

// A file owned by the current user is exactly what the check must reject when
// the current user is not trusted.
func TestRealFileOwnedByAgentUserRejected(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root owns everything it creates")
	}
	d := realTrustDir(t)
	p := filepath.Join(d, "p.yaml")
	if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := policy.CheckTrustedFile(p)
	var te *policy.TrustError
	if !errors.As(err, &te) {
		t.Fatalf("want *TrustError, got %v", err)
	}
}

func TestRealSymlinkOwnedByAgentUserRejected(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root owns everything it creates")
	}
	d := realTrustDir(t)
	l := filepath.Join(d, "link")
	if err := os.Symlink("/etc/hosts", l); err != nil {
		t.Fatal(err)
	}
	if err := policy.CheckTrustedFile(l); !errors.Is(err, policy.ErrNotTrusted) {
		t.Fatalf("%v", err)
	}
}

func TestRelativePathResolved(t *testing.T) {
	d := realTrustDir(t)
	if err := os.WriteFile(filepath.Join(d, "p.yaml"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	old, _ := os.Getwd()
	defer func() { _ = os.Chdir(old) }()
	if err := os.Chdir(d); err != nil {
		t.Fatal(err)
	}
	// Owned by the agent user (or root): either way an error or success, but
	// the relative path must reach the file, not report "cannot stat".
	err := policy.CheckTrustedFile("p.yaml")
	var te *policy.TrustError
	if errors.As(err, &te) && te.Err != nil && errors.Is(te.Err, os.ErrNotExist) {
		t.Fatalf("relative path not resolved: %v", err)
	}
}

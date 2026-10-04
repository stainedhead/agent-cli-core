//go:build unix

package policy

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// realDir creates a scratch directory under the home directory (the system
// temp dir is sticky and world-writable, which the check rejects), and
// pretends the process runs as a different user so that the current uid can
// play the trusted owner. It works as a non-root user on Linux and macOS.
// openStatDir returns a writable scratch directory anywhere on disk. openStat
// does not walk ancestors, so it needs no clean home chain and the O_NOFOLLOW
// and SameFile paths are exercised on every runner.
func openStatDir(t *testing.T) string {
	t.Helper()
	d, err := os.MkdirTemp("", "ocd")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(d) })
	return d
}

// skipRealFS skips the test and says so loudly in the log, so a CI runner whose
// home chain is unsuitable shows up as a visible skip rather than silence.
func skipRealFS(t *testing.T, format string, args ...any) {
	t.Helper()
	t.Skipf("SKIPPED real-filesystem trust test: "+format, args...)
}

func realDir(t *testing.T) (string, trustConfig) {
	t.Helper()
	home, err := os.UserHomeDir()
	if err != nil {
		skipRealFS(t, "no home directory")
	}
	d, err := os.MkdirTemp(home, ".trusttest-*")
	if err != nil {
		skipRealFS(t, "cannot create scratch dir under home: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(d) })
	d, err = filepath.EvalSymlinks(d)
	if err != nil {
		t.Fatal(err)
	}
	withEUID(t, os.Getuid()+100000)
	cfg := cfgOf(uint32(os.Getuid()))
	probe := filepath.Join(d, "probe")
	if err := os.WriteFile(probe, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := checkTrustedFile(probe, cfg); err != nil {
		skipRealFS(t, "home directory chain is not clean enough: %v", err)
	}
	return d, cfg
}

func TestRealFilesystem(t *testing.T) {
	d, cfg := realDir(t)
	write := func(name string, mode os.FileMode) string {
		p := filepath.Join(d, name)
		if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(p, mode); err != nil {
			t.Fatal(err)
		}
		return p
	}
	good := write("good", 0o640)
	if err := checkTrustedFile(good, cfg); err != nil {
		t.Fatalf("good file: %v", err)
	}
	if err := checkTrustedFile(good, cfgOf()); err == nil || !strings.Contains(err.Error(), "not trusted") {
		t.Fatalf("not-trusted uid must fail: %v", err)
	}
	if err := checkTrustedFile(write("gw", 0o660), cfg); err == nil || !strings.Contains(err.Error(), "writable by group or others") {
		t.Fatalf("group writable: %v", err)
	}
	if err := checkTrustedFile(write("ww", 0o602), cfg); err == nil {
		t.Fatal("world writable must fail")
	}

	// A symlink owned by a trusted uid to a clean file is followed.
	l := filepath.Join(d, "link")
	if err := os.Symlink(good, l); err != nil {
		t.Fatal(err)
	}
	if err := checkTrustedFile(l, cfg); err != nil {
		t.Fatalf("link: %v", err)
	}
	// ...but not when the link owner is untrusted.
	if err := checkTrustedFile(l, cfgOf()); err == nil {
		t.Fatal("untrusted link owner must fail")
	}
	// A symlink loop fails closed.
	loop := filepath.Join(d, "loop")
	if err := os.Symlink(loop, loop); err != nil {
		t.Fatal(err)
	}
	if err := checkTrustedFile(loop, cfg); err == nil {
		t.Fatal("loop must fail")
	}
	// A directory is not a file; a group-writable directory is rejected.
	if err := checkTrustedFile(d, cfg); err == nil {
		t.Fatal("directory must fail")
	}
	sub := filepath.Join(d, "sub")
	if err := os.Mkdir(sub, 0o700); err != nil {
		t.Fatal(err)
	}
	inner := filepath.Join(sub, "p")
	if err := os.WriteFile(inner, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := checkTrustedFile(inner, cfg); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(sub, 0o770); err != nil {
		t.Fatal(err)
	}
	if err := checkTrustedFile(inner, cfg); err == nil || !strings.Contains(err.Error(), "writable by group or others") {
		t.Fatalf("writable ancestor: %v", err)
	}
	// Special file is rejected without opening it.
	// Relative path.
	old, _ := os.Getwd()
	defer func() { _ = os.Chdir(old) }()
	if err := os.Chdir(d); err != nil {
		t.Fatal(err)
	}
	if err := checkTrustedFile("good", cfg); err != nil {
		t.Fatalf("relative: %v", err)
	}
}

func TestOSTrustFSOpenStat(t *testing.T) {
	d := openStatDir(t)
	p := filepath.Join(d, "f")
	if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	var fsys osTrustFS
	if _, err := fsys.openStat(p); err != nil {
		t.Fatal(err)
	}
	if _, err := fsys.openStat(filepath.Join(d, "absent")); err == nil {
		t.Fatal("absent must fail")
	}
	// O_NOFOLLOW: opening a symlink path directly must fail.
	l := filepath.Join(d, "l")
	if err := os.Symlink(p, l); err != nil {
		t.Fatal(err)
	}
	if _, err := fsys.openStat(l); err == nil {
		t.Fatal("O_NOFOLLOW must refuse a symlink")
	}
	if _, err := fsys.lstat(filepath.Join(d, "absent")); err == nil {
		t.Fatal("absent lstat must fail")
	}
	if _, err := fsys.readlink(p); err == nil {
		t.Fatal("readlink of a regular file must fail")
	}
}

func TestOSTrustFSOpenUnreadable(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root reads anything")
	}
	d := openStatDir(t)
	p := filepath.Join(d, "f")
	if err := os.WriteFile(p, []byte("x"), 0o000); err != nil {
		t.Fatal(err)
	}
	var fsys osTrustFS
	if _, err := fsys.openStat(p); !errors.Is(err, os.ErrPermission) {
		t.Fatalf("%v", err)
	}
}

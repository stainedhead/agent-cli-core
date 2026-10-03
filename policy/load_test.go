package policy_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stainedhead/agent-cli-core/policy"
)

func write(t *testing.T, dir string, mode os.FileMode, content string) string {
	t.Helper()
	path := filepath.Join(dir, "policy.yaml")
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadErrors(t *testing.T) {
	if _, err := policy.Load(filepath.Join(t.TempDir(), "missing.yaml")); err == nil {
		t.Fatal("missing file must fail")
	}
	path := write(t, t.TempDir(), 0o644, "version: 1\nbogus: true\n")
	p, err := policy.Load(path)
	var ie *policy.InvalidError
	if p != nil || !errors.As(err, &ie) {
		t.Fatalf("want fail closed, got %v %v", p, err)
	}
}

func TestLoadWritableFile(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can write everything")
	}
	dir := t.TempDir()
	path := write(t, dir, 0o644, good)

	p, err := policy.Load(path)
	if err != nil || len(p.Warnings()) != 2 {
		t.Fatalf("warn mode: %v %v", err, p.Warnings())
	}
	if !strings.Contains(p.Warnings()[0], "writable by the current user") {
		t.Errorf("warning text: %v", p.Warnings())
	}

	_, err = policy.Load(path, policy.WithWritable(policy.WritableRefuse))
	var we *policy.WritableError
	if !errors.As(err, &we) || we.What != "file" {
		t.Fatalf("refuse: %v", err)
	}

	p, err = policy.Load(path, policy.WithWritable(policy.WritableIgnore))
	if err != nil || p.Warnings() != nil {
		t.Fatalf("ignore: %v %v", err, p.Warnings())
	}
}

func TestLoadReadOnlyLocation(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can write everything")
	}
	dir := t.TempDir()
	path := write(t, dir, 0o444, good)
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
	p, err := policy.Load(path, policy.WithWritable(policy.WritableRefuse))
	if err != nil || len(p.Warnings()) != 0 {
		t.Fatalf("read-only location must load clean: %v %v", err, p.Warnings())
	}
}

func TestLoadWritableDirectoryOnly(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can write everything")
	}
	dir := t.TempDir()
	path := write(t, dir, 0o444, good)
	_, err := policy.Load(path, policy.WithWritable(policy.WritableRefuse))
	var we *policy.WritableError
	if !errors.As(err, &we) || we.What != "directory" {
		t.Fatalf("got %v", err)
	}
}

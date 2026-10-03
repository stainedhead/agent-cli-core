//go:build unix

package policy

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestWritableByMeErrors(t *testing.T) {
	old := accessFn
	defer func() { accessFn = old }()
	for _, e := range []error{syscall.EACCES, syscall.EROFS, syscall.EPERM} {
		accessFn = func(string, uint32) error { return e }
		if w, err := writableByMe("x"); w || err != nil {
			t.Errorf("%v: %v %v", e, w, err)
		}
	}
	boom := errors.New("boom")
	accessFn = func(string, uint32) error { return boom }
	if _, err := writableByMe("x"); !errors.Is(err, boom) {
		t.Errorf("want boom, got %v", err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "p.yaml")
	if err := os.WriteFile(path, []byte("version: 1\nrules:\n - {id: a, effect: allow, verbs: [a], resources: [b]}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Error("check error must fail Load")
	}
}

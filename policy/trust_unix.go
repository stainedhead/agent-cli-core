//go:build unix

package policy

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// maxSymlinkHops bounds symlink resolution, so a loop fails instead of
// spinning.
const maxSymlinkHops = 40

// euidFn is replaced in tests.
var euidFn = os.Geteuid

// trustMeta is what the check needs to know about a path.
type trustMeta struct {
	uid  uint32
	mode fs.FileMode
}

// trustFS is the filesystem seen by the check; tests substitute a fake with
// arbitrary owners.
type trustFS interface {
	lstat(path string) (trustMeta, error)
	readlink(path string) (string, error)
	// openStat opens path with O_NOFOLLOW, fstats the descriptor, confirms it
	// is the same file lstat saw, and closes it.
	openStat(path string) (trustMeta, error)
}

func checkTrustedFile(path string, cfg trustConfig) error {
	return checkTrustedFS(osTrustFS{}, path, cfg)
}

func checkTrustedFS(fsys trustFS, path string, cfg trustConfig) error {
	if path == "" {
		return &TrustError{Path: path, Reason: "empty path"}
	}
	if e := euidFn(); e != 0 && cfg.uids[uint32(e)] {
		return &TrustError{Path: path, Reason: fmt.Sprintf("uid %d is the effective uid of this process and cannot be a trusted owner", e)}
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return &TrustError{Path: path, Reason: "cannot make the path absolute", Err: err}
	}

	// Walk component by component, resolving symlinks ourselves so that every
	// directory and every link on the real route is inspected.
	pending := splitPath(abs)
	cur := string(filepath.Separator)
	if err := checkDir(fsys, cur, cfg); err != nil {
		return err
	}
	hops := 0
	for len(pending) > 0 {
		comp := pending[0]
		pending = pending[1:]
		switch comp {
		case "", ".":
			continue
		case "..":
			cur = filepath.Dir(cur)
			continue
		}
		next := filepath.Join(cur, comp)
		m, err := fsys.lstat(next)
		if err != nil {
			return &TrustError{Path: next, Reason: "cannot stat", Err: err}
		}
		switch {
		case m.mode&fs.ModeSymlink != 0:
			if !cfg.uids[m.uid] {
				return &TrustError{Path: next, Reason: fmt.Sprintf("symlink owned by uid %d, which is not trusted", m.uid)}
			}
			if hops++; hops > maxSymlinkHops {
				return &TrustError{Path: next, Reason: "too many levels of symbolic links"}
			}
			target, err := fsys.readlink(next)
			if err != nil {
				return &TrustError{Path: next, Reason: "cannot read symlink", Err: err}
			}
			if filepath.IsAbs(target) {
				cur = string(filepath.Separator)
			}
			pending = append(splitPath(target), pending...)
		case len(pending) == 0:
			return checkFile(fsys, next, m, cfg)
		case m.mode.IsDir():
			if err := checkMeta(next, m, cfg); err != nil {
				return err
			}
			cur = next
		default:
			return &TrustError{Path: next, Reason: "not a directory"}
		}
	}
	// The path resolved to a directory (for example "/" or a trailing "..").
	return &TrustError{Path: abs, Reason: "not a regular file"}
}

func splitPath(p string) []string { return strings.Split(p, string(filepath.Separator)) }

func checkDir(fsys trustFS, dir string, cfg trustConfig) error {
	m, err := fsys.lstat(dir)
	if err != nil {
		return &TrustError{Path: dir, Reason: "cannot stat", Err: err}
	}
	return checkMeta(dir, m, cfg)
}

// checkMeta applies the owner and mode rules shared by directories and the
// file.
func checkMeta(path string, m trustMeta, cfg trustConfig) error {
	if !cfg.uids[m.uid] {
		return &TrustError{Path: path, Reason: fmt.Sprintf("owned by uid %d, which is not trusted", m.uid)}
	}
	if m.mode.Perm()&0o022 != 0 {
		return &TrustError{Path: path, Reason: fmt.Sprintf("writable by group or others (mode %04o)", m.mode.Perm())}
	}
	return nil
}

func checkFile(fsys trustFS, path string, pre trustMeta, cfg trustConfig) error {
	if !pre.mode.IsRegular() {
		return &TrustError{Path: path, Reason: "not a regular file"}
	}
	m, err := fsys.openStat(path)
	if err != nil {
		return &TrustError{Path: path, Reason: "cannot open", Err: err}
	}
	if !m.mode.IsRegular() {
		return &TrustError{Path: path, Reason: "not a regular file"}
	}
	return checkMeta(path, m, cfg)
}

// osTrustFS is the real filesystem.
type osTrustFS struct{}

func metaOf(fi fs.FileInfo) (trustMeta, error) {
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return trustMeta{}, errors.New("no ownership information")
	}
	return trustMeta{uid: st.Uid, mode: fi.Mode()}, nil
}

func (osTrustFS) lstat(path string) (trustMeta, error) {
	fi, err := os.Lstat(path)
	if err != nil {
		return trustMeta{}, err
	}
	return metaOf(fi)
}

func (osTrustFS) readlink(path string) (string, error) { return os.Readlink(path) }

func (osTrustFS) openStat(path string) (trustMeta, error) {
	before, err := os.Lstat(path)
	if err != nil {
		return trustMeta{}, err
	}
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return trustMeta{}, err
	}
	defer func() { _ = f.Close() }()
	fi, err := f.Stat()
	if err != nil {
		return trustMeta{}, err
	}
	if !os.SameFile(before, fi) {
		return trustMeta{}, errors.New("file changed while being checked")
	}
	return metaOf(fi)
}

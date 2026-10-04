//go:build unix

package policy

import (
	"errors"
	"io/fs"
	"strings"
	"testing"
)

// fakeFS is an in-memory trustFS with arbitrary owners and modes.
type fakeFS struct {
	entries map[string]trustMeta
	links   map[string]string
	openErr error
	openAs  *trustMeta // overrides what openStat reports (a swapped file)
	lstErr  map[string]error
}

func (f *fakeFS) lstat(p string) (trustMeta, error) {
	if err := f.lstErr[p]; err != nil {
		return trustMeta{}, err
	}
	m, ok := f.entries[p]
	if !ok {
		return trustMeta{}, fs.ErrNotExist
	}
	return m, nil
}

func (f *fakeFS) readlink(p string) (string, error) {
	t, ok := f.links[p]
	if !ok {
		return "", errors.New("readlink failed")
	}
	return t, nil
}

func (f *fakeFS) openStat(p string) (trustMeta, error) {
	if f.openErr != nil {
		return trustMeta{}, f.openErr
	}
	if f.openAs != nil {
		return *f.openAs, nil
	}
	return f.entries[p], nil
}

const (
	dir755  = fs.ModeDir | 0o755
	file644 = fs.FileMode(0o644)
	link    = fs.ModeSymlink | 0o777
)

func baseFS() *fakeFS {
	return &fakeFS{
		entries: map[string]trustMeta{
			"/":              {0, dir755},
			"/etc":           {0, dir755},
			"/etc/agent":     {0, dir755},
			"/etc/agent/p.y": {0, file644},
		},
		links:  map[string]string{},
		lstErr: map[string]error{},
	}
}

func cfgOf(uids ...uint32) trustConfig {
	c := trustConfig{uids: map[uint32]bool{0: true}}
	for _, u := range uids {
		c.uids[u] = true
	}
	return c
}

func withEUID(t *testing.T, e int) {
	t.Helper()
	old := euidFn
	euidFn = func() int { return e }
	t.Cleanup(func() { euidFn = old })
}

func TestCheckTrustedFS(t *testing.T) {
	withEUID(t, 1000)
	type tc struct {
		name   string
		mut    func(*fakeFS)
		path   string
		cfg    trustConfig
		reason string // substring; "" means success
	}
	cases := []tc{
		{name: "root owned", path: "/etc/agent/p.y", cfg: cfgOf()},
		{name: "unclean path", path: "/etc/./agent/../agent/p.y", cfg: cfgOf()},
		{name: "configured trusted uid owns all", cfg: cfgOf(500), path: "/etc/agent/p.y",
			mut: func(f *fakeFS) {
				for k := range f.entries {
					m := f.entries[k]
					m.uid = 500
					f.entries[k] = m
				}
			}},
		{name: "untrusted file owner", path: "/etc/agent/p.y", cfg: cfgOf(), reason: "owned by uid 1000",
			mut: func(f *fakeFS) { f.entries["/etc/agent/p.y"] = trustMeta{1000, file644} }},
		{name: "untrusted ancestor owner", path: "/etc/agent/p.y", cfg: cfgOf(), reason: "owned by uid 1000",
			mut: func(f *fakeFS) { f.entries["/etc/agent"] = trustMeta{1000, dir755} }},
		{name: "untrusted root dir", path: "/etc/agent/p.y", cfg: cfgOf(), reason: "owned by uid 7",
			mut: func(f *fakeFS) { f.entries["/"] = trustMeta{7, dir755} }},
		{name: "group writable file", path: "/etc/agent/p.y", cfg: cfgOf(), reason: "writable by group or others (mode 0664)",
			mut: func(f *fakeFS) { f.entries["/etc/agent/p.y"] = trustMeta{0, 0o664} }},
		{name: "world writable file", path: "/etc/agent/p.y", cfg: cfgOf(), reason: "mode 0646",
			mut: func(f *fakeFS) { f.entries["/etc/agent/p.y"] = trustMeta{0, 0o646} }},
		{name: "group writable ancestor", path: "/etc/agent/p.y", cfg: cfgOf(), reason: "mode 0775",
			mut: func(f *fakeFS) { f.entries["/etc/agent"] = trustMeta{0, fs.ModeDir | 0o775} }},
		{name: "sticky world writable ancestor rejected", path: "/etc/agent/p.y", cfg: cfgOf(), reason: "mode 0777",
			mut: func(f *fakeFS) { f.entries["/etc"] = trustMeta{0, fs.ModeDir | fs.ModeSticky | 0o777} }},
		{name: "effective uid untrusted even if file owner", path: "/etc/agent/p.y", cfg: cfgOf(1000), reason: "effective uid"},
		{name: "empty path", path: "", cfg: cfgOf(), reason: "empty path"},
		{name: "missing file", path: "/etc/agent/none", cfg: cfgOf(), reason: "cannot stat"},
		{name: "missing ancestor", path: "/nope/p.y", cfg: cfgOf(), reason: "cannot stat"},
		{name: "directory not file", path: "/etc/agent", cfg: cfgOf(), reason: "not a regular file"},
		{name: "root path", path: "/", cfg: cfgOf(), reason: "not a regular file"},
		{name: "file used as directory", path: "/etc/agent/p.y/x", cfg: cfgOf(), reason: "not a directory"},
		{name: "special file", path: "/etc/agent/p.y", cfg: cfgOf(), reason: "not a regular file",
			mut: func(f *fakeFS) { f.entries["/etc/agent/p.y"] = trustMeta{0, fs.ModeNamedPipe | 0o644} }},
		{name: "stat error on root", path: "/etc/agent/p.y", cfg: cfgOf(), reason: "cannot stat",
			mut: func(f *fakeFS) { f.lstErr["/"] = errors.New("EACCES") }},
		{name: "open error", path: "/etc/agent/p.y", cfg: cfgOf(), reason: "cannot open",
			mut: func(f *fakeFS) { f.openErr = errors.New("ELOOP") }},
		{name: "swapped for untrusted file after walk", path: "/etc/agent/p.y", cfg: cfgOf(), reason: "owned by uid 1000",
			mut: func(f *fakeFS) { f.openAs = &trustMeta{1000, file644} }},
		{name: "swapped for non-regular after walk", path: "/etc/agent/p.y", cfg: cfgOf(), reason: "not a regular file",
			mut: func(f *fakeFS) { f.openAs = &trustMeta{0, fs.ModeDevice | 0o644} }},
		// symlinks
		{name: "root owned link to file", path: "/etc/link", cfg: cfgOf(),
			mut: func(f *fakeFS) {
				f.entries["/etc/link"] = trustMeta{0, link}
				f.links["/etc/link"] = "agent/p.y"
			}},
		{name: "absolute link", path: "/etc/link", cfg: cfgOf(),
			mut: func(f *fakeFS) {
				f.entries["/etc/link"] = trustMeta{0, link}
				f.links["/etc/link"] = "/etc/agent/p.y"
			}},
		{name: "dotdot link", path: "/etc/agent/link", cfg: cfgOf(),
			mut: func(f *fakeFS) {
				f.entries["/etc/agent/link"] = trustMeta{0, link}
				f.links["/etc/agent/link"] = "../agent/p.y"
			}},
		{name: "link to directory then file", path: "/etc/d/p.y", cfg: cfgOf(),
			mut: func(f *fakeFS) {
				f.entries["/etc/d"] = trustMeta{0, link}
				f.links["/etc/d"] = "agent"
			}},
		{name: "untrusted link owner", path: "/etc/link", cfg: cfgOf(), reason: "symlink owned by uid 1000",
			mut: func(f *fakeFS) {
				f.entries["/etc/link"] = trustMeta{1000, link}
				f.links["/etc/link"] = "agent/p.y"
			}},
		{name: "trusted link to untrusted target", path: "/etc/link", cfg: cfgOf(), reason: "owned by uid 1000",
			mut: func(f *fakeFS) {
				f.entries["/etc/link"] = trustMeta{0, link}
				f.links["/etc/link"] = "evil"
				f.entries["/etc/evil"] = trustMeta{1000, file644}
			}},
		{name: "trusted link into writable dir", path: "/etc/link", cfg: cfgOf(), reason: "mode 0777",
			mut: func(f *fakeFS) {
				f.entries["/etc/link"] = trustMeta{0, link}
				f.links["/etc/link"] = "/tmp/x"
				f.entries["/tmp"] = trustMeta{0, fs.ModeDir | fs.ModeSticky | 0o777}
				f.entries["/tmp/x"] = trustMeta{0, file644}
			}},
		{name: "link loop", path: "/etc/a", cfg: cfgOf(), reason: "too many levels",
			mut: func(f *fakeFS) {
				f.entries["/etc/a"] = trustMeta{0, link}
				f.links["/etc/a"] = "a"
			}},
		{name: "unreadable link", path: "/etc/a", cfg: cfgOf(), reason: "cannot read symlink",
			mut: func(f *fakeFS) { f.entries["/etc/a"] = trustMeta{0, link} }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := baseFS()
			if c.mut != nil {
				c.mut(f)
			}
			err := checkTrustedFS(f, c.path, c.cfg)
			if c.reason == "" {
				if err != nil {
					t.Fatalf("want trusted, got %v", err)
				}
				return
			}
			var te *TrustError
			if !errors.As(err, &te) || !errors.Is(err, ErrNotTrusted) {
				t.Fatalf("want *TrustError, got %v", err)
			}
			if !strings.Contains(te.Reason, c.reason) {
				t.Fatalf("reason %q lacks %q (%v)", te.Reason, c.reason, err)
			}
		})
	}
}

func TestRootEffectiveUIDMayTrustRoot(t *testing.T) {
	withEUID(t, 0)
	if err := checkTrustedFS(baseFS(), "/etc/agent/p.y", cfgOf()); err != nil {
		t.Fatal(err)
	}
}

func TestTrustErrorIOUnwrap(t *testing.T) {
	withEUID(t, 1000)
	f := baseFS()
	f.openErr = fs.ErrPermission
	err := checkTrustedFS(f, "/etc/agent/p.y", cfgOf())
	if !errors.Is(err, fs.ErrPermission) || !errors.Is(err, ErrNotTrusted) {
		t.Fatalf("%v", err)
	}
}

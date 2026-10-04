package policy

import (
	"errors"
	"fmt"

	"github.com/stainedhead/agent-cli-core/output"
)

// ErrNotTrusted is matched by errors.Is for every *TrustError.
var ErrNotTrusted = errors.New("policy: file is not trusted")

// TrustError reports that a file failed CheckTrustedFile. The check fails
// closed: an I/O error, a missing file and an unsupported platform are all
// reported as a TrustError, never as success. It maps to the policy_denied
// category (exit 6).
type TrustError struct {
	// Path is the path that failed, which may be an ancestor or symlink of
	// the file that was asked about.
	Path string
	// Reason says why the path is not trusted, for example "owned by uid
	// 1000, which is not trusted".
	Reason string
	// Err is the underlying I/O error, if any.
	Err error
}

// Error describes the failure.
func (e *TrustError) Error() string {
	msg := fmt.Sprintf("policy: %s is not a trusted file: %s", e.Path, e.Reason)
	if e.Err != nil {
		msg += ": " + e.Err.Error()
	}
	return msg
}

// Unwrap returns the underlying I/O error, if any.
func (e *TrustError) Unwrap() error { return e.Err }

// Is reports whether target is ErrNotTrusted.
func (e *TrustError) Is(target error) bool { return target == ErrNotTrusted }

// Category implements output.CategoryError.
func (e *TrustError) Category() output.Category { return output.CategoryPolicyDenied }

// Hint implements output.Hinter.
func (e *TrustError) Hint() string {
	return "place the file where only root (or a configured trusted user) can change it, in directories only they can change"
}

// TrustOption configures CheckTrustedFile.
type TrustOption func(*trustConfig)

type trustConfig struct{ uids map[uint32]bool }

// WithTrustedUIDs adds user ids that may own the file, its ancestors and any
// symlink on the way. Root (uid 0) is always trusted. The effective uid of the
// process is never accepted as a trusted owner unless it is root: a file the
// agent's own user owns is a file the agent can rewrite. Passing such a uid
// makes CheckTrustedFile fail with a *TrustError.
func WithTrustedUIDs(uids ...uint32) TrustOption {
	return func(c *trustConfig) {
		for _, u := range uids {
			c.uids[u] = true
		}
	}
}

func newTrustConfig(opts []TrustOption) trustConfig {
	c := trustConfig{uids: map[uint32]bool{0: true}}
	for _, o := range opts {
		if o != nil {
			o(&c)
		}
	}
	return c
}

// CheckTrustedFile proves that the file at path can only have been written by
// a trusted user. It is stricter than Load's writable check, which asks what
// the current user may do rather than who may have authored the file.
//
// Every directory from the root down to the file must be owned by a trusted
// uid (root, plus any WithTrustedUIDs) and must not be writable by group or
// others (mode & 022 == 0, sticky or not). Each symlink met on the way must
// itself be owned by a trusted uid, and its target is checked in turn, so the
// whole resolved path is verified. The file is then opened with O_NOFOLLOW and
// the checks of record (regular file, trusted owner, not group or world
// writable) are made with fstat on the open descriptor, so a file swapped in
// after the walk is still caught.
//
// Any failure, including a missing file, a permission error or a platform
// without POSIX ownership, returns a *TrustError (never nil): the check fails
// closed. Load and WritableMode are unaffected.
func CheckTrustedFile(path string, opts ...TrustOption) error {
	return checkTrustedFile(path, newTrustConfig(opts))
}

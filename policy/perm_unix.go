//go:build unix

package policy

import "syscall"

// accessFn is replaced in tests.
var accessFn = syscall.Access

// writableByMe reports whether the current user may write path.
func writableByMe(path string) (bool, error) {
	err := accessFn(path, 2) // W_OK
	switch err {
	case nil:
		return true, nil
	case syscall.EACCES, syscall.EROFS, syscall.EPERM:
		return false, nil
	}
	return false, err
}

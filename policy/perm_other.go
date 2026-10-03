//go:build !unix

package policy

// writableByMe is a no-op off POSIX platforms.
func writableByMe(string) (bool, error) { return false, nil }

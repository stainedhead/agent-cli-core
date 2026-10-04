//go:build !unix

package policy

// checkTrustedFile fails closed: ownership cannot be proven off POSIX
// platforms.
func checkTrustedFile(path string, _ trustConfig) error {
	return &TrustError{Path: path, Reason: "ownership checks are not supported on this platform"}
}

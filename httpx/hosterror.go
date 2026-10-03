package httpx

import (
	"net"
	"net/http"
	"strings"

	"github.com/stainedhead/agent-cli-core/output"
)

// ForbiddenHostCode is the stable machine code of *ForbiddenHostError.
const ForbiddenHostCode = "auth/forbidden-host"

// ForbiddenHostError reports that a request (or a redirect) was refused
// before any credential was attached, because its host is not in
// Config.AllowedHosts (by default: the host of the first request), or
// because it uses plain http to a non-loopback host without
// Config.AllowInsecureHTTP. It maps to exit code 4 and carries only the host
// name, never a URL, query or credential.
type ForbiddenHostError struct {
	// Host is the refused host (host or host:port as in the request URL).
	Host string
	// Insecure is true when the refusal is for plain http rather than the host.
	Insecure bool
}

// Error describes the refusal without any credential or URL.
func (e *ForbiddenHostError) Error() string {
	if e.Insecure {
		return "forbidden host: plain http to " + e.Host + " is refused (set AllowInsecureHTTP to permit it)"
	}
	return "forbidden host: " + e.Host + " is not an allowed host; credentials are not sent to it"
}

// Code returns ForbiddenHostCode.
func (e *ForbiddenHostError) Code() string { return ForbiddenHostCode }

// Category implements output.CategoryError.
func (e *ForbiddenHostError) Category() output.Category { return output.CategoryForbidden }

// Hint implements output.Hinter.
func (e *ForbiddenHostError) Hint() string {
	return "add the host to Config.AllowedHosts if it is trusted; otherwise do not retry"
}

// maxRedirects mirrors the net/http default.
const maxRedirects = 10

// checkRedirect is the CheckRedirect installed by NewClient: it refuses any
// redirect to a host that is not allowed, and plain-http downgrades.
func (t *Transport) checkRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= maxRedirects {
		return &redirectLimitError{}
	}
	if err := t.checkRequest(req); err != nil {
		t.trace(0, req, nil, err, nil)
		return err
	}
	return nil
}

type redirectLimitError struct{}

func (*redirectLimitError) Error() string { return "stopped after 10 redirects" }

// checkRequest enforces the host allow-list and the plain-http rule. When no
// AllowedHosts are configured, the first request's host is pinned.
func (t *Transport) checkRequest(req *http.Request) error {
	u := req.URL
	host := strings.ToLower(u.Host)
	if host == "" {
		return &ForbiddenHostError{Host: host}
	}
	t.mu.Lock()
	if len(t.allowed) == 0 {
		t.allowed = []string{host}
	}
	ok := false
	for _, a := range t.allowed {
		if a == host || a == strings.ToLower(u.Hostname()) {
			ok = true
			break
		}
	}
	t.mu.Unlock()
	if !ok {
		return &ForbiddenHostError{Host: host}
	}
	if u.Scheme == "http" && !t.cfg.AllowInsecureHTTP && !isLoopback(u.Hostname()) {
		return &ForbiddenHostError{Host: host, Insecure: true}
	}
	return nil
}

func isLoopback(h string) bool {
	if strings.EqualFold(h, "localhost") {
		return true
	}
	ip := net.ParseIP(h)
	return ip != nil && ip.IsLoopback()
}

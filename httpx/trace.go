package httpx

import (
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
)

// trace writes one redacted line for an attempt when tracing is enabled.
// Bodies are never written; headers pass through the redactor; the URL is
// reduced to scheme, host and path (credentials and query are dropped).
func (t *Transport) trace(attempt int, req *http.Request, resp *http.Response, err error) {
	w := t.cfg.Trace
	if w == nil {
		return
	}
	var b strings.Builder
	fmt.Fprintf(&b, "httpx attempt=%d %s %s", attempt, req.Method, t.cfg.Redactor.String(safeURL(req.URL)))
	fmt.Fprintf(&b, " request-headers=%s", t.headers(req.Header))
	switch {
	case err != nil:
		fmt.Fprintf(&b, " error=%q", t.cfg.Redactor.String(err.Error()))
	case resp != nil:
		fmt.Fprintf(&b, " status=%d response-headers=%s", resp.StatusCode, t.headers(resp.Header))
	}
	b.WriteByte('\n')
	_, _ = w.Write([]byte(b.String()))
}

func (t *Transport) headers(h http.Header) string {
	red := t.cfg.Redactor.Header(h)
	keys := make([]string, 0, len(red))
	for k := range red {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+"="+strings.Join(red[k], ","))
	}
	return "[" + strings.Join(parts, " ") + "]"
}

func safeURL(u *url.URL) string {
	if u == nil {
		return ""
	}
	s := u.Scheme + "://" + u.Hostname()
	if p := u.Port(); p != "" {
		s += ":" + p
	}
	s += u.EscapedPath()
	if u.RawQuery != "" {
		s += "?[query redacted]"
	}
	return s
}

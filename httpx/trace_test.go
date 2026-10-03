package httpx

import (
	"bytes"
	"net/http"
	"strings"
	"testing"
)

func TestTraceOffByDefault(t *testing.T) {
	srv, _ := script(t, []int{200}, nil)
	var buf bytes.Buffer
	resp, err := get(t, client(Config{Clock: newFake()}), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if buf.Len() != 0 {
		t.Fatal("trace written while disabled")
	}
}

func TestTraceRedacts(t *testing.T) {
	srv, _ := script(t, []int{503, 200}, map[string]string{"Set-Cookie": "sid=abcdefghijklmnop"})
	var buf bytes.Buffer
	fr := &fakeRefresher{}
	fr.token.Store("supersecrettokenvalue123")
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/path?access_token=querysecret99&x=1", nil)
	req.Header.Set("X-Api-Key", "apikeysecret12345")
	resp, err := client(Config{Clock: newFake(), Refresher: fr, Trace: &buf}).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	out := buf.String()
	for _, leak := range []string{"supersecrettokenvalue123", "querysecret99", "apikeysecret12345", "abcdefghijklmnop", "secret-body-content"} {
		if strings.Contains(out, leak) {
			t.Fatalf("trace leaked %q:\n%s", leak, out)
		}
	}
	for _, want := range []string{"GET", "/path", "503", "200", "attempt=1", "attempt=2"} {
		if !strings.Contains(out, want) {
			t.Fatalf("trace missing %q:\n%s", want, out)
		}
	}
}

func TestTraceRecordsErrors(t *testing.T) {
	rt := &failingRT{errs: []error{errString("dial failed")}}
	var buf bytes.Buffer
	tr := NewTransport(rt, Config{Clock: newFake(), Trace: &buf})
	req, _ := http.NewRequest(http.MethodGet, "https://u:pw@x.invalid/", nil)
	resp, err := tr.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if !strings.Contains(buf.String(), "dial failed") || strings.Contains(buf.String(), "pw@") {
		t.Fatalf("trace:\n%s", buf.String())
	}
}

type errString string

func (e errString) Error() string { return string(e) }

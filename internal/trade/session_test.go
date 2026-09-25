package trade

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

type captureTransport struct{ cookies map[string]string }

func (t *captureTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	t.cookies[req.URL.Host] = req.Header.Get("Cookie")
	return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{}`)), Request: req}, nil
}

// The session goes to pathofexile.com and nowhere else, and only while set.
func TestSessionIsSentOnlyToPathOfExile(t *testing.T) {
	const fake = "0123456789abcdef0123456789abcdef" // not a real session
	capture := &captureTransport{cookies: map[string]string{}}
	c := NewInteractiveClient("Standard", 1)
	c.http.Transport = capture
	get := func(u string) {
		req, _ := http.NewRequest(http.MethodGet, u, nil)
		var out map[string]any
		if err := c.do(context.Background(), c.Fetch, req, &out); err != nil {
			t.Fatal(err)
		}
	}

	get("https://www.pathofexile.com/api/trade2/data/static")
	if capture.cookies["www.pathofexile.com"] != "" || c.SignedIn() {
		t.Fatal("a cookie went out before any session was set")
	}
	c.SetSession(fake)
	get("https://www.pathofexile.com/api/trade2/data/static")
	get("https://poe.ninja/api/whatever")
	if capture.cookies["www.pathofexile.com"] != "POESESSID="+fake || !c.SignedIn() {
		t.Fatalf("pathofexile.com cookie = %q", capture.cookies["www.pathofexile.com"])
	}
	if capture.cookies["poe.ninja"] != "" {
		t.Fatal("the session leaked to another host")
	}
	c.SetSession("")
	get("https://www.pathofexile.com/api/trade2/data/static")
	if capture.cookies["www.pathofexile.com"] != "" {
		t.Fatal("the session was still sent after clearing it")
	}
}

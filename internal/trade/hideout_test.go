package trade

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

type hideoutTransport struct {
	bodies  []map[string]any
	answers []bool
}

func (t *hideoutTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	var body map[string]any
	_ = json.NewDecoder(req.Body).Decode(&body)
	if req.URL.Path != "/api/trade2/whisper" || req.Header.Get("X-Requested-With") != "XMLHttpRequest" || !strings.HasPrefix(req.Header.Get("Cookie"), "POESESSID=") {
		return &http.Response{StatusCode: 400, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{}`)), Request: req}, nil
	}
	t.bodies = append(t.bodies, body)
	ok := t.answers[0]
	t.answers = t.answers[1:]
	answer := `{"success":false}`
	if ok {
		answer = `{"success":true}`
	}
	return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(answer)), Request: req}, nil
}

func TestHideoutTravel(t *testing.T) {
	const token = "hideout-token-0123456789abcdef"
	c := NewInteractiveClient("Standard", 1)
	if err := c.TravelToHideout(context.Background(), token); !errors.Is(err, ErrNotSignedIn) {
		t.Fatalf("signed out: %v", err)
	}
	c.SetSession("0123456789abcdef0123456789abcdef") // not a real session

	tr := &hideoutTransport{answers: []bool{true}}
	c.http.Transport = tr
	if err := c.TravelToHideout(context.Background(), token); err != nil || len(tr.bodies) != 1 || tr.bodies[0]["token"] != token {
		t.Fatalf("first try: %v %+v", err, tr.bodies)
	}

	// A first refusal is retried once with "continue", as the trade site does.
	tr = &hideoutTransport{answers: []bool{false, true}}
	c.http.Transport = tr
	if err := c.TravelToHideout(context.Background(), token); err != nil || len(tr.bodies) != 2 || tr.bodies[1]["continue"] != true {
		t.Fatalf("retry: %v %+v", err, tr.bodies)
	}

	tr = &hideoutTransport{answers: []bool{false, false}}
	c.http.Transport = tr
	if err := c.TravelToHideout(context.Background(), token); !errors.Is(err, ErrHideoutDenied) {
		t.Fatalf("refused twice: %v", err)
	}
	if err := c.TravelToHideout(context.Background(), "bad token!"); err == nil {
		t.Fatal("a malformed token was sent")
	}
}

package trade

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// liveAPI answers the trade API calls a live search makes.
type liveAPI struct {
	searches atomic.Int32
	fetches  atomic.Int32
	status   int // search answer status; 0 = 200
	// tokens maps a live result token to the listing ids it stands for.
	tokens   map[string][]string
	lastPath atomic.Value
}

func (a *liveAPI) RoundTrip(req *http.Request) (*http.Response, error) {
	reply := func(status int, body string) (*http.Response, error) {
		return &http.Response{StatusCode: status, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body)), Request: req}, nil
	}
	switch {
	case strings.Contains(req.URL.Path, "/search/"):
		a.searches.Add(1)
		if a.status != 0 {
			return reply(a.status, `{"error":{"message":"bad query"}}`)
		}
		return reply(200, `{"id":"Srch01","result":["00aa11bb22cc33dd"],"total":1}`)
	case strings.Contains(req.URL.Path, "/fetch/"):
		a.fetches.Add(1)
		a.lastPath.Store(req.URL.RequestURI())
		ids := strings.Split(strings.TrimPrefix(req.URL.Path[strings.Index(req.URL.Path, "/fetch/"):], "/fetch/"), ",")
		if got, ok := a.tokens[ids[0]]; ok {
			ids = got
		}
		var rows []string
		for _, id := range ids {
			rows = append(rows, fmt.Sprintf(`{"id":%q,"listing":{"price":{"amount":5,"currency":"divine"},"account":{"name":"seller"},"hideout_token":"tok"},"item":{"typeLine":"Irradiated Tablet","baseType":"Irradiated Tablet","rarity":"Rare","ilvl":80,"identified":true}}`, id))
		}
		return reply(200, `{"result":[`+strings.Join(rows, ",")+`]}`)
	}
	return reply(404, "")
}

// fakeSocket plays frames the test pushes; closing frames ends the read.
type fakeSocket struct {
	frames chan []byte
	closed chan struct{}
	once   sync.Once
}

func (s *fakeSocket) Read(ctx context.Context) ([]byte, error) {
	select {
	case f, ok := <-s.frames:
		if !ok {
			return nil, io.EOF
		}
		return f, nil
	case <-s.closed:
		return nil, errors.New("closed")
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (s *fakeSocket) Close() error { s.once.Do(func() { close(s.closed) }); return nil }

type liveHarness struct {
	manager *LiveManager
	api     *liveAPI
	sockets chan *fakeSocket
	dials   atomic.Int32
	header  http.Header
	url     string
	found   chan []EvaluatedListing
	mu      sync.Mutex
}

func newLiveHarness(t *testing.T, signedIn bool) *liveHarness {
	t.Helper()
	h := &liveHarness{api: &liveAPI{}, sockets: make(chan *fakeSocket, 8), found: make(chan []EvaluatedListing, 8)}
	client := NewInteractiveClient("Forbidden Rites", 1)
	client.http.Transport = h.api
	if signedIn {
		client.SetSession("sess")
	}
	h.manager = NewLiveManager(LiveOptions{
		Client:  client,
		Base:    "wss://live.test/",
		Backoff: []time.Duration{10 * time.Millisecond},
		OnFound: func(_ LiveState, listings []EvaluatedListing) { h.found <- listings },
		Dial: func(_ context.Context, u string, header http.Header) (LiveSocket, error) {
			h.dials.Add(1)
			h.mu.Lock()
			h.header, h.url = header, u
			h.mu.Unlock()
			s := &fakeSocket{frames: make(chan []byte, 8), closed: make(chan struct{})}
			h.sockets <- s
			return s, nil
		},
	})
	t.Cleanup(h.manager.StopAll)
	return h
}

func (h *liveHarness) socket(t *testing.T) *fakeSocket {
	t.Helper()
	select {
	case s := <-h.sockets:
		return s
	case <-time.After(3 * time.Second):
		t.Fatal("no live socket opened")
		return nil
	}
}

func waitFor(t *testing.T, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func (h *liveHarness) state(id string) LiveState {
	for _, s := range h.manager.States() {
		if s.ID == id {
			return s
		}
	}
	return LiveState{}
}

func TestLiveSearchNeedsASession(t *testing.T) {
	h := newLiveHarness(t, false)
	if err := h.manager.Start("s1", "Tablet", EvaluateRequest{BaseType: "Irradiated Tablet"}); !errors.Is(err, ErrNotSignedIn) {
		t.Fatalf("err = %v", err)
	}
}

func TestLiveSearchReportsNewListingsOnce(t *testing.T) {
	h := newLiveHarness(t, true)
	if err := h.manager.Start("s1", "Tablet", EvaluateRequest{BaseType: "Irradiated Tablet"}); err != nil {
		t.Fatal(err)
	}
	sock := h.socket(t)
	h.mu.Lock()
	url, header := h.url, h.header
	h.mu.Unlock()
	if url != "wss://live.test/Forbidden%20Rites/Srch01" || header.Get("Cookie") != "POESESSID=sess" || header.Get("Origin") != "https://www.pathofexile.com" {
		t.Fatalf("dial %s %v", url, header)
	}
	sock.frames <- []byte(`{"auth":true}`)
	waitFor(t, "live status", func() bool { return h.state("s1").Status == LiveActive })

	sock.frames <- []byte(`{"new":["00aa11bb22cc33dd","11aa11bb22cc33dd"]}`)
	got := <-h.found
	if len(got) != 2 || got[0].Amount != 5 || got[0].HideoutToken != "tok" {
		t.Fatalf("found = %+v", got)
	}
	// A listing pushed again (a price change) is not reported twice.
	sock.frames <- []byte(`{"new":["00aa11bb22cc33dd","22aa11bb22cc33dd"]}`)
	got = <-h.found
	if len(got) != 1 || got[0].ID != "22aa11bb22cc33dd" {
		t.Fatalf("second found = %+v", got)
	}
	waitFor(t, "found count", func() bool { return h.state("s1").Found == 3 })
	if res := h.manager.Results("s1"); len(res) != 3 || res[0].ID != "22aa11bb22cc33dd" {
		t.Fatalf("results newest first = %+v", res)
	}
	if h.api.searches.Load() != 1 || h.api.fetches.Load() != 2 {
		t.Fatalf("searches %d fetches %d", h.api.searches.Load(), h.api.fetches.Load())
	}
	h.manager.Stop("s1")
	if s := h.state("s1"); s.Status != LiveIdle || s.Found != 3 {
		t.Fatalf("stopped = %+v", s)
	}
}

func TestLiveSearchReconnectsWithoutSearchingAgain(t *testing.T) {
	h := newLiveHarness(t, true)
	if err := h.manager.Start("s1", "Tablet", EvaluateRequest{BaseType: "Irradiated Tablet"}); err != nil {
		t.Fatal(err)
	}
	close(h.socket(t).frames) // the connection drops
	second := h.socket(t)
	second.frames <- []byte(`{"auth":true}`)
	waitFor(t, "live again", func() bool { return h.state("s1").Status == LiveActive })
	if h.dials.Load() != 2 || h.api.searches.Load() != 1 {
		t.Fatalf("dials %d searches %d", h.dials.Load(), h.api.searches.Load())
	}
}

func TestLiveSearchStopsWhenGGGRefusesTheSession(t *testing.T) {
	h := newLiveHarness(t, true)
	if err := h.manager.Start("s1", "Tablet", EvaluateRequest{BaseType: "Irradiated Tablet"}); err != nil {
		t.Fatal(err)
	}
	h.socket(t).frames <- []byte(`{"auth":false}`)
	waitFor(t, "error status", func() bool { return h.state("s1").Status == LiveError })
	if h.manager.Active() != 0 {
		t.Fatal("refused search still counts as active")
	}
}

func TestLiveSearchStopsOnABadQuery(t *testing.T) {
	h := newLiveHarness(t, true)
	h.api.status = 400
	if err := h.manager.Start("s1", "Tablet", EvaluateRequest{BaseType: "Irradiated Tablet"}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "error status", func() bool { return h.state("s1").Status == LiveError })
	if h.dials.Load() != 0 || h.api.searches.Load() != 1 {
		t.Fatalf("dials %d searches %d", h.dials.Load(), h.api.searches.Load())
	}
}

func TestLiveSearchLimit(t *testing.T) {
	h := newLiveHarness(t, true)
	h.manager.opts.Dial = func(ctx context.Context, _ string, _ http.Header) (LiveSocket, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	for i := 0; i < MaxLiveSearches; i++ {
		if err := h.manager.Start(fmt.Sprint("s", i), "x", EvaluateRequest{BaseType: "Irradiated Tablet"}); err != nil {
			t.Fatal(err)
		}
	}
	if err := h.manager.Start("one more", "x", EvaluateRequest{BaseType: "Irradiated Tablet"}); !errors.Is(err, ErrLiveLimit) {
		t.Fatalf("err = %v", err)
	}
	h.manager.Stop("s0")
	if err := h.manager.Start("one more", "x", EvaluateRequest{BaseType: "Irradiated Tablet"}); err != nil {
		t.Fatalf("after a stop: %v", err)
	}
}

func TestLiveSearchFetchesResultTokens(t *testing.T) {
	h := newLiveHarness(t, true)
	h.api.tokens = map[string][]string{"eyJhbGciOi.J9-x_y": {"aa00aa00aa00aa00", "bb00bb00bb00bb00"}}
	if err := h.manager.Start("s1", "Tablet", EvaluateRequest{BaseType: "Irradiated Tablet"}); err != nil {
		t.Fatal(err)
	}
	sock := h.socket(t)
	sock.frames <- []byte(`{"auth":true}`)
	waitFor(t, "live status", func() bool { return h.state("s1").Status == LiveActive })
	sock.frames <- []byte(`{"result":"eyJhbGciOi.J9-x_y","count":2}`)
	got := <-h.found
	if len(got) != 2 || got[1].ID != "bb00bb00bb00bb00" {
		t.Fatalf("found = %+v", got)
	}
	// The trade site fetches a token by itself, without the search id.
	if path := h.api.lastPath.Load().(string); path != "/api/trade2/fetch/eyJhbGciOi.J9-x_y" {
		t.Fatalf("fetch path = %s", path)
	}
}

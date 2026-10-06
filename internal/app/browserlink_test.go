package app

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"poe2filter/internal/session"
	"poe2filter/internal/trade"
)

func newLinkTestService(t *testing.T) *AppService {
	s := &AppService{
		session:          session.New(t.TempDir()),
		overlayClient:    trade.NewInteractiveClient("Standard", 1),
		overlayEvalCache: map[string]overlayEvaluationCacheEntry{},
	}
	s.link.code, s.link.state = "code-0123456789abcdef", LinkWaiting
	return s
}

func postLink(s *AppService, path, origin, body string) int {
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:47819"+path, strings.NewReader(body))
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	rec := httptest.NewRecorder()
	s.handleLink(rec, req)
	return rec.Code
}

const (
	chromeOrigin = "chrome-extension://" + chromiumExtensionID
	fakeSession  = "0123456789abcdef0123456789abcdef" // not a real session
)

func TestLinkRefusesStrangers(t *testing.T) {
	s := newLinkTestService(t)
	good := `{"code":"code-0123456789abcdef","session":"` + fakeSession + `"}`
	for name, origin := range map[string]string{
		"web page":         "https://evil.example",
		"other extension":  "chrome-extension://aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"no origin":        "",
		"pathofexile page": "https://www.pathofexile.com",
	} {
		if code := postLink(s, "/link", origin, good); code != http.StatusForbidden {
			t.Errorf("%s: status %d, want 403", name, code)
		}
	}
	if code := postLink(s, "/link", chromeOrigin, `{"code":"wrong-code-0123456789","session":"`+fakeSession+`"}`); code != http.StatusConflict {
		t.Errorf("wrong code: status %d, want 409", code)
	}
	if s.overlayClient.SignedIn() || s.session.Load() != "" {
		t.Fatal("a refused request stored a session")
	}
}

func TestLinkSteps(t *testing.T) {
	s := newLinkTestService(t)
	// Step 2: the extension is there but pathofexile.com is signed out.
	if code := postLink(s, "/status", chromeOrigin, `{"code":"code-0123456789abcdef","state":"no-session"}`); code != http.StatusNoContent {
		t.Fatalf("status: %d", code)
	}
	if st := s.BrowserLinkState(); st.State != LinkNeedLogin || st.Connected {
		t.Fatalf("after no-session: %+v", st)
	}
	// A malformed session is refused and nothing is stored.
	if code := postLink(s, "/link", chromeOrigin, `{"code":"code-0123456789abcdef","session":"bad value"}`); code != http.StatusBadRequest {
		t.Fatalf("bad session: %d", code)
	}
	// Signed in: the session arrives, is stored encrypted and used.
	if code := postLink(s, "/link", "moz-extension://1234-5678", `{"code":"code-0123456789abcdef","session":"`+fakeSession+`","accountName":"MrW#1234"}`); code != http.StatusNoContent {
		t.Fatalf("link: %d", code)
	}
	if st := s.BrowserLinkState(); st.State != LinkLinked || !st.Connected || st.URL != "" || st.AccountName != "MrW#1234" {
		t.Fatalf("after link: %+v", st)
	}
	if s.session.Load() != fakeSession || !s.overlayClient.SignedIn() {
		t.Fatal("the session was not stored and applied")
	}
	// The code is spent.
	if code := postLink(s, "/link", chromeOrigin, `{"code":"code-0123456789abcdef","session":"`+fakeSession+`"}`); code != http.StatusConflict {
		t.Fatalf("reused code: %d", code)
	}
	if st, err := s.DisconnectBrowser(); err != nil || st.Connected || st.AccountName != "" || s.session.Load() != "" {
		t.Fatalf("disconnect: %+v %v", st, err)
	}
}

func TestExtensionOriginAcceptsOnlyOurExtensions(t *testing.T) {
	saved := chromiumStoreIDs
	chromiumStoreIDs = []string{"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}
	t.Cleanup(func() { chromiumStoreIDs = saved })
	for origin, want := range map[string]bool{
		"chrome-extension://" + chromiumExtensionID:            true, // unpacked, fixed key
		"chrome-extension://bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb":  true, // a store build
		"chrome-extension://cccccccccccccccccccccccccccccccc":  false,
		"moz-extension://6f1c2d7e-0000-4000-8000-000000000000": true,
		"https://www.pathofexile.com":                          false,
		"":                                                     false,
	} {
		if got := extensionOrigin(origin); got != want {
			t.Errorf("extensionOrigin(%q) = %v, want %v", origin, got, want)
		}
	}
}

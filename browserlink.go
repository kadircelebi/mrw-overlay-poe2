package main

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"poe2filter/internal/i18n"
)

// Connecting the browser: the app opens pathofexile.com with a one-time code
// in the address fragment; the browser extension (browser-extension/) reads
// it and sends the pathofexile.com session to a listener on 127.0.0.1. Only a
// request from the extension that carries the current code is accepted, and
// the listener runs only while a connection is being made.

//go:embed browser-extension
var browserExtension embed.FS

const (
	linkAddr = "127.0.0.1:47819"
	// chromiumExtensionID follows from the public key in the manifest, so it
	// is the same on every install; Chromium puts it in the Origin header.
	chromiumExtensionID = "amajldbklpbnbmbeepkmhialdgncfdpj"
	// linkURL is a pathofexile.com page that loads without a redirect, so
	// the fragment survives whether or not the player is signed in.
	linkURL = "https://www.pathofexile.com/trade2"
	// extensionWait is how long the app waits to hear from the extension
	// before it offers to install it.
	extensionWait = 12 * time.Second
	// linkLifetime bounds how long a code and the listener stay alive.
	linkLifetime = 15 * time.Minute
)

// Link states shown in Settings.
const (
	LinkIdle          = "idle"          // nothing in progress
	LinkWaiting       = "waiting"       // browser opened, extension not heard from yet
	LinkNeedExtension = "needExtension" // step 1: the extension is not installed (or not in this browser)
	LinkNeedLogin     = "needLogin"     // step 2: the extension answered, but pathofexile.com is signed out
	LinkLinked        = "linked"        // the session arrived
	LinkError         = "error"
)

type BrowserLinkStatus struct {
	State string `json:"state"`
	// Connected reports a stored session (the overlay searches signed in).
	Connected   bool  `json:"connected"`
	ConnectedAt int64 `json:"connectedAt"`
	// URL is the link to open by hand in another browser (the code is valid
	// until the request ends).
	URL   string `json:"url,omitempty"`
	Error string `json:"error,omitempty"`
	// Rules is GGG's rate-limit policy on the last search ("Account,Ip" when
	// it counted the search as signed in, "Ip" when not; "" before any).
	Rules string `json:"rules,omitempty"`
	// ExtensionOutdated is set when the extension that answered is older
	// than the one this app carries: its folder has been refreshed, and the
	// browser needs "Reload" on its extensions page to pick it up.
	ExtensionOutdated bool `json:"extensionOutdated,omitempty"`
}

type browserLink struct {
	mu sync.Mutex
	// seenVersion is the extension version that last answered.
	seenVersion string
	state       string
	code     string
	err      string
	server   *http.Server
	deadline time.Time
	cancel   context.CancelFunc
	// waitGen tells the latest extension-wait timer from older ones.
	waitGen int
}

// ConnectBrowser starts (or restarts) a connection: a new code, the local
// listener, and optionally the default browser on pathofexile.com.
func (s *AppService) ConnectBrowser(openBrowser bool) BrowserLinkStatus {
	code, err := newLinkCode()
	if err != nil {
		return s.setLinkError(err)
	}
	l := &s.link
	l.mu.Lock()
	if l.cancel != nil {
		l.cancel()
	}
	ctx, cancel := context.WithTimeout(context.Background(), linkLifetime)
	l.code, l.state, l.err, l.cancel, l.deadline = code, LinkWaiting, "", cancel, time.Now().Add(linkLifetime)
	if l.server == nil {
		if err := s.startLinkServer(); err != nil {
			l.mu.Unlock()
			cancel()
			return s.setLinkError(err)
		}
	}
	l.mu.Unlock()

	go func() {
		<-ctx.Done()
		s.endLink(code)
	}()
	if openBrowser && s.app != nil {
		if err := s.app.Browser.OpenURL(linkURL + "#mrw-link=" + code); err != nil {
			return s.setLinkError(err)
		}
		s.armExtensionWait()
	}
	return s.BrowserLinkState()
}

// armExtensionWait starts the clock once the link has been opened in a
// browser: no word from the extension in time means step 1 (install it).
// Opening the link again restarts it.
func (s *AppService) armExtensionWait() {
	l := &s.link
	l.mu.Lock()
	l.waitGen++
	gen, code := l.waitGen, l.code
	if l.state == LinkNeedExtension {
		l.state = LinkWaiting
	}
	l.mu.Unlock()
	go func() {
		time.Sleep(extensionWait)
		l.mu.Lock()
		if l.waitGen == gen && l.code == code && l.state == LinkWaiting {
			l.state = LinkNeedExtension
		}
		l.mu.Unlock()
	}()
}

// BrowserLinkState is polled by Settings while a connection is in progress.
func (s *AppService) BrowserLinkState() BrowserLinkStatus {
	l := &s.link
	l.mu.Lock()
	defer l.mu.Unlock()
	st := BrowserLinkStatus{State: l.state, Error: l.err, Connected: s.overlayClient.SignedIn(), Rules: s.overlayClient.Search.Status().Rules}
	if st.State == "" {
		st.State = LinkIdle
	}
	if at := s.session.SavedAt(); st.Connected && !at.IsZero() {
		st.ConnectedAt = at.UnixMilli()
	}
	if l.code != "" && st.State != LinkLinked {
		st.URL = linkURL + "#mrw-link=" + l.code
	}
	st.ExtensionOutdated = l.seenVersion != "" && l.seenVersion != bundledExtensionVersion()
	return st
}

// bundledExtensionVersion is the version of the extension this app carries.
func bundledExtensionVersion() string {
	raw, err := browserExtension.ReadFile("browser-extension/manifest.json")
	if err != nil {
		return ""
	}
	var m struct {
		Version string `json:"version"`
	}
	_ = json.Unmarshal(raw, &m)
	return m.Version
}

// refreshBrowserExtension rewrites an extension folder written earlier, so a
// browser that loaded it gets the current version on its next reload. A
// player who never set the extension up gets no folder.
func (s *AppService) refreshBrowserExtension() {
	if _, err := os.Stat(filepath.Join(s.meta.DataDir, "browser-extension", "manifest.json")); err == nil {
		_, _ = s.PrepareBrowserExtension()
	}
}

// CancelBrowserConnect stops waiting for the extension.
func (s *AppService) CancelBrowserConnect() BrowserLinkStatus {
	s.link.mu.Lock()
	code := s.link.code
	s.link.mu.Unlock()
	s.endLink(code)
	s.link.mu.Lock()
	if s.link.state != LinkLinked {
		s.link.state = LinkIdle
	}
	s.link.mu.Unlock()
	return s.BrowserLinkState()
}

// DisconnectBrowser forgets the stored session; searches go out anonymous.
func (s *AppService) DisconnectBrowser() (BrowserLinkStatus, error) {
	s.overlayClient.SetSession("")
	s.live.StopAll()
	err := s.session.Clear()
	s.link.mu.Lock()
	s.link.state = LinkIdle
	s.link.mu.Unlock()
	return s.BrowserLinkState(), err
}

// PrepareBrowserExtension writes the extension into the data folder, where
// the browser can load it ("Load unpacked"), and returns that folder.
func (s *AppService) PrepareBrowserExtension() (string, error) {
	dir := filepath.Join(s.meta.DataDir, "browser-extension")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	err := fs.WalkDir(browserExtension, "browser-extension", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := browserExtension.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(dir, filepath.Base(path)), data, 0o644)
	})
	return dir, err
}

func (s *AppService) setLinkError(err error) BrowserLinkStatus {
	s.link.mu.Lock()
	s.link.state, s.link.err = LinkError, err.Error()
	s.link.mu.Unlock()
	return s.BrowserLinkState()
}

// endLink retires a code and, if no other code is live, stops the listener.
func (s *AppService) endLink(code string) {
	l := &s.link
	l.mu.Lock()
	defer l.mu.Unlock()
	if code == "" || l.code != code {
		return
	}
	l.code = ""
	if l.cancel != nil {
		l.cancel()
		l.cancel = nil
	}
	if l.state == LinkWaiting || l.state == LinkNeedExtension || l.state == LinkNeedLogin {
		l.state = LinkIdle
	}
	if l.server != nil {
		srv := l.server
		l.server = nil
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			_ = srv.Shutdown(ctx)
		}()
	}
}

// startLinkServer listens on 127.0.0.1 only. Callers hold s.link.mu.
func (s *AppService) startLinkServer() error {
	ln, err := net.Listen("tcp", linkAddr)
	if err != nil {
		return errors.New(i18n.T("account.err.port", linkAddr))
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/link", s.handleLink)
	mux.HandleFunc("/status", s.handleLink)
	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 5 * time.Second}
	s.link.server = srv
	go func() { _ = srv.Serve(ln) }()
	return nil
}

type linkMessage struct {
	Code    string `json:"code"`
	Version string `json:"version,omitempty"`
	Session string `json:"session,omitempty"`
	State   string `json:"state,omitempty"`
}

func (s *AppService) handleLink(rw http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost || !extensionOrigin(req.Header.Get("Origin")) {
		http.Error(rw, "forbidden", http.StatusForbidden)
		return
	}
	var msg linkMessage
	if err := json.NewDecoder(io.LimitReader(req.Body, 4096)).Decode(&msg); err != nil {
		http.Error(rw, "bad request", http.StatusBadRequest)
		return
	}
	l := &s.link
	l.mu.Lock()
	current := l.code
	l.mu.Unlock()
	if current == "" || subtle.ConstantTimeCompare([]byte(msg.Code), []byte(current)) != 1 {
		http.Error(rw, "no such request", http.StatusConflict)
		return
	}
	l.mu.Lock()
	if len(msg.Version) <= 16 {
		l.seenVersion = msg.Version
	}
	l.mu.Unlock()
	switch req.URL.Path {
	case "/status":
		l.mu.Lock()
		if l.code == current && msg.State == "no-session" {
			l.state = LinkNeedLogin
		}
		l.mu.Unlock()
		rw.WriteHeader(http.StatusNoContent)
	case "/link":
		if err := s.session.Save(msg.Session); err != nil {
			http.Error(rw, "invalid session", http.StatusBadRequest)
			return
		}
		s.overlayClient.SetSession(msg.Session)
		s.clearEvaluationCache()
		l.mu.Lock()
		l.state = LinkLinked
		l.mu.Unlock()
		rw.WriteHeader(http.StatusNoContent)
		// The code is spent; stop listening.
		go s.endLink(current)
	default:
		http.NotFound(rw, req)
	}
}

// extensionOrigin accepts our Chromium extension (fixed ID) and Firefox
// extensions (Firefox gives each install a random ID; the one-time code is
// what proves the request belongs to this connection). Web pages cannot send
// these origins.
func extensionOrigin(origin string) bool {
	return origin == "chrome-extension://"+chromiumExtensionID || strings.HasPrefix(origin, "moz-extension://")
}

func newLinkCode() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// clearEvaluationCache drops cached searches, whose limits (and so results)
// depended on whether they went out signed in.
func (s *AppService) clearEvaluationCache() {
	s.overlayEvalMu.Lock()
	for key := range s.overlayEvalCache {
		delete(s.overlayEvalCache, key)
	}
	s.overlayEvalMu.Unlock()
}

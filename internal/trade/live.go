package trade

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"sync"
	"time"

	"github.com/coder/websocket"

	"poe2filter/internal/useragent"
)

// Live search: the trade site's "Activate Live Search". A search is run once
// for its id, then GGG's websocket pushes the ids of listings that match it as
// they are listed. Only a signed-in session may open one, and GGG allows at
// most 20 per account at a time.

const (
	liveBase = "wss://www.pathofexile.com/api/trade2/live/poe2/"
	// MaxLiveSearches is GGG's limit of live searches open at once.
	MaxLiveSearches = 20
	// liveKeep is how many of the newest listings a live search keeps.
	liveKeep = 50
)

var (
	ErrLiveLimit = errors.New("too many live searches")
	// ErrLiveAuth means GGG refused the socket's session ("auth": false).
	ErrLiveAuth = errors.New("GGG refused the live search session")
)

// Live search states.
const (
	LiveIdle         = "idle"
	LiveConnecting   = "connecting"
	LiveActive       = "live"
	LiveReconnecting = "reconnecting"
	LiveError        = "error"
)

// LiveState is what the market lists for one live search. ID is the saved
// search it belongs to.
type LiveState struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Status   string `json:"status"`
	Error    string `json:"error,omitempty"`
	SearchID string `json:"searchId,omitempty"`
	TradeURL string `json:"tradeUrl,omitempty"`
	// Found counts the listings pushed since the search was started.
	Found       int   `json:"found"`
	StartedAtMs int64 `json:"startedAtMs,omitempty"`
	LastFoundMs int64 `json:"lastFoundMs,omitempty"`
}

// LiveSocket is one open live search connection.
type LiveSocket interface {
	Read(ctx context.Context) ([]byte, error)
	Close() error
}

// LiveOptions wires a LiveManager to the app. Only Client is required.
type LiveOptions struct {
	Client *Client
	// OnChange is called after any state change (from a background goroutine).
	OnChange func()
	// OnFound is called with the newly listed items of a live search.
	OnFound func(state LiveState, listings []EvaluatedListing)
	// Describe turns an error into the text the market shows.
	Describe func(error) string
	Log      func(string)

	// For tests: the socket dialer, the websocket base URL and the waits
	// between reconnects.
	Dial    func(ctx context.Context, url string, header http.Header) (LiveSocket, error)
	Base    string
	Backoff []time.Duration
}

type liveRun struct {
	state   LiveState
	cancel  context.CancelFunc
	results []EvaluatedListing
	seen    map[string]bool
}

// LiveManager runs the live searches the player started.
type LiveManager struct {
	opts LiveOptions

	mu   sync.Mutex
	runs map[string]*liveRun
}

func NewLiveManager(opts LiveOptions) *LiveManager {
	if opts.Dial == nil {
		opts.Dial = dialLiveSocket
	}
	if opts.Base == "" {
		opts.Base = liveBase
	}
	if len(opts.Backoff) == 0 {
		opts.Backoff = []time.Duration{5 * time.Second, 15 * time.Second, 30 * time.Second, 60 * time.Second}
	}
	if opts.Describe == nil {
		opts.Describe = func(err error) string { return err.Error() }
	}
	return &LiveManager{opts: opts, runs: map[string]*liveRun{}}
}

// Start opens a live search for a saved search. Starting one that already
// runs does nothing. It spends one search of the quota to get the search id.
func (m *LiveManager) Start(id, name string, in EvaluateRequest) error {
	if !m.opts.Client.SignedIn() {
		return ErrNotSignedIn
	}
	m.mu.Lock()
	run := m.runs[id]
	if run != nil && run.cancel != nil {
		m.mu.Unlock()
		return nil
	}
	if m.activeLocked() >= MaxLiveSearches {
		m.mu.Unlock()
		return ErrLiveLimit
	}
	ctx, cancel := context.WithCancel(context.Background())
	if run == nil {
		run = &liveRun{seen: map[string]bool{}}
		m.runs[id] = run
	}
	run.cancel = cancel
	run.state = LiveState{ID: id, Name: name, Status: LiveConnecting, Found: run.state.Found, LastFoundMs: run.state.LastFoundMs, StartedAtMs: time.Now().UnixMilli()}
	m.mu.Unlock()
	m.changed()
	go m.loop(ctx, run, in)
	return nil
}

// Stop closes a live search; its found listings stay until cleared.
func (m *LiveManager) Stop(id string) {
	m.mu.Lock()
	run := m.runs[id]
	if run == nil || run.cancel == nil {
		m.mu.Unlock()
		return
	}
	run.cancel()
	run.cancel = nil
	run.state.Status, run.state.Error = LiveIdle, ""
	m.mu.Unlock()
	m.changed()
}

// StopAll closes every live search (the app quits, or the session is gone).
func (m *LiveManager) StopAll() {
	if m == nil {
		return
	}
	m.mu.Lock()
	ids := make([]string, 0, len(m.runs))
	for id := range m.runs {
		ids = append(ids, id)
	}
	m.mu.Unlock()
	for _, id := range ids {
		m.Stop(id)
	}
}

// Forget stops a live search and drops its results (its saved search was
// deleted).
func (m *LiveManager) Forget(id string) {
	if m == nil {
		return
	}
	m.Stop(id)
	m.mu.Lock()
	delete(m.runs, id)
	m.mu.Unlock()
	m.changed()
}

// Clear drops a live search's found listings and resets its count.
func (m *LiveManager) Clear(id string) {
	m.mu.Lock()
	if run := m.runs[id]; run != nil {
		run.results = nil
		run.state.Found, run.state.LastFoundMs = 0, 0
	}
	m.mu.Unlock()
	m.changed()
}

// States lists every live search that was started in this session.
func (m *LiveManager) States() []LiveState {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]LiveState, 0, len(m.runs))
	for _, run := range m.runs {
		out = append(out, run.state)
	}
	slices.SortFunc(out, func(a, b LiveState) int {
		if a.ID < b.ID {
			return -1
		}
		if a.ID > b.ID {
			return 1
		}
		return 0
	})
	return out
}

// Results returns a live search's found listings, newest first.
func (m *LiveManager) Results(id string) []EvaluatedListing {
	m.mu.Lock()
	defer m.mu.Unlock()
	if run := m.runs[id]; run != nil {
		return slices.Clone(run.results)
	}
	return []EvaluatedListing{}
}

// Active counts the live searches open or opening.
func (m *LiveManager) Active() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.activeLocked()
}

func (m *LiveManager) activeLocked() int {
	n := 0
	for _, run := range m.runs {
		if run.cancel != nil {
			n++
		}
	}
	return n
}

func (m *LiveManager) changed() {
	if m.opts.OnChange != nil {
		m.opts.OnChange()
	}
}

func (m *LiveManager) log(format string, args ...any) {
	if m.opts.Log != nil {
		m.opts.Log(fmt.Sprintf(format, args...))
	}
}

// update changes a run's state unless it was stopped in the meantime.
func (m *LiveManager) update(ctx context.Context, run *liveRun, change func(*LiveState)) bool {
	m.mu.Lock()
	if ctx.Err() != nil {
		m.mu.Unlock()
		return false
	}
	change(&run.state)
	m.mu.Unlock()
	m.changed()
	return true
}

// fail ends a run for good: the query or the session is wrong, and retrying
// would only spend quota.
func (m *LiveManager) fail(ctx context.Context, run *liveRun, err error) {
	m.mu.Lock()
	if ctx.Err() == nil {
		run.cancel()
		run.cancel = nil
		run.state.Status, run.state.Error = LiveError, m.opts.Describe(err)
	}
	m.mu.Unlock()
	m.changed()
}

func (m *LiveManager) wait(ctx context.Context, failures int) bool {
	d := m.opts.Backoff[min(failures, len(m.opts.Backoff)-1)]
	select {
	case <-ctx.Done():
		return false
	case <-time.After(d):
		return true
	}
}

func (m *LiveManager) loop(ctx context.Context, run *liveRun, in EvaluateRequest) {
	var searchID, league string
	failures := 0
	for ctx.Err() == nil {
		if searchID == "" {
			searchCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
			search, got, err := m.opts.Client.search(searchCtx, in)
			cancel()
			if ctx.Err() != nil {
				return
			}
			var apiErr *APIError
			if err != nil && (!errors.As(err, &apiErr) || (apiErr.Status >= 400 && apiErr.Status < 500 && apiErr.Status != 429)) {
				m.fail(ctx, run, err)
				return
			}
			if err != nil {
				failures++
				m.update(ctx, run, func(s *LiveState) { s.Status, s.Error = LiveReconnecting, m.opts.Describe(err) })
				if !m.wait(ctx, failures) {
					return
				}
				continue
			}
			searchID, league = search.ID, got
			m.update(ctx, run, func(s *LiveState) {
				s.SearchID = searchID
				s.TradeURL = fmt.Sprintf("https://www.pathofexile.com/trade2/search/poe2/%s/%s/live", url.PathEscape(league), searchID)
			})
		}

		header := http.Header{}
		header.Set("Origin", "https://www.pathofexile.com")
		header.Set("User-Agent", useragent.Value())
		if session := m.opts.Client.sessionValue(); session != "" {
			header.Set("Cookie", "POESESSID="+session)
		}
		dialCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		sock, err := m.opts.Dial(dialCtx, m.opts.Base+url.PathEscape(league)+"/"+searchID, header)
		cancel()
		if ctx.Err() != nil {
			if sock != nil {
				sock.Close()
			}
			return
		}
		if err != nil {
			failures++
			var apiErr *APIError
			if errors.As(err, &apiErr) && (apiErr.Status == 401 || apiErr.Status == 403) && failures >= 2 {
				m.fail(ctx, run, err)
				return
			}
			// A search id can expire; after a few refusals run the search again.
			if failures%3 == 0 {
				searchID = ""
			}
			m.log("live %s: dial: %v", run.state.ID, err)
			m.update(ctx, run, func(s *LiveState) { s.Status, s.Error = LiveReconnecting, m.opts.Describe(err) })
			if !m.wait(ctx, failures) {
				return
			}
			continue
		}

		// The socket is open; the search is live once GGG accepts the session.
		m.update(ctx, run, func(s *LiveState) { s.Error = "" })
		err = m.read(ctx, run, sock, searchID)
		sock.Close()
		if ctx.Err() != nil {
			return
		}
		if errors.Is(err, ErrLiveAuth) {
			m.fail(ctx, run, err)
			return
		}
		m.log("live %s: closed: %v", run.state.ID, err)
		failures = 1
		m.update(ctx, run, func(s *LiveState) { s.Status, s.Error = LiveReconnecting, m.opts.Describe(err) })
		if !m.wait(ctx, 0) {
			return
		}
	}
}

// liveMessage is a frame of the live socket: {"auth": true} once GGG has
// checked the session, then {"result": "<token>", "count": N} for each batch
// of new listings; the token is fetched like a listing id list. Older
// servers (and PoE1) sent {"new": ["<listing id>", …]} instead.
type liveMessage struct {
	Auth   *bool           `json:"auth"`
	Result json.RawMessage `json:"result"`
	Count  int             `json:"count"`
	New    []string        `json:"new"`
}

func (m *LiveManager) read(ctx context.Context, run *liveRun, sock LiveSocket, searchID string) error {
	for {
		data, err := sock.Read(ctx)
		if err != nil {
			return err
		}
		var msg liveMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			m.log("live %s: unreadable frame %.200q", run.state.ID, data)
			continue
		}
		switch {
		case msg.Auth != nil && !*msg.Auth:
			return ErrLiveAuth
		case msg.Auth != nil:
			m.log("live %s: authorised", run.state.ID)
			m.update(ctx, run, func(s *LiveState) { s.Status, s.Error = LiveActive, "" })
		case len(msg.Result) > 0 && string(msg.Result) != "null":
			var token string
			if err := json.Unmarshal(msg.Result, &token); err != nil || token == "" {
				m.log("live %s: result is not a token: %.200s", run.state.ID, msg.Result)
				continue
			}
			m.log("live %s: %d new (token %d chars)", run.state.ID, msg.Count, len(token))
			m.found(ctx, run, func(ctx context.Context) ([]EvaluatedListing, error) {
				return m.opts.Client.FetchLiveToken(ctx, token)
			})
		case len(msg.New) > 0:
			m.log("live %s: %d new ids", run.state.ID, len(msg.New))
			m.foundIDs(ctx, run, searchID, msg.New)
		default:
			m.log("live %s: unknown frame %.200q", run.state.ID, data)
		}
	}
}

// foundIDs fetches pushed listing ids, ten per fetch.
func (m *LiveManager) foundIDs(ctx context.Context, run *liveRun, searchID string, ids []string) {
	for len(ids) > 0 {
		page := ids[:min(FetchPageSize, len(ids))]
		ids = ids[len(page):]
		m.found(ctx, run, func(ctx context.Context) ([]EvaluatedListing, error) {
			return m.opts.Client.FetchEvaluated(ctx, searchID, page)
		})
	}
}

// found runs one fetch (fetch quota only) and reports the listings not seen
// before: a listing edited or re-priced is pushed again.
func (m *LiveManager) found(ctx context.Context, run *liveRun, fetch func(context.Context) ([]EvaluatedListing, error)) {
	fetchCtx, cancel := context.WithTimeout(ctx, time.Minute)
	listings, err := fetch(fetchCtx)
	cancel()
	if err != nil {
		m.log("live %s: fetch: %v", run.state.ID, err)
		return
	}
	m.mu.Lock()
	if ctx.Err() != nil {
		m.mu.Unlock()
		return
	}
	fresh := listings[:0:0]
	for _, listing := range listings {
		if listing.ID != "" && !run.seen[listing.ID] {
			run.seen[listing.ID] = true
			fresh = append(fresh, listing)
		}
	}
	if len(fresh) == 0 {
		m.mu.Unlock()
		return
	}
	run.results = append(slices.Clone(fresh), run.results...)
	if len(run.results) > liveKeep {
		run.results = run.results[:liveKeep]
	}
	run.state.Found += len(fresh)
	run.state.LastFoundMs = time.Now().UnixMilli()
	state := run.state
	m.mu.Unlock()
	if m.opts.OnFound != nil {
		m.opts.OnFound(state, fresh)
	}
	m.changed()
}

type websocketSocket struct{ conn *websocket.Conn }

func (s websocketSocket) Read(ctx context.Context) ([]byte, error) {
	_, data, err := s.conn.Read(ctx)
	return data, err
}

func (s websocketSocket) Close() error { return s.conn.Close(websocket.StatusNormalClosure, "") }

func dialLiveSocket(ctx context.Context, u string, header http.Header) (LiveSocket, error) {
	conn, resp, err := websocket.Dial(ctx, u, &websocket.DialOptions{HTTPHeader: header})
	if err != nil {
		if resp != nil && resp.StatusCode != http.StatusSwitchingProtocols {
			return nil, &APIError{Status: resp.StatusCode, Body: "live search handshake refused"}
		}
		return nil, err
	}
	conn.SetReadLimit(1 << 20)
	return websocketSocket{conn}, nil
}

// Package engine runs the filter pipeline: it keeps prices fresh, drives the
// exceptional scanner and writes the loot filter on a schedule. It knows
// nothing about the UI; front ends observe it through State and OnChange.
package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"poe2filter/internal/collector"
	"poe2filter/internal/filter"
	"poe2filter/internal/i18n"
	"poe2filter/internal/insights"
	"poe2filter/internal/neversink"
	"poe2filter/internal/prices"
	"poe2filter/internal/provider"
	"poe2filter/internal/shared"
	"poe2filter/internal/trade"
)

// Options configures an Engine.
type Options struct {
	Dir      string                   // folder for config and data
	OutPath  string                   // write the filter here instead of the game folder
	OnChange func()                   // called (possibly often) when State changes
	Notify   func(title, body string) // desktop notification, may be nil
}

// RunResult summarises the last successful filter write.
type RunResult struct {
	FilterPath       string  `json:"filterPath"`
	PriceSource      string  `json:"priceSource"`
	PricesAtMs       int64   `json:"pricesAtMs"`
	ThresholdEx      float64 `json:"thresholdEx"`
	DivineEx         float64 `json:"divineEx"`
	ChaosEx          float64 `json:"chaosEx"`
	ValuableCurrency int     `json:"valuableCurrency"`
	CheapCurrency    int     `json:"cheapCurrency"`
	ValuableUniques  int     `json:"valuableUniques"`
	CheapUniques     int     `json:"cheapUniques"`
	ValuableExcept   int     `json:"valuableExcept"`
	CheapExcept      int     `json:"cheapExcept"`
}

// ScanState is the exceptional scanner progress.
type ScanState struct {
	Enabled    bool    `json:"enabled"`
	Candidates int     `json:"candidates"`
	Keys       int     `json:"keys"` // roughly candidates x 2 (sockets, quality)
	Scanned    int     `json:"scanned"`
	Valuable   int     `json:"valuable"`
	Current    string  `json:"current"` // key waiting for its search
	Last       string  `json:"last"`    // last finished key and result
	NextAtMs   int64   `json:"nextAtMs"`
	EtaSec     float64 `json:"etaSec"` // until every base has been scanned once
}

// State is everything a front end needs to render.
type State struct {
	Running     bool    `json:"running"`
	Step        string  `json:"step"`
	Progress    float64 `json:"progress"` // 0..1
	LastRunAtMs int64   `json:"lastRunAtMs"`
	NextRunAtMs int64   `json:"nextRunAtMs"` // 0 when auto update is off
	LastError   string  `json:"lastError"`
	// LastOkAtMs is when the filter was last written; it stays put when a run
	// fails, so the panel can say the file in the game folder is stale.
	LastOkAtMs int64 `json:"lastOkAtMs"`
	// NextRetryAtMs is the automatic retry after a failed run (0 when none).
	NextRetryAtMs int64      `json:"nextRetryAtMs"`
	FailCount     int        `json:"failCount"`
	Last          *RunResult `json:"last"`
	Scan          ScanState  `json:"scan"`
	// Shared is the scan servers' prices; SharedUsed means they cover the
	// league and are what the filter uses (the own scanner then idles).
	Shared     shared.Status `json:"shared"`
	SharedUsed bool          `json:"sharedUsed"`
	Warnings   []string      `json:"warnings"`
	Log        []string      `json:"log"`
}

// Engine owns the pipeline, scanner and schedule.
type Engine struct {
	opt     Options
	dataDir string

	cfgMu sync.Mutex
	cfg   filter.Config

	runMu sync.Mutex // held for the duration of a run

	stMu      sync.Mutex
	st        State
	lastRunAt time.Time
	lastOkAt  time.Time
	retryAt   time.Time // set after a failed run, cleared on success
	failCount int

	leagueMu  sync.Mutex
	leagues   []string
	leaguesAt time.Time

	profileMu sync.Mutex

	shared *shared.Store

	scanMu     sync.Mutex
	scanner    *trade.Scanner
	scanCancel context.CancelFunc
	scanLeague string

	ns nsCache

	snapMu     sync.Mutex
	snap       *prices.Snapshot
	validBases map[string]string

	stop chan struct{}
}

// New loads (and migrates) the config from opt.Dir.
func New(opt Options) *Engine {
	e := &Engine{opt: opt, dataDir: filepath.Join(opt.Dir, "data"), stop: make(chan struct{})}
	e.shared = shared.New(filepath.Join(e.dataDir, "shared"))
	e.cfg = filter.LoadConfig(e.configPath())
	e.shared.LoadCached(e.cfg.LeagueName)
	_ = e.cfg.Save(e.configPath()) // persist migrated format
	e.loadLeagues()
	e.st.Step = i18n.T("step.ready")
	return e
}

func (e *Engine) configPath() string   { return filepath.Join(e.opt.Dir, "config.json") }
func (e *Engine) snapshotPath() string { return filepath.Join(e.dataDir, "prices.json") }

func (e *Engine) changed() {
	if e.opt.OnChange != nil {
		e.opt.OnChange()
	}
}

func (e *Engine) logf(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	e.stMu.Lock()
	e.st.Log = append(e.st.Log, time.Now().Format("15:04:05 ")+msg)
	if n := len(e.st.Log); n > 200 {
		e.st.Log = e.st.Log[n-200:]
	}
	e.stMu.Unlock()
	fmt.Println(msg)
	e.changed()
}

func (e *Engine) setStep(progress float64, step string) {
	e.stMu.Lock()
	e.st.Progress, e.st.Step = progress, step
	e.stMu.Unlock()
	e.changed()
}

// Config returns the current settings.
func (e *Engine) Config() filter.Config {
	e.cfgMu.Lock()
	defer e.cfgMu.Unlock()
	return e.cfg
}

// SetConfig validates, saves and applies new settings.
// leagueListTTL is how long a fetched league list is considered fresh.
const leagueListTTL = 6 * time.Hour

type leagueCache struct {
	FetchedAt time.Time `json:"fetched_at"`
	Leagues   []string  `json:"leagues"`
}

func (e *Engine) leaguesPath() string { return filepath.Join(e.dataDir, "leagues.json") }

// loadLeagues restores the last fetched league list, so the picker is populated
// before (and without) any network call.
func (e *Engine) loadLeagues() {
	data, err := os.ReadFile(e.leaguesPath())
	if err != nil {
		return
	}
	var lc leagueCache
	if json.Unmarshal(data, &lc) != nil || len(lc.Leagues) == 0 {
		return
	}
	e.leagueMu.Lock()
	e.leagues, e.leaguesAt = lc.Leagues, lc.FetchedAt
	e.leagueMu.Unlock()
}

// refreshLeagues fetches the live league list when the cached one is stale.
// A failure keeps whatever we already have: the list is a convenience, and the
// user's own league choice is never touched by it.
func (e *Engine) refreshLeagues(ctx context.Context) {
	e.leagueMu.Lock()
	fresh := len(e.leagues) > 0 && time.Since(e.leaguesAt) < leagueListTTL
	e.leagueMu.Unlock()
	if fresh {
		return
	}
	list, err := collector.FetchLeagues(ctx, nil)
	if err != nil {
		e.logf("%s", i18n.T("log.leaguesFailed", err))
		return
	}
	now := time.Now()
	e.leagueMu.Lock()
	e.leagues, e.leaguesAt = list, now
	e.leagueMu.Unlock()
	if data, err := json.MarshalIndent(leagueCache{FetchedAt: now, Leagues: list}, "", "  "); err == nil {
		_ = prices.WriteFileAtomic(e.leaguesPath(), data)
	}
	e.changed()
}

// Leagues returns the leagues on offer: the live list when one was fetched,
// the built-in list otherwise. It is a suggestion list only — nothing here
// writes LeagueName, so the user's choice survives an ended league, a renamed
// one or a failed fetch. Front ends keep the configured league selectable even
// when this list no longer carries it.
func (e *Engine) Leagues() []string {
	e.leagueMu.Lock()
	list := append([]string(nil), e.leagues...)
	e.leagueMu.Unlock()
	if len(list) == 0 {
		list = append(list, filter.DefaultLeagues...)
	}
	return list
}

func (e *Engine) SetConfig(c filter.Config) (filter.Config, error) {
	c.Normalize()
	if err := c.Save(e.configPath()); err != nil {
		return e.Config(), fmt.Errorf(i18n.T("err.settingsSave"), err)
	}
	e.cfgMu.Lock()
	e.cfg = c
	e.cfgMu.Unlock()
	e.ensureScanner(true)
	e.syncActiveProfile()
	e.changed()
	return c, nil
}

// Start launches the scanner and the schedule; the first run starts at once.
func (e *Engine) Start() {
	e.ensureScanner(true)
	go e.loop()
}

// Stop ends background work.
func (e *Engine) Stop() {
	select {
	case <-e.stop:
	default:
		close(e.stop)
	}
	e.scanMu.Lock()
	if e.scanCancel != nil {
		e.scanCancel()
	}
	e.scanMu.Unlock()
}

// RunOnce updates prices and writes the filter synchronously (CLI use).
func (e *Engine) RunOnce(ctx context.Context) error {
	e.ensureScanner(false)
	return e.run(ctx)
}

// UpdateNow starts a run in the background.
func (e *Engine) UpdateNow() error {
	e.stMu.Lock()
	busy := e.st.Running
	e.stMu.Unlock()
	if busy {
		return errors.New(i18n.T("err.updateRunning"))
	}
	go func() {
		if err := e.run(context.Background()); err != nil {
			e.logf("[HATA] %v", err)
		}
	}()
	return nil
}

// Retry schedule after a failed run: a filter that could not be written is
// worth another try in minutes, not in AutoUpdateHours hours.
const (
	retryFirstDelay = 5 * time.Minute
	retryMaxDelay   = 30 * time.Minute
)

// retryDelay backs off 5, 10, 20 then 30 minutes.
func retryDelay(fails int) time.Duration {
	d := retryFirstDelay
	for i := 1; i < fails && d < retryMaxDelay; i++ {
		d *= 2
	}
	return min(d, retryMaxDelay)
}

func (e *Engine) loop() {
	if err := e.run(context.Background()); err != nil {
		e.logf("[HATA] %v", err)
	}
	tick := time.NewTicker(30 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-e.stop:
			return
		case <-tick.C:
		}
		cfg := e.Config()
		e.stMu.Lock()
		last, retryAt, fails := e.lastRunAt, e.retryAt, e.failCount
		e.stMu.Unlock()
		switch {
		case !retryAt.IsZero() && !time.Now().Before(retryAt):
			e.logf("%s", i18n.T("log.retry", fails+1))
			if err := e.run(context.Background()); err != nil {
				e.logf("[HATA] %v", err)
			}
		case cfg.AutoUpdateEnabled && time.Since(last) >= time.Duration(cfg.AutoUpdateHours)*time.Hour:
			e.logf("%s", i18n.T("log.scheduled", cfg.AutoUpdateHours))
			if err := e.run(context.Background()); err != nil {
				e.logf("[HATA] %v", err)
			}
		}
		e.changed() // refresh scan progress for listeners
	}
}

func (e *Engine) ensureScanner(run bool) {
	cfg := e.Config()
	var msg string
	defer func() {
		if msg != "" {
			e.logf("%s", msg) // outside scanMu: logf notifies listeners that call State
		}
	}()
	e.scanMu.Lock()
	defer e.scanMu.Unlock()

	if e.scanner == nil || e.scanLeague != cfg.LeagueName {
		if e.scanCancel != nil {
			e.scanCancel()
			e.scanCancel = nil
		}
		client := trade.NewClient(cfg.LeagueName, float64(cfg.ScanBudgetPct)/100)
		e.scanner = trade.NewScanner(client, filepath.Join(e.dataDir, "exceptional_scan.json"),
			func(s string) { e.logf("%s", s) })
		e.scanner.SetOnChange(e.changed)
		e.scanLeague = cfg.LeagueName
	} else {
		e.scanner.SetBudget(float64(cfg.ScanBudgetPct) / 100)
	}

	covered := cfg.SharedScan && cfg.PriceSourceURL == "" && e.shared.Covers(cfg.LeagueName)
	want := cfg.ExceptionalScan && !covered
	switch {
	case !want && e.scanCancel != nil:
		e.scanCancel()
		e.scanCancel = nil
		msg = i18n.T("log.scanStopped")
		if covered {
			msg = i18n.T("log.scanShared")
		}
	case want && run && e.scanCancel == nil:
		ctx, cancel := context.WithCancel(context.Background())
		e.scanCancel = cancel
		go e.scanner.Run(ctx)
	}
}

func (e *Engine) basePath(ctx context.Context, cfg filter.Config) (string, error) {
	if cfg.CustomBaseFilter != "" {
		if fi, err := os.Stat(cfg.CustomBaseFilter); err == nil && !fi.IsDir() {
			return cfg.CustomBaseFilter, nil
		}
		e.logf("%s", i18n.T("log.customBaseMissing"))
	}
	return neversink.Ensure(ctx, cfg.Strictness, filepath.Join(e.dataDir, "neversink"), 12*time.Hour, collector.UserAgent)
}

// run refreshes prices and writes the filter. Only one run at a time.
func (e *Engine) run(ctx context.Context) (err error) {
	if !e.runMu.TryLock() {
		return errors.New(i18n.T("err.updateRunning"))
	}
	defer e.runMu.Unlock()

	e.stMu.Lock()
	e.st.Running, e.st.LastError = true, ""
	e.stMu.Unlock()
	defer func() {
		e.stMu.Lock()
		e.st.Running = false
		e.lastRunAt = time.Now()
		e.st.LastRunAtMs = e.lastRunAt.UnixMilli()
		if err != nil {
			e.st.LastError, e.st.Step, e.st.Progress = err.Error(), "Hata", 0
			e.failCount++
			e.retryAt = e.lastRunAt.Add(retryDelay(e.failCount))
		} else {
			e.failCount, e.retryAt = 0, time.Time{}
			e.lastOkAt = e.lastRunAt
			e.st.LastOkAtMs = e.lastOkAt.UnixMilli()
		}
		e.st.FailCount = e.failCount
		e.st.NextRetryAtMs = 0
		if !e.retryAt.IsZero() {
			e.st.NextRetryAtMs = e.retryAt.UnixMilli()
		}
		e.stMu.Unlock()
		e.changed()
	}()

	cfg := e.Config()
	e.refreshLeagues(ctx)
	e.setStep(0.1, i18n.T("step.neversink"))
	basePath, err := e.basePath(ctx, cfg)
	if err != nil {
		return err
	}
	baseContent, err := os.ReadFile(basePath)
	if err != nil {
		return err
	}
	validBases := neversink.BaseTypes(string(baseContent))

	e.setStep(0.3, i18n.T("step.prices"))
	e.scanMu.Lock()
	scanner := e.scanner
	e.scanMu.Unlock()

	// The scan servers' prices, when they cover the league, replace the own
	// scanner's; the scanner is then paused so it stops spending quota.
	var exceptional provider.ExceptionalSource
	if scanner != nil {
		exceptional = scanner
	}
	if cfg.SharedScan && cfg.PriceSourceURL == "" {
		sctx, cancel := context.WithTimeout(ctx, time.Minute)
		if err := e.shared.Refresh(sctx, cfg.LeagueName); err != nil {
			e.logf("%s", i18n.T("log.sharedFailed", err))
		}
		cancel()
		if e.shared.Covers(cfg.LeagueName) {
			exceptional = e.shared
		}
		e.ensureScanner(true)
	}

	var chain provider.Chain
	if cfg.PriceSourceURL != "" {
		chain = append(chain, provider.Remote{URL: cfg.PriceSourceURL, League: cfg.LeagueName, CachePath: e.snapshotPath()})
	}
	local := provider.Local{
		Options:   collector.Options{League: cfg.LeagueName, Log: func(s string) { e.logf("%s", strings.TrimSpace(s)) }},
		CachePath: e.snapshotPath(),
	}
	if exceptional != nil && cfg.PriceSourceURL == "" {
		local.Exceptional = exceptional
	}
	chain = append(chain, local, provider.Cache{Path: e.snapshotPath()})

	snap, source, err := chain.Get(ctx)
	if err != nil {
		return err
	}
	// The base filter is one source of valid base names, the trade item list is
	// the other and it is complete: a base NeverSink never mentions used to be
	// unknown to us, which meant it could not be priced and could not be named
	// in a rule, so it fell through to the catch-all and was always shown.
	tradeBases, berr := trade.EquipmentBaseTypes(ctx, filepath.Join(e.dataDir, "trade_items.json"))
	if berr != nil {
		e.logf("%s", i18n.T("log.basesFailed", berr))
	}
	for _, b := range tradeBases {
		if _, ok := validBases[strings.ToLower(b)]; !ok {
			validBases[strings.ToLower(b)] = b
		}
	}

	e.snapMu.Lock()
	e.snap, e.validBases = snap, validBases
	e.snapMu.Unlock()

	if scanner != nil {
		scanner.SetMarket(snap)
		scanner.SetHotThreshold(cfg.ThresholdEx(snap.Rates) / 2)
		if len(tradeBases) > 0 {
			scanner.SetCandidates(trade.BuildCandidates(tradeBases, nil, neversink.ExceptionalBases(string(baseContent))))
		}
	}

	e.setStep(0.7, i18n.T("step.rules"))
	ns := e.ns.set(basePath, string(baseContent))
	block, st := filter.GenerateDynamicFilterBlock(cfg, snap, validBases, ns)

	e.setStep(0.85, i18n.T("step.writing"))
	dest := e.opt.OutPath
	if dest == "" {
		if dest, err = filter.FilterPath(cfg.FilterName); err != nil {
			return err
		}
	}
	if err := filter.WriteFilter(basePath, block, dest); err != nil {
		return err
	}

	res := &RunResult{
		FilterPath: dest, PriceSource: source, PricesAtMs: snap.GeneratedAt.UnixMilli(),
		ThresholdEx: st.ThresholdEx, DivineEx: snap.Rates.DivineEx, ChaosEx: snap.Rates.ChaosEx,
		ValuableCurrency: st.ValuableCurrency, CheapCurrency: st.CheapCurrency,
		ValuableUniques: st.ValuableUniques, CheapUniques: st.CheapUniques,
		ValuableExcept: st.ValuableExcept, CheapExcept: st.CheapExcept,
	}
	e.stMu.Lock()
	e.st.Last, e.st.Warnings = res, st.Warnings
	e.st.Step, e.st.Progress = i18n.T("step.done"), 1
	e.stMu.Unlock()
	e.logf("%s", i18n.T("log.written", st.ValuableCurrency, st.ValuableUniques, st.ValuableExcept))

	if cfg.NotifyEnabled && e.opt.Notify != nil {
		e.opt.Notify(i18n.T("notify.title"), i18n.T("notify.body", cfg.FilterName))
	}
	return nil
}

// State returns a snapshot of the current state.
func (e *Engine) State() State {
	cfg := e.Config()
	e.scanMu.Lock()
	scanner, scanning := e.scanner, e.scanCancel != nil
	e.scanMu.Unlock()

	e.stMu.Lock()
	s := e.st
	s.Log = append([]string(nil), e.st.Log...)
	s.Warnings = append([]string(nil), e.st.Warnings...)
	if cfg.AutoUpdateEnabled && !e.lastRunAt.IsZero() {
		s.NextRunAtMs = e.lastRunAt.Add(time.Duration(cfg.AutoUpdateHours) * time.Hour).UnixMilli()
	}
	e.stMu.Unlock()

	s.Shared = e.shared.Status()
	s.SharedUsed = cfg.SharedScan && cfg.PriceSourceURL == "" && e.shared.Covers(cfg.LeagueName)
	if scanner != nil {
		ss := scanner.Status()
		s.Scan = ScanState{Enabled: scanning, Candidates: ss.Candidates, Keys: ss.Keys,
			Scanned: ss.Scanned, Valuable: ss.Valuable, Current: ss.Current, Last: ss.Last,
			NextAtMs: ss.NextAt, EtaSec: ss.EtaSec}
	}
	return s
}

// Prices returns the latest price snapshot (from disk before the first update
// of this session), or nil when none has been written yet.
func (e *Engine) Prices() *prices.Snapshot {
	e.snapMu.Lock()
	snap := e.snap
	e.snapMu.Unlock()
	if snap == nil {
		snap, _ = prices.Load(e.snapshotPath())
	}
	return snap
}

// SearchItems returns items matching query for the custom lists (max limit).
func (e *Engine) SearchItems(query string, limit int) []insights.SearchItem {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return nil
	}
	e.snapMu.Lock()
	snap, valid := e.snap, e.validBases
	e.snapMu.Unlock()
	if snap == nil {
		snap, _ = prices.Load(e.snapshotPath())
	}
	var out []insights.SearchItem
	for _, it := range insights.SearchItems(snap, valid) {
		if strings.Contains(strings.ToLower(it.Name), q) {
			out = append(out, it)
		}
	}
	// Prefix matches first, then by value.
	sort.SliceStable(out, func(i, j int) bool {
		pi := strings.HasPrefix(strings.ToLower(out[i].Name), q)
		pj := strings.HasPrefix(strings.ToLower(out[j].Name), q)
		if pi != pj {
			return pi
		}
		return out[i].PriceExalt > out[j].PriceExalt
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

// ExportScan returns the exceptional scan results in shareable form, so a
// player who has already spent the search budget can pass them on.
func (e *Engine) ExportScan() ([]byte, error) {
	e.scanMu.Lock()
	sc := e.scanner
	e.scanMu.Unlock()
	if sc == nil {
		return nil, errors.New(i18n.T("err.scanOff"))
	}
	return sc.Export()
}

// ImportScan merges someone else's results into ours and reports what changed.
func (e *Engine) ImportScan(data []byte) (trade.ImportResult, error) {
	e.scanMu.Lock()
	sc := e.scanner
	e.scanMu.Unlock()
	if sc == nil {
		return trade.ImportResult{}, errors.New(i18n.T("err.scanOff"))
	}
	res, err := sc.Import(data)
	if err != nil {
		return res, err
	}
	e.logf(i18n.T("log.imported"), res.Added, res.Updated, res.Skipped)
	e.changed()
	return res, nil
}

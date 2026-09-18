// Package engine runs the filter pipeline: it keeps prices fresh, drives the
// exceptional scanner and writes the loot filter on a schedule. It knows
// nothing about the UI; front ends observe it through State and OnChange.
package engine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"poe2filter/internal/collector"
	"poe2filter/internal/filter"
	"poe2filter/internal/insights"
	"poe2filter/internal/neversink"
	"poe2filter/internal/prices"
	"poe2filter/internal/provider"
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
	Running     bool       `json:"running"`
	Step        string     `json:"step"`
	Progress    float64    `json:"progress"` // 0..1
	LastRunAtMs int64      `json:"lastRunAtMs"`
	NextRunAtMs int64      `json:"nextRunAtMs"` // 0 when auto update is off
	LastError   string     `json:"lastError"`
	Last        *RunResult `json:"last"`
	Scan        ScanState  `json:"scan"`
	Warnings    []string   `json:"warnings"`
	Log         []string   `json:"log"`
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

	scanMu     sync.Mutex
	scanner    *trade.Scanner
	scanCancel context.CancelFunc
	scanLeague string

	snapMu     sync.Mutex
	snap       *prices.Snapshot
	validBases map[string]string

	stop chan struct{}
}

// New loads (and migrates) the config from opt.Dir.
func New(opt Options) *Engine {
	e := &Engine{opt: opt, dataDir: filepath.Join(opt.Dir, "data"), stop: make(chan struct{})}
	e.cfg = filter.LoadConfig(e.configPath())
	_ = e.cfg.Save(e.configPath()) // persist migrated format
	e.st.Step = "Hazır"
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
func (e *Engine) SetConfig(c filter.Config) (filter.Config, error) {
	c.Normalize()
	if err := c.Save(e.configPath()); err != nil {
		return e.Config(), fmt.Errorf("ayarlar kaydedilemedi: %w", err)
	}
	e.cfgMu.Lock()
	e.cfg = c
	e.cfgMu.Unlock()
	e.ensureScanner(true)
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
		return fmt.Errorf("zaten bir güncelleme çalışıyor")
	}
	go func() {
		if err := e.run(context.Background()); err != nil {
			e.logf("[HATA] %v", err)
		}
	}()
	return nil
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
		last := e.lastRunAt
		e.stMu.Unlock()
		if cfg.AutoUpdateEnabled && time.Since(last) >= time.Duration(cfg.AutoUpdateHours)*time.Hour {
			e.logf("Zamanlanmış güncelleme (%d saatte bir)", cfg.AutoUpdateHours)
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

	switch {
	case !cfg.ExceptionalScan && e.scanCancel != nil:
		e.scanCancel()
		e.scanCancel = nil
		msg = "Exceptional taraması durduruldu."
	case cfg.ExceptionalScan && run && e.scanCancel == nil:
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
		e.logf("[Uyarı] Özel temel filtre bulunamadı, NeverSink kullanılıyor.")
	}
	return neversink.Ensure(ctx, cfg.Strictness, filepath.Join(e.dataDir, "neversink"), 12*time.Hour, collector.UserAgent)
}

// run refreshes prices and writes the filter. Only one run at a time.
func (e *Engine) run(ctx context.Context) (err error) {
	if !e.runMu.TryLock() {
		return fmt.Errorf("zaten bir güncelleme çalışıyor")
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
		}
		e.stMu.Unlock()
		e.changed()
	}()

	cfg := e.Config()
	e.setStep(0.1, "NeverSink filtresi hazırlanıyor")
	basePath, err := e.basePath(ctx, cfg)
	if err != nil {
		return err
	}
	baseContent, err := os.ReadFile(basePath)
	if err != nil {
		return err
	}
	validBases := neversink.BaseTypes(string(baseContent))

	e.setStep(0.3, "Piyasa fiyatları alınıyor")
	e.scanMu.Lock()
	scanner := e.scanner
	e.scanMu.Unlock()

	var chain provider.Chain
	if cfg.PriceSourceURL != "" {
		chain = append(chain, provider.Remote{URL: cfg.PriceSourceURL, League: cfg.LeagueName, CachePath: e.snapshotPath()})
	}
	local := provider.Local{
		Options:   collector.Options{League: cfg.LeagueName, Log: func(s string) { e.logf("%s", strings.TrimSpace(s)) }},
		CachePath: e.snapshotPath(),
	}
	if scanner != nil && cfg.PriceSourceURL == "" {
		local.Exceptional = scanner
	}
	chain = append(chain, local, provider.Cache{Path: e.snapshotPath()})

	snap, source, err := chain.Get(ctx)
	if err != nil {
		return err
	}
	e.snapMu.Lock()
	e.snap, e.validBases = snap, validBases
	e.snapMu.Unlock()

	if scanner != nil {
		scanner.SetMarket(snap)
		scanner.SetHotThreshold(cfg.ThresholdEx(snap.Rates) / 2)
		if bases, berr := trade.EquipmentBaseTypes(ctx, filepath.Join(e.dataDir, "trade_items.json")); berr == nil {
			droppable := map[string]bool{}
			for _, b := range validBases {
				droppable[b] = true
			}
			scanner.SetCandidates(trade.BuildCandidates(bases, droppable, neversink.ExceptionalBases(string(baseContent))))
		} else {
			e.logf("[Uyarı] Exceptional taban listesi alınamadı: %v", berr)
		}
	}

	e.setStep(0.7, "Kurallar üretiliyor")
	block, st := filter.GenerateDynamicFilterBlock(cfg, snap, validBases)

	e.setStep(0.85, "Filtre yazılıyor")
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
	e.st.Step, e.st.Progress = "Tamamlandı", 1
	e.stMu.Unlock()
	e.logf("Filtre yazıldı: %d değerli currency, %d unique taban, %d exceptional", st.ValuableCurrency, st.ValuableUniques, st.ValuableExcept)

	if cfg.NotifyEnabled && e.opt.Notify != nil {
		e.opt.Notify("Filtre güncellendi", fmt.Sprintf("%s.filter hazır. Oyunda Item Filter → Reload yap.", cfg.FilterName))
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

	if scanner != nil {
		ss := scanner.Status()
		s.Scan = ScanState{Enabled: scanning, Candidates: ss.Candidates, Keys: ss.Keys,
			Scanned: ss.Scanned, Valuable: ss.Valuable, Current: ss.Current, Last: ss.Last,
			NextAtMs: ss.NextAt, EtaSec: ss.EtaSec}
	}
	return s
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

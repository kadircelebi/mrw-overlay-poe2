package app

import (
	"context"
	"fmt"
	"image"
	"log"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"poe2filter/internal/collector"
	"poe2filter/internal/filter"
	"poe2filter/internal/overlay"
	"poe2filter/internal/prices"
)

// expeditionShowFor is how long the price labels stay over the game. The
// panel's closing cannot be seen without watching the screen, which the
// feature never does; a click anywhere, Escape or a second press of the
// shortcut hides them sooner.
const expeditionShowFor = 30 * time.Second

// expeditionNoticeFor is how long "no panel found" stays up.
const expeditionNoticeFor = 2 * time.Second

// ExpeditionPrice is one reward of the Runeshape panel as the labels show
// it. Y and H are the reward text's line in the game's client area, in
// physical pixels. ValueEx is the reward's worth (the unit's when the count
// is unknown), 0 when it has no price; Below marks a worth under the loot
// filter's threshold. Pending marks an uncut gem whose price is still being
// fetched. Bg, Color and Border are the loot filter's colours for an item of
// this worth (see valueStyle), as CSS; empty when no value group takes it.
type ExpeditionPrice struct {
	Name string `json:"name"`
	// Display is Name as the game's language writes it ("" in English).
	Display    string  `json:"display,omitempty"`
	Text       string  `json:"text"`
	Count      int     `json:"count"`
	CountKnown bool    `json:"countKnown"`
	ValueEx    float64 `json:"valueEx"`
	Below      bool    `json:"below"`
	Pending    bool    `json:"pending"`
	Bg         string  `json:"bg"`
	Color      string  `json:"color"`
	Border     string  `json:"border"`
	Y          float64 `json:"y"`
	H          float64 `json:"h"`
}

// ExpeditionView is what the label window shows: the rows, where the panel
// ends on the right (physical pixels of the game's client area) and the
// Divine rate for the amounts. Message is "notFound" when no panel was read,
// "error" when the screen could not be read. Seq changes with every read.
type ExpeditionView struct {
	Rows       []ExpeditionPrice `json:"rows"`
	PanelRight float64           `json:"panelRight"`
	DivineEx   float64           `json:"divineEx"`
	Message    string            `json:"message"`
	Seq        int               `json:"seq"`
}

type expeditionState struct {
	mu    sync.Mutex
	view  ExpeditionView
	timer *time.Timer
	// gems are poe2scout's uncut gem prices by name (the shared price list
	// has none), fetched at gemsAt and reused a while.
	gems   map[string]float64
	gemsAt time.Time
	// threshold is the loot filter's value threshold at the last read.
	threshold float64
}

// uncutGemsFor is how long poe2scout's uncut gem prices are reused.
const uncutGemsFor = 30 * time.Minute

// GetExpeditionView returns the labels last read, for a window that opens
// after the read.
func (s *AppService) GetExpeditionView() ExpeditionView {
	s.expedition.mu.Lock()
	defer s.expedition.mu.Unlock()
	return s.expedition.view
}

// captureExpedition reads the Runeshape panel once and lays the prices over
// the game; pressed again while they show, it hides them.
func (s *AppService) captureExpedition() {
	s.overlayMu.RLock()
	enabled := s.overlaySettings.Enabled
	s.overlayMu.RUnlock()
	if !enabled || s.expeditionWindow == nil {
		return
	}
	if s.expeditionWindow.IsVisible() {
		s.hideExpedition()
		return
	}
	view, client, gems := s.readExpedition()
	s.expedition.mu.Lock()
	view.Seq = s.expedition.view.Seq + 1
	s.expedition.view = view
	if s.expedition.timer != nil {
		s.expedition.timer.Stop()
	}
	showFor := expeditionShowFor
	if view.Message != "" {
		showFor = expeditionNoticeFor
	}
	s.expedition.timer = time.AfterFunc(showFor, s.hideExpedition)
	s.expedition.mu.Unlock()
	s.app.Event.Emit("expedition-view", view)
	if len(gems) > 0 {
		go s.priceExpeditionGems(view.Seq, gems)
	}

	if client.Empty() {
		return
	}
	s.expeditionWindow.Show()
	overlay.ShowOver(uintptr(s.expeditionWindow.NativeWindow()), client)
	// The window is made not to take focus; should Windows give it anyway,
	// the game gets it back so the next keys still reach it.
	if !overlay.IsGameWindow(overlay.ForegroundWindow()) {
		overlay.FocusGame()
	}
	go s.hideExpeditionOnClick(view.Seq)
}

// hideExpeditionOnClick hides the labels of read seq at the next click or
// Escape (choosing a reward changes the panel). The window lets clicks
// through, so the buttons are polled while it shows; a button already down
// when the labels appear counts only once released.
func (s *AppService) hideExpeditionOnClick(seq int) {
	held := overlay.ClickOrEscape()
	shown, start := false, time.Now()
	for range time.Tick(30 * time.Millisecond) {
		s.expedition.mu.Lock()
		current := s.expedition.view.Seq == seq
		s.expedition.mu.Unlock()
		visible := s.expeditionWindow.IsVisible()
		shown = shown || visible
		// Showing the window may take a moment; once shown, hidden is done.
		if !current || !visible && (shown || time.Since(start) > time.Second) {
			return
		}
		down := overlay.ClickOrEscape()
		if down && !held {
			s.hideExpedition()
			return
		}
		held = down
	}
}

func (s *AppService) hideExpedition() {
	if s.expeditionWindow != nil && s.expeditionWindow.IsVisible() {
		s.expeditionWindow.Hide()
	}
}

// readExpedition reads the panel and prices its rewards; client is the
// game's client area on the screen (empty when the game is not found). gems
// are the uncut gem rows (row index to gem name) left to price once
// poe2scout's prices arrive.
func (s *AppService) readExpedition() (view ExpeditionView, client image.Rectangle, gems map[int]string) {
	catalog, err := s.overlayCatalog.Load(context.Background())
	if err != nil {
		log.Printf("expedition: catalog: %v", err)
		view.Message = "error"
		return view, client, nil
	}
	var names []string
	for _, group := range catalog.Items {
		if group.ID == "currency" {
			for _, entry := range group.Entries {
				names = append(names, entry.Type)
			}
		}
	}
	loc := screenLocale()
	if loc != nil {
		// Local names, and the English ones the game shows while Alt is held.
		names = append(loc.NamesFor(names), names...)
	}
	rows, client, err := overlay.ReadRunePanel(names)
	// Prices and recipes go by the English names; the label keeps the
	// name the game shows.
	display := make([]string, len(rows))
	if loc != nil {
		for i := range rows {
			if en, ok := loc.EnglishName(rows[i].Name); ok {
				display[i] = rows[i].Name
				rows[i].Name = en
			}
		}
	}
	if err != nil {
		log.Printf("expedition: screen text: %v", err)
		view.Message = "error"
		return view, client, nil
	}
	if len(rows) == 0 {
		view.Message = "notFound"
		return view, client, nil
	}
	areaLevel := 0
	if path, _ := s.gameLog.Load().(string); path != "" {
		areaLevel, _ = overlay.LastArea(path)
	}
	overlay.ResolveRuneCounts(rows, areaLevel)
	threshold := 0.0
	if last := s.eng.State().Last; last != nil {
		threshold = last.ThresholdEx
	}
	s.expedition.mu.Lock()
	s.expedition.threshold = threshold
	s.expedition.mu.Unlock()
	snap := s.eng.Prices()
	view.DivineEx = overlay.QuoteCurrency(snap, "Divine Orb").DivineEx
	style := s.expeditionStyler(snap)
	gems = map[int]string{}
	s.expedition.mu.Lock()
	gemPrices := s.expedition.gems
	if time.Since(s.expedition.gemsAt) > uncutGemsFor {
		gemPrices = nil
	}
	s.expedition.mu.Unlock()
	var rights []float64
	for i, row := range rows {
		price := ExpeditionPrice{Name: row.Name, Display: display[i], Text: row.Text, Count: row.Count, CountKnown: row.CountRead, Y: row.Y, H: row.H}
		if row.Name != "" {
			if q := overlay.QuoteCurrency(snap, row.Name); q.Found {
				price.ValueEx = q.ValueEx
				if row.CountRead {
					price.ValueEx *= float64(row.Count)
				}
				price.Below = threshold > 0 && price.ValueEx < threshold
				style(&price)
			}
		} else if name, ok := overlay.UncutGem(row.Text); ok {
			price.Name = name
			// The shared price list carries skill and spirit gems by level
			// once the scan servers collect them; poe2scout fills in until
			// then, and for support gems.
			if q := overlay.QuoteCurrency(snap, name); q.Found {
				price.ValueEx = q.ValueEx * float64(max(1, row.Count))
				price.Below = threshold > 0 && price.ValueEx < threshold
				style(&price)
			} else if gemPrices != nil {
				price.ValueEx = gemPrices[name] * float64(max(1, row.Count))
				price.Below = price.ValueEx > 0 && threshold > 0 && price.ValueEx < threshold
				style(&price)
			} else {
				price.Pending = true
				gems[i] = name
			}
		}
		view.Rows = append(view.Rows, price)
		rights = append(rights, row.Right)
	}
	slices.Sort(rights)
	view.PanelRight = rights[len(rights)/2]
	return view, client, gems
}

// priceExpeditionGems fetches poe2scout's uncut gem prices for the gem rows
// of read seq and puts them on their labels.
func (s *AppService) priceExpeditionGems(seq int, gems map[int]string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	prices, err := collector.FetchScoutUncutGems(ctx, &http.Client{Timeout: 30 * time.Second}, s.eng.Config().LeagueName)
	if err != nil {
		log.Printf("expedition: uncut gem prices: %v", err)
	}
	style := s.expeditionStyler(s.eng.Prices())
	s.expedition.mu.Lock()
	if err == nil {
		s.expedition.gems, s.expedition.gemsAt = prices, time.Now()
	}
	if s.expedition.view.Seq != seq {
		s.expedition.mu.Unlock()
		return
	}
	for i, name := range gems {
		if i >= len(s.expedition.view.Rows) {
			continue
		}
		row := &s.expedition.view.Rows[i]
		row.Pending = false
		row.ValueEx = prices[name] * float64(max(1, row.Count))
		row.Below = row.ValueEx > 0 && s.expedition.threshold > 0 && row.ValueEx < s.expedition.threshold
		style(row)
	}
	view := s.expedition.view
	view.Rows = slices.Clone(view.Rows)
	s.expedition.mu.Unlock()
	s.app.Event.Emit("expedition-view", view)
}

// expeditionStyler colours a priced label as the loot filter would show an
// item of that worth.
func (s *AppService) expeditionStyler(snap *prices.Snapshot) func(*ExpeditionPrice) {
	cfg, ns := s.eng.Config(), s.eng.NeverSinkStyles()
	var rates prices.Rates
	if snap != nil {
		rates = snap.Rates
	}
	return func(p *ExpeditionPrice) {
		if t, ok := valueStyle(cfg, ns, rates, p.Name, p.ValueEx); ok {
			p.Bg, p.Color, p.Border = cssColor(t.BgColor), cssColor(t.TextColor), cssColor(t.Border)
		}
	}
}

// valueStyle is the look the loot filter gives an item worth valueEx: that
// of the highest value group whose threshold it reaches (the filter writes
// them highest first, and the first match wins). Divine Orb keeps its own
// group's look, as in the filter.
func valueStyle(cfg filter.Config, ns map[string]filter.Theme, rates prices.Rates, name string, valueEx float64) (filter.Theme, bool) {
	if name == "Divine Orb" {
		t, _ := cfg.Palette(filter.GroupDivine, ns)
		return t, true
	}
	if valueEx <= 0 {
		return filter.Theme{}, false
	}
	best, bestEx := -1, -1.0
	for i, g := range cfg.ItemGroups {
		if g.GroupMode() != filter.ItemGroupModeValue {
			continue
		}
		if ex := g.ThresholdEx(rates); ex > 0 && valueEx >= ex && ex > bestEx {
			best, bestEx = i, ex
		}
	}
	if best < 0 {
		return filter.Theme{}, false
	}
	t, _ := cfg.Palette(cfg.ItemGroups[best].StyleKey(), ns)
	return t, true
}

// cssColor turns a filter colour ("255 215 0 255") into CSS; "" when unset.
func cssColor(rgba string) string {
	f := strings.Fields(rgba)
	if len(f) < 3 {
		return ""
	}
	alpha := 1.0
	if len(f) > 3 {
		if a, err := strconv.Atoi(f[3]); err == nil {
			alpha = float64(a) / 255
		}
	}
	return fmt.Sprintf("rgba(%s,%s,%s,%.2f)", f[0], f[1], f[2], alpha)
}

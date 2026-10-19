package app

import (
	"log"
	"time"

	"poe2filter/internal/lootlog"
	"poe2filter/internal/overlay"
)

// The loot log keeps what each map brought, from the inventory read before
// it and after it (the inventory scan's shortcut, in a hideout or town).
// The map comes from the game's log, as for the Expedition history.

// LootMap is one map as the tracker window shows it; Name is its name made
// readable from its id.
type LootMap struct {
	lootlog.Map
	Name string `json:"map"`
}

// LootLogView is what the tracker window's loot tab shows. Waiting names
// the map whose run waits for a read in the hideout ("" when none). The
// totals count like the Expedition tab's, but the session from the player's
// restart of it when later than the game's start (Restarted), with its
// figures (Farm). Box tells the
// loot box is on, Placing that it is being dragged.
type LootLogView struct {
	League       string        `json:"league"`
	Mode         string        `json:"mode"`
	Maps         []LootMap     `json:"maps"`
	Waiting      string        `json:"waiting"`
	SessionStart int64         `json:"sessionStart"`
	Session      lootlog.Total `json:"session"`
	Today        lootlog.Total `json:"today"`
	LeagueTotal  lootlog.Total `json:"leagueTotal"`
	DivineEx     float64       `json:"divineEx"`
	Restarted    bool          `json:"restarted"`
	Farm         FarmView      `json:"farm"`
	Box          bool          `json:"box"`
	Placing      bool          `json:"placing"`
}

// InvScanMap is the map a read closed a run of, as the worth card shows it:
// what this run brought and all the map's runs so far.
type InvScanMap struct {
	Name   string  `json:"name"`
	RunEx  float64 `json:"runEx"`
	MapEx  float64 `json:"mapEx"`
	Runs   int     `json:"runs"`
	Fresh  bool    `json:"fresh"`
	NoBase bool    `json:"noBase"`
	Unsure int     `json:"unsure"`
}

func lootArea(a overlay.Area) lootlog.Area {
	return lootlog.Area{ID: a.ID, Level: a.Level, Seed: a.Seed, Rest: a.Rest()}
}

// lootAreaEntered follows the game into an area, while the inventory scan
// is on.
func (s *AppService) lootAreaEntered(a overlay.Area) {
	if mode, _ := s.invScanMode(); mode == overlay.ScanOff {
		return
	}
	if !a.Rest() {
		// Reads still being gathered back from the last map count before
		// the next one begins.
		s.closeAfterIfDue(true)
	}
	if err := s.lootLog.Entered(s.overlayClient.League(), lootArea(a), time.Now()); err != nil {
		log.Printf("loot log: %v", err)
	}
	s.lootChanged()
}

// placeLoot tells the loot log where the player is, from the game's log,
// when the app has not followed them yet.
func (s *AppService) placeLoot() {
	if s.lootLog.Known() {
		return
	}
	if path, _ := s.gameLog.Load().(string); path != "" {
		if a, ok := overlay.CurrentArea(path); ok {
			s.lootLog.Place(lootArea(a))
		}
	}
}

// keepRead files a read of the inventory (items by name, cells not read
// for sure) in the loot log: in a hideout or town it is the next map's
// base, and the first one after a map closes that map's run, returned.
func (s *AppService) keepRead(items map[string]int, unsure int, doubts []lootlog.Doubt) *InvScanMap {
	snap := s.eng.Prices()
	worth := func(name string) float64 { return overlay.QuoteCurrency(snap, name).ValueEx }
	m, r, err := s.lootLog.Scanned(s.overlayClient.League(), lootlog.Scan{At: time.Now(), Items: items, Unsure: unsure, Doubts: doubts}, worth)
	if err != nil {
		log.Printf("loot log: %v", err)
	}
	s.lootChanged()
	if m == nil || r == nil {
		return nil
	}
	return &InvScanMap{Name: mapName(m.AreaID), RunEx: r.ValueEx, MapEx: m.ValueEx, Runs: len(m.Runs), Fresh: r.Fresh, NoBase: r.NoBase, Unsure: r.Unsure}
}

// readItems are the card's items by name.
func readItems(view *InvScanView) map[string]int {
	items := map[string]int{}
	for _, r := range view.Rows {
		items[r.Name] += r.Count
	}
	return items
}

// recordInventory files a read made with the shortcut: kept in a hideout
// or town (shown on the card), kept for nothing in a map.
func (s *AppService) recordInventory(view *InvScanView) {
	s.placeLoot()
	if !s.lootLog.InRest() {
		view.InMap = s.lootLog.Known()
		return
	}
	view.Base = true
	view.Map = s.keepRead(readItems(view), view.Unsure, view.Doubts)
}

// GetLootLog returns the current league's maps.
func (s *AppService) GetLootLog() LootLogView {
	league := s.overlayClient.League()
	start, farm := s.lootSessionStart()
	times := s.farmTimes(league, start)
	list := s.lootLog.Maps(league)
	farmFill(&times, list, start)
	mode, _ := s.invScanMode()
	v := LootLogView{Farm: times, League: league, Mode: mode, Maps: make([]LootMap, len(list)), Box: s.lootBoxOn(), Placing: s.lootPlacing.Load()}
	for i, m := range list {
		v.Maps[i] = LootMap{Map: m, Name: mapName(m.AreaID)}
	}
	if id, _, ok := s.lootLog.Waiting(league); ok {
		v.Waiting = mapName(id)
	}
	if snap := s.eng.Prices(); snap != nil {
		v.DivineEx = overlay.QuoteCurrency(snap, "Divine Orb").DivineEx
	}
	now := time.Now()
	v.SessionStart, v.Restarted = start.UnixMilli(), farm
	v.Session = lootlog.Sum(list, start)
	v.Today = lootlog.Sum(list, time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()))
	v.LeagueTotal = lootlog.Sum(list, time.Time{})
	return v
}

// DeleteLootMap forgets the map first entered at at (RFC 3339, as the view
// gives it).
func (s *AppService) DeleteLootMap(at string) LootLogView {
	if t, err := time.Parse(time.RFC3339Nano, at); err == nil {
		if err := s.lootLog.Delete(s.overlayClient.League(), t); err != nil {
			log.Printf("loot log: delete: %v", err)
		}
	}
	s.lootChanged()
	return s.GetLootLog()
}

// ClearLootLog forgets every map and read.
func (s *AppService) ClearLootLog() (LootLogView, error) {
	err := s.lootLog.Clear()
	s.lootChanged()
	return s.GetLootLog(), err
}

func (s *AppService) lootChanged() {
	if s.app != nil {
		s.app.Event.Emit("loot-log", s.GetLootLog())
	}
	s.emitLootStatus()
}

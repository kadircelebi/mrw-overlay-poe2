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

// LootLogView is what the tracker window's loot tab shows: the league's
// sessions, newest first, and the maps from before the first (Before).
// Waiting names the map whose run waits for a read in the hideout ("" when
// none). Today counts from local midnight, LeagueTotal all the maps. Box
// tells the loot box is on, Placing that it is being dragged.
type LootLogView struct {
	League      string            `json:"league"`
	Mode        string            `json:"mode"`
	Sessions    []LootSessionView `json:"sessions"`
	Before      []LootMap         `json:"before"`
	BeforeTotal lootlog.Total     `json:"beforeTotal"`
	Waiting     string            `json:"waiting"`
	Today       lootlog.Total     `json:"today"`
	LeagueTotal lootlog.Total     `json:"leagueTotal"`
	DivineEx    float64           `json:"divineEx"`
	Box         bool              `json:"box"`
	Placing     bool              `json:"placing"`
}

// LootSessionView is one session as the tracker shows it: N is its number
// in the league (from 1), Start and End Unix ms (End 0 while Open), CostEx
// what went into it (0: not told), Total what its maps brought, Farm its
// figures and Maps its maps, newest first.
type LootSessionView struct {
	ID     int64         `json:"id"`
	N      int           `json:"n"`
	Name   string        `json:"name"`
	Start  int64         `json:"start"`
	End    int64         `json:"end"`
	Open   bool          `json:"open"`
	CostEx float64       `json:"costEx"`
	Total  lootlog.Total `json:"total"`
	Farm   FarmView      `json:"farm"`
	Maps   []LootMap     `json:"maps"`
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

// lootSessions are league's sessions (oldest first) with their maps, the
// maps from before the first and all of them, after the game's log is read
// on.
func (s *AppService) lootSessions(league string) ([]lootlog.Session, [][]lootlog.Map, []lootlog.Map, []lootlog.Map) {
	s.followLog()
	sessions := s.lootLog.Sessions(league)
	list := s.lootLog.Maps(league)
	by, before := lootlog.Group(sessions, list)
	return sessions, by, before, list
}

func lootMaps(list []lootlog.Map) []LootMap {
	out := make([]LootMap, len(list))
	for i, m := range list {
		out[i] = LootMap{Map: m, Name: mapName(m.AreaID)}
	}
	return out
}

func (s *AppService) sessionView(league string, n int, sess lootlog.Session, maps []lootlog.Map, now time.Time, running bool) LootSessionView {
	v := LootSessionView{ID: sess.ID(), N: n, Name: sess.Name, Start: sess.Start.UnixMilli(), Open: sess.Open(), Total: lootlog.Sum(maps, time.Time{}), Maps: lootMaps(maps)}
	if !sess.Open() {
		v.End = sess.End.UnixMilli()
	}
	if sess.Cost != nil {
		v.CostEx = sess.Cost.ValueEx
	}
	v.Farm = s.farmTimes(league, sess, maps, now, running)
	return v
}

// GetLootLog returns the current league's sessions and maps.
func (s *AppService) GetLootLog() LootLogView {
	league := s.overlayClient.League()
	sessions, by, before, list := s.lootSessions(league)
	mode, _ := s.invScanMode()
	v := LootLogView{League: league, Mode: mode, Before: lootMaps(before), BeforeTotal: lootlog.Sum(before, time.Time{}), Box: s.lootBoxOn(), Placing: s.lootPlacing.Load()}
	now, running := s.farmNow()
	for i := len(sessions) - 1; i >= 0; i-- {
		v.Sessions = append(v.Sessions, s.sessionView(league, i+1, sessions[i], by[i], now, running))
	}
	if id, _, ok := s.lootLog.Waiting(league); ok {
		v.Waiting = mapName(id)
	}
	if snap := s.eng.Prices(); snap != nil {
		v.DivineEx = overlay.QuoteCurrency(snap, "Divine Orb").DivineEx
	}
	today := time.Now()
	v.Today = lootlog.Sum(list, time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, today.Location()))
	v.LeagueTotal = lootlog.Sum(list, time.Time{})
	return v
}

// StartLootSession ends the open session now and begins another: the maps
// stay with the session they were entered in.
func (s *AppService) StartLootSession() LootLogView {
	s.followLog()
	if err := s.lootLog.NewSession(s.overlayClient.League(), time.Now()); err != nil {
		log.Printf("loot log: new session: %v", err)
	}
	s.lootChanged()
	return s.GetLootLog()
}

// RenameLootSession names session id ("": its number again).
func (s *AppService) RenameLootSession(id int64, name string) LootLogView {
	return s.editSession(s.lootLog.RenameSession(s.overlayClient.League(), id, name))
}

// SetLootSessionCost keeps what went into session id, in Exalted (0:
// nothing).
func (s *AppService) SetLootSessionCost(id int64, valueEx float64) LootLogView {
	return s.editSession(s.lootLog.SetSessionCost(s.overlayClient.League(), id, &lootlog.Cost{ValueEx: valueEx}))
}

// MergeLootSession joins session id to the one before it.
func (s *AppService) MergeLootSession(id int64) LootLogView {
	s.followLog()
	return s.editSession(s.lootLog.MergeSession(s.overlayClient.League(), id))
}

// DeleteLootSession forgets session id and its maps; the open one begins
// again now.
func (s *AppService) DeleteLootSession(id int64) LootLogView {
	s.followLog()
	return s.editSession(s.lootLog.DeleteSession(s.overlayClient.League(), id, time.Now()))
}

func (s *AppService) editSession(ok bool, err error) LootLogView {
	if err != nil {
		log.Printf("loot log: session: %v", err)
	}
	if ok {
		s.lootChanged()
	}
	return s.GetLootLog()
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

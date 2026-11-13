package app

import (
	"time"

	"poe2filter/internal/filter"
	"poe2filter/internal/overlay"
)

// The game reads its item filter again only when told to. With the setting
// on, a newly written filter is reloaded through the chat line
// /reloaditemfilter, typed in as the game commands are, once the next
// loading screen is over: the player is not typing into the chat then, and
// the chat opens and closes before the area can be played. The line reloads
// whichever filter the game has selected.
const reloadFilterLine = "/reloaditemfilter"

// reloadSettle is how long the line waits after a loading screen, until the
// game takes keys again.
const reloadSettle = time.Second

// filterWritten notes a newly written filter for the game to reload.
func (s *AppService) filterWritten(cfg filter.Config) {
	if !cfg.ReloadInGame {
		s.reloadDue.Store(0)
		return
	}
	s.reloadDue.Store(time.Now().UnixNano())
	// The game may be in front already, with nothing else reading its log.
	if path, _ := s.gameLog.Load().(string); path != "" {
		s.whispers.Follow(path)
	}
}

// loadingScreenOver sends the reload line for a filter written before the
// loading screen that just ended.
func (s *AppService) loadingScreenOver() {
	due := s.reloadDue.Load()
	if due == 0 {
		return
	}
	if !reloadStillDue(due, s.whispers.SessionStart()) || !s.eng.Config().ReloadInGame {
		s.reloadDue.CompareAndSwap(due, 0)
		return
	}
	go func() {
		time.Sleep(reloadSettle)
		// A game in the background is left alone; the next loading screen
		// tries again.
		if !overlay.IsGameWindow(overlay.ForegroundWindow()) {
			return
		}
		if s.reloadDue.CompareAndSwap(due, 0) {
			s.sendChatCommand(reloadFilterLine)
		}
	}()
}

// reloadStillDue tells whether a filter written at due (Unix nanoseconds)
// still has to be reloaded: a game started after it read it as it began.
func reloadStillDue(due int64, gameStart time.Time) bool {
	return gameStart.IsZero() || gameStart.UnixNano() <= due
}

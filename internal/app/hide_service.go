package app

import (
	"errors"
	"strings"
	"time"

	"poe2filter/internal/filter"
	"poe2filter/internal/i18n"
)

// HideResult tells the hide window what happened.
type HideResult struct {
	Hidden []filter.HiddenItem `json:"hidden"`
	// Updating is true when the filter is being rewritten now; false when an
	// update was already running (the entry applies with the next one).
	Updating bool `json:"updating"`
}

// HideItem adds an entry to the "hidden by me" list (Alt+H) and rewrites the
// filter. An entry with the same base and limits is replaced.
func (s *AppService) HideItem(entry filter.HiddenItem) (HideResult, error) {
	entry.Base = strings.TrimSpace(entry.Base)
	if entry.Base == "" {
		return HideResult{}, errors.New(i18n.T("hide.noBase"))
	}
	entry.AddedAt = time.Now().UnixMilli()
	cfg := s.eng.Config()
	cfg.HiddenItems = append(cfg.HiddenItems, entry)
	cfg.HiddenOff = false
	return s.saveHidden(cfg)
}

// UnhideItem removes the entry with the same base and limits and rewrites
// the filter.
func (s *AppService) UnhideItem(entry filter.HiddenItem) (HideResult, error) {
	cfg := s.eng.Config()
	key := filter.HiddenKey(entry)
	kept := cfg.HiddenItems[:0:0]
	for _, h := range cfg.HiddenItems {
		if h.Key() != key {
			kept = append(kept, h)
		}
	}
	cfg.HiddenItems = kept
	return s.saveHidden(cfg)
}

func (s *AppService) saveHidden(cfg filter.Config) (HideResult, error) {
	saved, err := s.SaveConfig(cfg)
	if err != nil {
		return HideResult{}, err
	}
	res := HideResult{Hidden: saved.HiddenItems}
	res.Updating = s.eng.UpdateNow() == nil
	return res, nil
}

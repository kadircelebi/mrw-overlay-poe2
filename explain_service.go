package main

import (
	"context"
	"errors"
	"os"
	"time"

	"poe2filter/internal/filtereval"
	"poe2filter/internal/i18n"
	"poe2filter/internal/overlay"
)

// FilterExplanation is what the written filter does with an item and why.
type FilterExplanation struct {
	filtereval.Result
	Verdict filtereval.Verdict `json:"verdict"`
	// FilterPath and WrittenAt describe the file the answer comes from; the
	// game may still use an older copy until the filter is reloaded.
	FilterPath string    `json:"filterPath"`
	WrittenAt  time.Time `json:"writtenAt"`
	// AreaLevel and Area are the last area of this game session (hideouts
	// and towns skipped), used for NeverSink's AreaLevel rules; 0 = unknown.
	AreaLevel int    `json:"areaLevel"`
	Area      string `json:"area"`
}

type explainCache struct {
	path    string
	modTime time.Time
	blocks  []filtereval.Block
}

// ExplainItem tells which rule of the written filter decides the item in
// the copied text (Alt+E's), and whether that is certain.
func (s *AppService) ExplainItem(raw string) (FilterExplanation, error) {
	catalog, err := s.overlayCatalog.Load(context.Background())
	if err != nil {
		return FilterExplanation{}, err
	}
	item, err := overlay.ParseItem(raw, catalog)
	if err != nil {
		return FilterExplanation{}, err
	}
	path, err := s.eng.FilterPath()
	if err != nil {
		return FilterExplanation{}, err
	}
	blocks, written, err := s.filterBlocks(path)
	if err != nil {
		return FilterExplanation{}, err
	}
	facts := filtereval.FactsFromItem(item)
	out := FilterExplanation{FilterPath: path, WrittenAt: written}
	if log, _ := s.gameLog.Load().(string); log != "" {
		out.AreaLevel, out.Area = overlay.LastArea(log)
		facts.AreaLevel = out.AreaLevel
	}
	out.Result = filtereval.Evaluate(blocks, facts)
	out.Verdict = filtereval.VerdictOf(out.Result)
	return out, nil
}

// filterBlocks parses the filter file, again only when it changed.
func (s *AppService) filterBlocks(path string) ([]filtereval.Block, time.Time, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, time.Time{}, errors.New(i18n.T("explain.noFilter"))
	}
	s.explainMu.Lock()
	defer s.explainMu.Unlock()
	c := s.explainCache
	if c.path == path && c.modTime.Equal(info.ModTime()) {
		return c.blocks, c.modTime, nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, time.Time{}, err
	}
	blocks := filtereval.Parse(string(raw))
	s.explainCache = explainCache{path: path, modTime: info.ModTime(), blocks: blocks}
	return blocks, info.ModTime(), nil
}

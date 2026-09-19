package engine

import (
	"os"
	"path/filepath"
	"strings"
	"sync"

	"poe2filter/internal/filter"
	"poe2filter/internal/neversink"
)

// nsCache holds the NeverSink styles of the last parsed base filter.
type nsCache struct {
	mu    sync.Mutex
	path  string
	list  []filter.Theme
	byTag map[string]filter.Theme
}

func nsThemes(content string) ([]filter.Theme, map[string]filter.Theme) {
	styles := neversink.Styles(content)
	list := make([]filter.Theme, 0, len(styles))
	byTag := make(map[string]filter.Theme, len(styles))
	for _, s := range styles {
		t := filter.Theme{
			ID:        filter.NeverSinkThemePrefix + s.Tag,
			Label:     strings.ToUpper(s.Name),
			Category:  s.Category,
			Count:     s.Count,
			BgColor:   s.Bg,
			TextColor: s.Text,
			Border:    s.Border,
			Beam:      s.Effect,
			Icon:      s.IconColor,
			Shape:     s.IconShape,
			Full:      true,
		}
		list = append(list, t)
		byTag[s.Tag] = t
	}
	return list, byTag
}

// set parses content every time: NeverSink is re-downloaded to the same path,
// so the path alone does not identify the styles.
func (c *nsCache) set(path, content string) map[string]filter.Theme {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.list, c.byTag = nsThemes(content)
	c.path = path
	return c.byTag
}

// NeverSinkThemes lists the named styles of the current base filter. It only
// reads files already on disk; nothing is downloaded.
func (e *Engine) NeverSinkThemes() []filter.Theme {
	cfg := e.Config()
	path := cfg.CustomBaseFilter
	if path == "" {
		name, err := neversink.FileName(cfg.Strictness)
		if err != nil {
			return nil
		}
		path = filepath.Join(e.dataDir, "neversink", name)
	}
	e.ns.mu.Lock()
	cached := e.ns.path == path && e.ns.list != nil
	list := e.ns.list
	e.ns.mu.Unlock()
	if cached {
		return list
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return []filter.Theme{}
	}
	e.ns.set(path, string(content))
	e.ns.mu.Lock()
	defer e.ns.mu.Unlock()
	return e.ns.list
}

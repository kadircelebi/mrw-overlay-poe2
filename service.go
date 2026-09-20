package main

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"poe2filter/internal/engine"
	"poe2filter/internal/filter"
	"poe2filter/internal/i18n"
	"poe2filter/internal/insights"
)

// LanguageOption is one entry of the language picker.
type LanguageOption struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	// Auto marks the entry that follows the Windows display language. Label is
	// the language that choice currently resolves to, Resolved its code.
	Auto     bool   `json:"auto"`
	Resolved string `json:"resolved,omitempty"`
}

// StyleOptions are the choices offered in the custom style editor.
type StyleOptions struct {
	Colours []string          `json:"colours"`
	Shapes  []string          `json:"shapes"`
	Preset  map[string]string `json:"preset"` // group -> NeverSink style tag
}

// Meta is static information for the UI.
type Meta struct {
	Version  string `json:"version"`
	DataDir  string `json:"dataDir"`
	GameDir  string `json:"gameDir"`
	TestMode bool   `json:"testMode"` // writing somewhere other than the game folder
	// MaxItemGroups is how many of their own groups a user may keep.
	MaxItemGroups int `json:"maxItemGroups"`
}

// AppService is the API the panel calls. Its methods are exposed to the
// frontend through generated bindings.
type AppService struct {
	eng     *engine.Engine
	app     *application.App
	tray    *application.SystemTray
	meta    Meta
	signal  chan struct{}
	started sync.Once
	// relabel rebuilds the tray menu after a language change; the menu is
	// created once at start, so its labels do not follow i18n on their own.
	relabel func()
}

func newAppService(meta Meta) *AppService {
	meta.MaxItemGroups = filter.MaxItemGroups
	return &AppService{meta: meta, signal: make(chan struct{}, 1)}
}

// changed is the engine's OnChange hook. It never blocks and never touches
// engine locks; the pump goroutine does the actual work.
func (s *AppService) changed() {
	select {
	case s.signal <- struct{}{}:
	default:
	}
}

// ServiceStartup starts the engine once the application is running.
func (s *AppService) ServiceStartup(ctx context.Context, _ application.ServiceOptions) error {
	s.started.Do(func() {
		s.eng.Start()
		go s.pump()
	})
	return nil
}

// ServiceShutdown stops background work.
func (s *AppService) ServiceShutdown() error {
	s.eng.Stop()
	return nil
}

// pump coalesces change signals and pushes state to the panel and tray.
func (s *AppService) pump() {
	var lastTip string
	for range s.signal {
		time.Sleep(200 * time.Millisecond) // coalesce bursts
		st := s.eng.State()
		s.app.Event.Emit("state", st)

		tip := "PoE2 Filtre"
		switch {
		case st.Running:
			tip += i18n.T("tray.updating")
		case st.LastError != "":
			tip += i18n.T("tray.lastFailed")
		case st.LastRunAtMs > 0:
			tip += i18n.T("tray.lastSuccess") + time.UnixMilli(st.LastRunAtMs).Format("15:04")
		}
		if tip != lastTip && s.tray != nil {
			s.tray.SetTooltip(tip)
			lastTip = tip
		}
	}
}

// GetMeta returns static app information.
func (s *AppService) GetMeta() Meta { return s.meta }

// GetState returns the current engine state.
func (s *AppService) GetState() engine.State { return s.eng.State() }

// GetConfig returns the current settings.
func (s *AppService) GetConfig() filter.Config { return s.eng.Config() }

// SaveConfig stores new settings and returns them normalised.
func (s *AppService) SaveConfig(c filter.Config) (filter.Config, error) {
	before := i18n.Current()
	saved, err := s.eng.SetConfig(c)
	if err != nil {
		return saved, err
	}
	if lang := i18n.Resolve(saved.Language); lang != before {
		i18n.Set(lang)
		if s.relabel != nil {
			s.relabel()
		}
	}
	return saved, nil
}

// Languages lists the interface languages, each named in its own language.
// The first entry follows the Windows display language.
func (s *AppService) Languages() []LanguageOption {
	sys := i18n.Resolve(string(i18n.Auto))
	out := []LanguageOption{{ID: string(i18n.Auto), Label: i18n.Names[sys], Auto: true, Resolved: string(sys)}}
	for _, l := range i18n.Supported {
		out = append(out, LanguageOption{ID: string(l), Label: i18n.Names[l]})
	}
	return out
}

// Themes lists the selectable colour palettes.
func (s *AppService) Themes() []filter.Theme { return filter.LocalizedThemes() }

// NeverSinkThemes lists NeverSink's named styles from the current base filter.
func (s *AppService) NeverSinkThemes() []filter.Theme { return s.eng.NeverSinkThemes() }

// StyleOptions lists the colours and minimap shapes allowed in custom styles.
func (s *AppService) StyleOptions() StyleOptions {
	return StyleOptions{Colours: filter.EffectColours, Shapes: filter.IconShapes, Preset: filter.NeverSinkPreset}
}

// UserGroupTemplate is the look and sound a new user group starts from, so the
// panel can render its colour picker like the built-in groups.
func (s *AppService) UserGroupTemplate() filter.StyleGroup { return filter.UserGroupTemplate() }

// StyleGroups lists the drop groups whose colours can be changed.
func (s *AppService) StyleGroups() []filter.StyleGroup { return filter.LocalizedStyleGroups() }

// Leagues lists the leagues for the picker, live list first, always including
// the one currently configured.
func (s *AppService) Leagues() []string { return s.eng.Leagues() }

// UpdateNow starts a filter update in the background.
func (s *AppService) UpdateNow() error { return s.eng.UpdateNow() }

// SearchItems finds uniques, currency and bases for the custom lists.
func (s *AppService) SearchItems(query string) []insights.SearchItem {
	return s.eng.SearchItems(query, 12)
}

// OpenGameFolder opens the PoE2 filter folder in Explorer.
func (s *AppService) OpenGameFolder() error {
	if s.meta.GameDir == "" {
		return errors.New(i18n.T("err.gameDir"))
	}
	return exec.Command("explorer.exe", s.meta.GameDir).Start()
}

// OpenDataFolder opens the app's data folder in Explorer.
func (s *AppService) OpenDataFolder() error {
	if err := os.MkdirAll(s.meta.DataDir, 0o755); err != nil {
		return err
	}
	return exec.Command("explorer.exe", s.meta.DataDir).Start()
}

// HidePanel hides the tray panel.
func (s *AppService) HidePanel() {
	if s.tray != nil {
		s.tray.HideWindow()
	}
}

// Quit exits the application.
func (s *AppService) Quit() { s.app.Quit() }

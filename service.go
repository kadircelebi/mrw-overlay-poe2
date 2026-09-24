package main

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"poe2filter/internal/appupdate"
	"poe2filter/internal/engine"
	"poe2filter/internal/filter"
	"poe2filter/internal/i18n"
	"poe2filter/internal/insights"
	"poe2filter/internal/overlay"
	"poe2filter/internal/trade"
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
	// Sounds are the PlayAlertSound ids, so the panel and the filter writer
	// cannot drift apart about what the game accepts.
	Sounds []string `json:"sounds"`
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
	updater *appupdate.Manager
	app     *application.App
	tray    *application.SystemTray
	// panel is the tray window; file dialogs attach to it so the panel does not
	// disappear behind them when it loses focus.
	panel   application.Window
	meta    Meta
	signal  chan struct{}
	started sync.Once
	// relabel rebuilds the tray menu after a language change; the menu is
	// created once at start, so its labels do not follow i18n on their own.
	relabel func()
	// notifyAppUpdate is supplied by main after the platform notification
	// service exists. The manager remembers the last notified version.
	notifyAppUpdate func(string)
	updateCancel    context.CancelFunc

	// Overlay preferences are application-wide and intentionally separate from
	// filter profiles. The two windows share the latest captured item.
	overlayMu           sync.RWMutex
	overlaySettings     overlay.Settings
	overlaySettingsPath string
	overlayCatalog      *overlay.CatalogStore
	overlaySearches     *overlay.SearchStore
	overlayClient       *trade.Client
	overlayEvalMu       sync.Mutex
	overlayEvalCache    map[string]overlayEvaluationCacheEntry
	overlayEvalFlights  map[string]*overlayEvaluationFlight
	overlaySnapshot     overlay.Snapshot
	overlayDraft        trade.EvaluateRequest
	overlayWindow       application.Window
	marketWindow        application.Window
	overlayHotkey       string
	confineOnce         sync.Once
	rebindOverlay       func(old, next overlay.Settings) error
}

func newAppService(meta Meta) *AppService {
	meta.MaxItemGroups = filter.MaxItemGroups
	settingsPath := filepath.Join(meta.DataDir, "overlay.json")
	return &AppService{
		meta:                meta,
		signal:              make(chan struct{}, 1),
		overlaySettingsPath: settingsPath,
		overlaySettings:     overlay.LoadSettings(settingsPath),
		overlayCatalog:      overlay.NewCatalogStore(meta.DataDir),
		overlaySearches:     overlay.NewSearchStore(filepath.Join(meta.DataDir, "overlay_searches.json")),
		overlayClient:       trade.NewInteractiveClient(filter.LoadConfig(filepath.Join(meta.DataDir, "config.json")).LeagueName, 0.8),
		overlayEvalCache:    make(map[string]overlayEvaluationCacheEntry),
		overlayEvalFlights:  make(map[string]*overlayEvaluationFlight),
	}
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
		s.applyOverlayScale()
		go s.pump()
		go func() { _, _ = s.overlayCatalog.Load(ctx) }()
		if s.updater != nil {
			var updateCtx context.Context
			updateCtx, s.updateCancel = context.WithCancel(ctx)
			go func() {
				select {
				case <-time.After(5 * time.Second):
				case <-updateCtx.Done():
					return
				}
				_, _ = s.updater.CheckIfDue(updateCtx)
				s.notifyAvailableUpdate()
			}()
		}
	})
	return nil
}

// ServiceShutdown stops background work.
func (s *AppService) ServiceShutdown() error {
	if s.updateCancel != nil {
		s.updateCancel()
	}
	s.eng.Stop()
	return nil
}

func (s *AppService) appUpdateChanged(st appupdate.State) {
	if s.app != nil {
		s.app.Event.Emit("app-update", st)
	}
}

func (s *AppService) notifyAvailableUpdate() {
	if s.updater == nil || s.notifyAppUpdate == nil {
		return
	}
	if version := s.updater.TakeNotification(); version != "" {
		s.notifyAppUpdate(version)
	}
}

// pump coalesces change signals and pushes state to the panel and tray.
func (s *AppService) pump() {
	var lastTip string
	for range s.signal {
		time.Sleep(200 * time.Millisecond) // coalesce bursts
		st := s.eng.State()
		s.app.Event.Emit("state", st)

		tip := "MrW POE2 Filter"
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
	s.applyLanguage(saved, before)
	return saved, nil
}

// applyLanguage switches the Go side (and the tray menu, which is built once)
// when the settings that just landed carry a different language.
func (s *AppService) applyLanguage(cfg filter.Config, before i18n.Lang) {
	if lang := i18n.Resolve(cfg.Language); lang != before {
		i18n.Set(lang)
		if s.relabel != nil {
			s.relabel()
		}
	}
}

// Profiles lists the saved settings sets for the picker.
func (s *AppService) Profiles() []engine.ProfileInfo { return s.eng.Profiles() }

// SaveProfileAs stores the current settings under a name and makes it active.
func (s *AppService) SaveProfileAs(name string) ([]engine.ProfileInfo, error) {
	if err := s.eng.SaveProfileAs(name); err != nil {
		return s.eng.Profiles(), err
	}
	return s.eng.Profiles(), nil
}

// RenameProfile changes a saved profile's name without duplicating it.
func (s *AppService) RenameProfile(oldName, newName string) ([]engine.ProfileInfo, error) {
	if err := s.eng.RenameProfile(oldName, newName); err != nil {
		return s.eng.Profiles(), err
	}
	return s.eng.Profiles(), nil
}

// SwitchProfile loads another profile, applies it and rewrites the filter, so
// changing what you farm is one click rather than a dozen sliders.
func (s *AppService) SwitchProfile(name string) (filter.Config, error) {
	before := i18n.Current()
	cfg, err := s.eng.SwitchProfile(name)
	if err != nil {
		return s.eng.Config(), err
	}
	saved, err := s.eng.SetConfig(cfg)
	if err != nil {
		return saved, err
	}
	s.applyLanguage(saved, before)
	_ = s.eng.UpdateNow()
	return saved, nil
}

// DeleteProfile removes a profile and switches away from it when it was active.
func (s *AppService) DeleteProfile(name string) (filter.Config, error) {
	active, err := s.eng.DeleteProfile(name)
	if err != nil {
		return s.eng.Config(), err
	}
	return s.SwitchProfile(active)
}

// ExportProfile writes a profile to a file another player can import.
func (s *AppService) ExportProfile(name string) (string, error) {
	data, err := s.eng.ExportProfile(name)
	if err != nil {
		return "", err
	}
	return s.saveToFile(safeFileName("poe2filtre-profil-"+name)+".json", "JSON", "*.json", data)
}

// ImportProfile adds a profile from a file and switches to it. An empty name
// means the user cancelled the dialog.
func (s *AppService) ImportProfile() (string, error) {
	data, err := s.openFile("JSON", "*.json")
	if err != nil || data == nil {
		return "", err
	}
	name, err := s.eng.ImportProfile(data)
	if err != nil {
		return "", err
	}
	if _, err := s.SwitchProfile(name); err != nil {
		return name, err
	}
	return name, nil
}

// ExportFilter saves a copy of the filter that was last written, for sharing or
// for using it on a machine that does not run this app.
func (s *AppService) ExportFilter() (string, error) {
	st := s.eng.State()
	if st.Last == nil || st.Last.FilterPath == "" {
		return "", errors.New(i18n.T("err.noFilterYet"))
	}
	data, err := os.ReadFile(st.Last.FilterPath)
	if err != nil {
		return "", err
	}
	return s.saveToFile(filepath.Base(st.Last.FilterPath), "Filter", "*.filter", data)
}

// saveToFile asks where to put data and writes it; an empty path means the
// user cancelled.
func (s *AppService) saveToFile(name, filterName, pattern string, data []byte) (string, error) {
	dlg := s.app.Dialog.SaveFile().SetFilename(name).AddFilter(filterName, pattern)
	if s.panel != nil {
		dlg = dlg.AttachToWindow(s.panel)
	}
	path, err := dlg.PromptForSingleSelection()
	if err != nil || path == "" {
		return "", err
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", err
	}
	return path, nil
}

// pickFile asks for a file and returns its path; "" means cancelled.
func (s *AppService) pickFile(filterName, pattern string) (string, error) {
	dlg := s.app.Dialog.OpenFile().CanChooseFiles(true).AddFilter(filterName, pattern)
	if s.panel != nil {
		dlg = dlg.AttachToWindow(s.panel)
	}
	return dlg.PromptForSingleSelection()
}

// openFile asks for a file and returns its contents; nil means cancelled.
func (s *AppService) openFile(filterName, pattern string) ([]byte, error) {
	path, err := s.pickFile(filterName, pattern)
	if err != nil || path == "" {
		return nil, err
	}
	return os.ReadFile(path)
}

// safeFileName keeps a user-chosen profile name usable as a file name.
func safeFileName(s string) string {
	return strings.Map(func(r rune) rune {
		if strings.ContainsRune(`\/:*?"<>|`, r) {
			return '-'
		}
		return r
	}, s)
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
	return StyleOptions{
		Colours: filter.EffectColours,
		Shapes:  filter.IconShapes,
		Preset:  filter.NeverSinkPreset,
		Sounds:  filter.GameSounds,
	}
}

// UserGroupTemplate is the look and sound a new user group starts from, so the
// panel can render its colour picker like the built-in groups.
func (s *AppService) UserGroupTemplate() filter.StyleGroup { return filter.UserGroupTemplate() }

// StyleGroups lists the drop groups whose colours can be changed.
func (s *AppService) StyleGroups() []filter.StyleGroup { return filter.LocalizedStyleGroups() }

// ExportScan asks where to put the scan results and writes them there. It
// returns the chosen path, or an empty string when the user cancels.
func (s *AppService) ExportScan() (string, error) {
	data, err := s.eng.ExportScan()
	if err != nil {
		return "", err
	}
	return s.saveToFile("poe2filtre-tarama-"+time.Now().Format("2006-01-02")+".json", "JSON", "*.json", data)
}

// ImportScan asks for a file exported by another player and merges it. A nil
// result means the user cancelled the dialog.
func (s *AppService) ImportScan() (*trade.ImportResult, error) {
	data, err := s.openFile("JSON", "*.json")
	if err != nil || data == nil {
		return nil, err
	}
	res, err := s.eng.ImportScan(data)
	if err != nil {
		return nil, err
	}
	return &res, nil
}

// Leagues lists the leagues for the picker, live list first, always including
// the one currently configured.
func (s *AppService) Leagues() []string { return s.eng.Leagues() }

// UpdateNow starts a filter update in the background.
func (s *AppService) UpdateNow() error { return s.eng.UpdateNow() }

// GetAppUpdateState returns the cached application-update status without doing
// network work, so opening the settings panel stays instant.
func (s *AppService) GetAppUpdateState() appupdate.State {
	if s.updater == nil {
		return appupdate.State{Status: "disabled", CurrentVersion: s.meta.Version}
	}
	return s.updater.State()
}

// CheckForAppUpdate checks GitHub Releases now, regardless of the daily timer.
func (s *AppService) CheckForAppUpdate() (appupdate.State, error) {
	if s.updater == nil {
		return appupdate.State{}, errors.New("application updater is not available")
	}
	st, err := s.updater.Check(context.Background())
	if err == nil {
		s.notifyAvailableUpdate()
	}
	return st, err
}

// DownloadAppUpdate downloads and verifies the release executable.
func (s *AppService) DownloadAppUpdate() (appupdate.State, error) {
	if s.updater == nil {
		return appupdate.State{}, errors.New("application updater is not available")
	}
	return s.updater.Download(context.Background())
}

// InstallAppUpdate starts the staged updater and exits this process. The
// updater waits until Windows releases this executable before replacing it.
func (s *AppService) InstallAppUpdate() error {
	if s.updater == nil {
		return errors.New("application updater is not available")
	}
	if err := s.updater.LaunchInstaller(); err != nil {
		return err
	}
	go func() {
		time.Sleep(250 * time.Millisecond)
		s.app.Quit()
	}()
	return nil
}

// OpenAppUpdatePage opens the trusted GitHub release page in the default
// browser. Only URLs originating from the fixed GitHub API are accepted.
func (s *AppService) OpenAppUpdatePage() error {
	if s.updater == nil {
		return errors.New("application updater is not available")
	}
	url := s.updater.State().ReleaseURL
	if !strings.HasPrefix(url, "https://github.com/kadircelebi/poe2-filtre/releases/") {
		return errors.New("invalid application update page")
	}
	return exec.Command("rundll32.exe", "url.dll,FileProtocolHandler", url).Start()
}

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

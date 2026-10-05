package app

import (
	"context"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
	"github.com/wailsapp/wails/v3/pkg/services/notifications"

	"poe2filter/internal/appupdate"
	"poe2filter/internal/assets"
	"poe2filter/internal/engine"
	"poe2filter/internal/filter"
	"poe2filter/internal/i18n"
	"poe2filter/internal/overlay"
	"poe2filter/internal/platform"
	"poe2filter/internal/trade"
	"poe2filter/internal/useragent"
)

func init() {
	application.RegisterEvent[engine.State]("state")
	application.RegisterEvent[appupdate.State]("app-update")
	application.RegisterEvent[overlay.Snapshot]("overlay-item")
	application.RegisterEvent[trade.EvaluateRequest]("overlay-query")
	application.RegisterEvent[CraftImport]("craft-import")
}

func DefaultDataDir() string {
	if d, err := os.UserConfigDir(); err == nil {
		return filepath.Join(d, "PoE2Filtre")
	}
	return "."
}

// Options contains the executable's startup parameters. Assets and version
// are supplied by main so embedding and release linker flags stay at the root.
type Options struct {
	Version       string
	Frontend      fs.FS
	DataDir       string
	OutPath       string
	Headless      bool
	Show          bool
	ShowCraft     bool
	DebugPort     int
	ApplyUpdate   string
	WaitPID       int
	CleanupUpdate string
}

// Run starts the desktop application, or runs the requested headless/update mode.
func Run(opt Options) {
	if opt.ApplyUpdate != "" {
		if err := appupdate.Apply(opt.ApplyUpdate, opt.DataDir, opt.OutPath, opt.WaitPID); err != nil {
			fmt.Fprintln(os.Stderr, "update failed:", err)
			os.Exit(1)
		}
		return
	}
	if opt.CleanupUpdate != "" {
		if exe, err := os.Executable(); err == nil {
			go appupdate.CleanupAfterStart(opt.CleanupUpdate, exe)
		}
	}
	if !opt.Headless {
		keepCrashLog(opt.DataDir, opt.Version)
	}

	// The interface language must be known before any text is built: the tray
	// menu and the window title are created once, at start.
	i18n.Set(i18n.Resolve(filter.LoadConfig(filepath.Join(opt.DataDir, "config.json")).Language))

	if opt.Headless {
		eng := engine.New(engine.Options{Dir: opt.DataDir, OutPath: opt.OutPath})
		if err := eng.RunOnce(context.Background()); err != nil {
			fmt.Fprintln(os.Stderr, "HATA:", err)
			os.Exit(1)
		}
		return
	}

	useragent.Set("MrW-Overlay", opt.Version)
	svc := newAppService(Meta{
		Version:  opt.Version,
		DataDir:  opt.DataDir,
		GameDir:  filter.GetPoE2GameDir(),
		TestMode: opt.OutPath != "",
	})
	notifier := notifications.New()
	svc.updater = appupdate.New(appupdate.Options{
		CurrentVersion: opt.Version,
		DataDir:        opt.DataDir,
		Disabled:       opt.OutPath != "" || platform.StoreBuild,
		OnChange:       svc.appUpdateChanged,
	})
	aumid := platform.PackageAUMID()
	toasts := newToastQueue(func(opt notifications.NotificationOptions) {
		var err error
		if aumid != "" {
			err = platform.PushPackagedToast(aumid, opt.Title, opt.Body)
		} else {
			err = notifier.SendNotification(opt)
		}
		if err != nil {
			log.Printf("toast %s: %v", opt.ID, err)
		}
	})
	svc.notify = func(id, title, body string) {
		toasts.Push(notifications.NotificationOptions{ID: id, Title: title, Body: body})
	}
	svc.notifyAppUpdate = func(version string) {
		toasts.Push(notifications.NotificationOptions{
			ID: "application-update", Title: i18n.T("notify.appUpdateTitle"), Body: i18n.T("notify.appUpdateBody", version),
		})
	}

	var browserArgs []string
	if opt.DebugPort > 0 {
		browserArgs = append(browserArgs, fmt.Sprintf("--remote-debugging-port=%d", opt.DebugPort))
	}

	var tray *application.SystemTray
	app := application.New(application.Options{
		Windows: application.WindowsOptions{
			AdditionalBrowserArgs: browserArgs,
			WndProcInterceptor:    logQuitMessages,
		},
		Name:         "MrW Overlay for POE 2",
		Description:  i18n.T("app.description"),
		ErrorHandler: logAppError,
		ShouldQuit:   logQuitRequest,
		OnShutdown:   func() { log.Printf("shutdown: services stopping") },
		Services: []application.Service{
			application.NewService(svc),
			application.NewService(notifier),
		},
		Assets: application.AssetOptions{Handler: application.AssetFileServerFS(opt.Frontend), Middleware: sameOriginRuntime},
		SingleInstance: &application.SingleInstanceOptions{
			UniqueID: "com.kadir.poe2filter",
			OnSecondInstanceLaunch: func(application.SecondInstanceData) {
				if tray != nil {
					tray.ShowWindow()
				}
			},
		},
	})

	panel := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:             "panel",
		Title:            "MrW Overlay",
		Width:            800,
		Height:           640,
		Frameless:        true,
		AlwaysOnTop:      true,
		Hidden:           !opt.Show,
		DisableResize:    true,
		HideOnEscape:     true,
		HideOnFocusLost:  !opt.Show,
		BackgroundColour: svc.uiThemeBackground(),
		Windows:          application.WindowsWindow{HiddenOnTaskbar: true},
		URL:              "/",
	})
	// Closing the panel only hides it; the app keeps running in the tray.
	panel.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		panel.Hide()
		e.Cancel()
	})

	overlayWindow := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:             "overlay",
		Title:            "MrW Overlay",
		Width:            520,
		Height:           760,
		Frameless:        true,
		AlwaysOnTop:      true,
		Hidden:           true,
		DisableResize:    true,
		HideOnEscape:     true,
		HideOnFocusLost:  !opt.Show, // -show (testing) keeps it up for screenshots
		BackgroundColour: svc.uiThemeBackground(),
		Windows:          application.WindowsWindow{HiddenOnTaskbar: true},
		URL:              "/?view=overlay",
	})
	overlayWindow.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		overlayWindow.Hide()
		e.Cancel()
	})

	marketWindow := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:             "market",
		Title:            "MrW Overlay · Market",
		Width:            680,
		Height:           840,
		MinWidth:         560,
		MinHeight:        650,
		Frameless:        true,
		AlwaysOnTop:      true,
		Hidden:           true,
		HideOnEscape:     true,
		BackgroundColour: svc.uiThemeBackground(),
		Windows:          application.WindowsWindow{HiddenOnTaskbar: true},
		URL:              "/?view=market",
	})
	marketWindow.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		marketWindow.Hide()
		e.Cancel()
	})

	craftWindow := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name: "craft", Title: "MrW Overlay · Craft", Width: 1120, Height: 820,
		MinWidth: 760, MinHeight: 560, Frameless: true, AlwaysOnTop: true,
		Hidden: !opt.ShowCraft, HideOnEscape: true,
		BackgroundColour: svc.uiThemeBackground(),
		Windows:          application.WindowsWindow{HiddenOnTaskbar: true}, URL: "/?view=craft",
	})
	craftWindow.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		craftWindow.Hide()
		e.Cancel()
	})

	// Settings live in a window of their own: an ordinary one that stays open
	// beside the game, so a colour can be changed and tried with Reload
	// without the panel vanishing on every click in between.
	settingsWindow := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:             "settings",
		Title:            "MrW Overlay",
		Width:            980,
		Height:           700,
		MinWidth:         760,
		MinHeight:        520,
		Frameless:        true,
		Hidden:           true,
		BackgroundColour: svc.uiThemeBackground(),
		URL:              "/?view=settings",
	})
	settingsWindow.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		_ = svc.SetHotkeyCapture(false)
		settingsWindow.Hide()
		e.Cancel()
	})

	trayMenu := func() *application.Menu {
		m := app.NewMenu()
		m.Add(i18n.T("tray.open")).OnClick(func(*application.Context) { tray.ShowWindow() })
		m.Add(i18n.T("tray.settings")).OnClick(func(*application.Context) { svc.ShowSettings("") })
		m.Add(i18n.T("tray.update")).OnClick(func(*application.Context) { _ = svc.UpdateNow() })
		if !platform.StoreBuild {
			m.Add(i18n.T("tray.appUpdate")).OnClick(func(*application.Context) {
				tray.ShowWindow()
				go func() { _, _ = svc.CheckForAppUpdate() }()
			})
		}
		m.AddSeparator()
		m.Add(i18n.T("tray.openFolder")).OnClick(func(*application.Context) { _ = svc.OpenGameFolder() })
		m.AddSeparator()
		m.Add(i18n.T("tray.quit")).OnClick(func(*application.Context) { app.Quit() })
		return m
	}

	tray = app.SystemTray.New()
	tray.SetIcon(assets.Tray)
	tray.SetTooltip("MrW Overlay for POE 2")
	tray.SetMenu(trayMenu())
	svc.relabel = func() { tray.SetMenu(trayMenu()) }
	tray.AttachWindow(panel).WindowOffset(8)

	svc.app, svc.tray, svc.panel = app, tray, panel
	svc.overlayWindow, svc.marketWindow, svc.settingsWindow = overlayWindow, marketWindow, settingsWindow
	svc.craftWindow = craftWindow
	// Price check (Alt+E), market (Alt+M) and craft (Alt+F) shortcuts
	// exist only while the overlay is switched on.
	shortcuts := func(s overlay.Settings) [][2]any {
		if !s.Enabled {
			return nil
		}
		list := [][2]any{{s.Hotkey, svc.captureOverlay}, {s.MarketHotkey, svc.toggleMarketFromHotkey}, {s.CraftHotkey, svc.toggleCraftFromHotkey}}
		if s.HideHotkey != "" {
			list = append(list, [2]any{s.HideHotkey, svc.captureHide})
		}
		return list
	}
	register := func(list [][2]any) error {
		var done []string
		for _, sc := range list {
			key := sc[0].(string)
			if err := app.GlobalShortcut.Register(key, sc[1].(func())); err != nil {
				for _, k := range done {
					_ = app.GlobalShortcut.Unregister(k)
				}
				return fmt.Errorf("%s: %w", key, err)
			}
			done = append(done, key)
		}
		return nil
	}
	svc.rebindOverlay = func(old, next overlay.Settings) error {
		if err := next.DistinctHotkeys(); next.Enabled && err != nil {
			return err
		}
		for _, sc := range shortcuts(old) {
			if key := sc[0].(string); app.GlobalShortcut.IsRegistered(key) {
				_ = app.GlobalShortcut.Unregister(key)
			}
		}
		svc.overlayHotkey = ""
		if err := register(shortcuts(next)); err != nil {
			if register(shortcuts(old)) == nil && old.Enabled {
				svc.overlayHotkey = old.Hotkey
			}
			return err
		}
		if next.Enabled {
			svc.overlayHotkey = next.Hotkey
		}
		return nil
	}
	if initial := svc.GetOverlaySettings(); initial.Enabled {
		if err := svc.rebindOverlay(overlay.Settings{}, initial); err != nil {
			// A taken market shortcut must not cost the price check.
			log.Printf("overlay shortcuts: %v", err)
			if app.GlobalShortcut.Register(initial.Hotkey, svc.captureOverlay) == nil {
				svc.overlayHotkey = initial.Hotkey
			}
		}
	}
	go svc.watchGameFocus()
	svc.eng = engine.New(engine.Options{
		Dir:      opt.DataDir,
		OutPath:  opt.OutPath,
		OnChange: svc.changed,
		Notify: func(title, body string) {
			toasts.Push(notifications.NotificationOptions{
				ID: "filter-updated", Title: title, Body: body,
			})
		},
	})

	err := app.Run()
	log.Printf("app.Run returned: %v", err)
	if err != nil {
		log.Fatal(err)
	}
}

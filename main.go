package main

import (
	"context"
	"embed"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
	"github.com/wailsapp/wails/v3/pkg/services/notifications"

	"poe2filter/internal/assets"
	"poe2filter/internal/engine"
	"poe2filter/internal/filter"
	"poe2filter/internal/i18n"
)

// version is set at build time with -ldflags "-X main.version=...".
var version = "0.7.0"

//go:embed all:frontend/dist
var frontend embed.FS

func init() {
	application.RegisterEvent[engine.State]("state")
}

func defaultDataDir() string {
	if d, err := os.UserConfigDir(); err == nil {
		return filepath.Join(d, "PoE2Filtre")
	}
	return "."
}

func main() {
	dataDir := flag.String("data", defaultDataDir(), "settings and data folder")
	outPath := flag.String("out", "", "write the filter here instead of the game folder (testing)")
	headless := flag.Bool("headless", false, "update once without a window and exit")
	show := flag.Bool("show", false, "show the panel on start")
	debugPort := flag.Int("debug-port", 0, "WebView2 remote debugging port (development)")
	flag.Parse()

	// The interface language must be known before any text is built: the tray
	// menu and the window title are created once, at start.
	i18n.Set(i18n.Resolve(filter.LoadConfig(filepath.Join(*dataDir, "config.json")).Language))

	if *headless {
		eng := engine.New(engine.Options{Dir: *dataDir, OutPath: *outPath})
		if err := eng.RunOnce(context.Background()); err != nil {
			fmt.Fprintln(os.Stderr, "HATA:", err)
			os.Exit(1)
		}
		return
	}

	svc := newAppService(Meta{
		Version:  version,
		DataDir:  *dataDir,
		GameDir:  filter.GetPoE2GameDir(),
		TestMode: *outPath != "",
	})
	notifier := notifications.New()

	var browserArgs []string
	if *debugPort > 0 {
		browserArgs = append(browserArgs, fmt.Sprintf("--remote-debugging-port=%d", *debugPort))
	}

	var tray *application.SystemTray
	app := application.New(application.Options{
		Windows:     application.WindowsOptions{AdditionalBrowserArgs: browserArgs},
		Name:        "PoE2 Filtre",
		Description: i18n.T("app.description"),
		Services: []application.Service{
			application.NewService(svc),
			application.NewService(notifier),
		},
		Assets: application.AssetOptions{Handler: application.AssetFileServerFS(frontend)},
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
		Title:            "PoE2 Filtre",
		Width:            380,
		Height:           640,
		Frameless:        true,
		AlwaysOnTop:      true,
		Hidden:           !*show,
		DisableResize:    true,
		HideOnEscape:     true,
		HideOnFocusLost:  !*show,
		BackgroundColour: application.NewRGB(15, 13, 17),
		Windows:          application.WindowsWindow{HiddenOnTaskbar: true},
		URL:              "/",
	})
	// Closing the panel only hides it; the app keeps running in the tray.
	panel.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		panel.Hide()
		e.Cancel()
	})

	trayMenu := func() *application.Menu {
		m := app.NewMenu()
		m.Add(i18n.T("tray.open")).OnClick(func(*application.Context) { tray.ShowWindow() })
		m.Add(i18n.T("tray.update")).OnClick(func(*application.Context) { _ = svc.UpdateNow() })
		m.AddSeparator()
		m.Add(i18n.T("tray.openFolder")).OnClick(func(*application.Context) { _ = svc.OpenGameFolder() })
		m.AddSeparator()
		m.Add(i18n.T("tray.quit")).OnClick(func(*application.Context) { app.Quit() })
		return m
	}

	tray = app.SystemTray.New()
	tray.SetIcon(assets.Tray)
	tray.SetTooltip("PoE2 Filtre")
	tray.SetMenu(trayMenu())
	svc.relabel = func() { tray.SetMenu(trayMenu()) }
	tray.AttachWindow(panel).WindowOffset(8)

	svc.app, svc.tray = app, tray
	svc.eng = engine.New(engine.Options{
		Dir:      *dataDir,
		OutPath:  *outPath,
		OnChange: svc.changed,
		Notify: func(title, body string) {
			_ = notifier.SendNotification(notifications.NotificationOptions{
				ID: "filter-updated", Title: title, Body: body,
			})
		},
	})

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}

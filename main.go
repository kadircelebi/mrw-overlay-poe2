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
)

// version is set at build time with -ldflags "-X main.version=...".
var version = "0.4.0"

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
	dataDir := flag.String("data", defaultDataDir(), "Ayar ve veri klasörü")
	outPath := flag.String("out", "", "Filtreyi oyun klasörü yerine bu dosyaya yaz (test)")
	headless := flag.Bool("headless", false, "Arayüz açmadan bir kez güncelle ve çık")
	show := flag.Bool("show", false, "Açılışta paneli göster")
	debugPort := flag.Int("debug-port", 0, "WebView2 uzaktan hata ayıklama portu (geliştirme)")
	flag.Parse()

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
		Description: "Canlı piyasa fiyatlarıyla güncellenen PoE2 loot filtresi",
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

	menu := app.NewMenu()
	menu.Add("Paneli aç").OnClick(func(*application.Context) { tray.ShowWindow() })
	menu.Add("Şimdi güncelle").OnClick(func(*application.Context) { _ = svc.UpdateNow() })
	menu.AddSeparator()
	menu.Add("Filtre klasörünü aç").OnClick(func(*application.Context) { _ = svc.OpenGameFolder() })
	menu.AddSeparator()
	menu.Add("Çıkış").OnClick(func(*application.Context) { app.Quit() })

	tray = app.SystemTray.New()
	tray.SetIcon(assets.Tray)
	tray.SetTooltip("PoE2 Filtre")
	tray.SetMenu(menu)
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

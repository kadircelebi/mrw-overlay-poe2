package main

import (
	"embed"
	"flag"

	"poe2filter/internal/app"
)

// version is set at build time with -ldflags "-X main.version=...".
var version = "2.13.0"

//go:embed all:frontend/dist
var frontend embed.FS

func main() {
	dataDir := flag.String("data", app.DefaultDataDir(), "settings and data folder")
	outPath := flag.String("out", "", "write the filter here instead of the game folder (testing)")
	headless := flag.Bool("headless", false, "update once without a window and exit")
	show := flag.Bool("show", false, "show the panel on start")
	showCraft := flag.Bool("craft", false, "show theoretical craft on start")
	debugPort := flag.Int("debug-port", 0, "WebView2 remote debugging port (development)")
	applyUpdate := flag.String("apply-update", "", "replace this executable after it exits (internal)")
	waitPID := flag.Int("wait-pid", 0, "wait for this process before applying an update (internal)")
	cleanupUpdate := flag.String("cleanup-update", "", "remove staged updater after a successful start (internal)")
	flag.Parse()
	app.Run(app.Options{
		Version: version, Frontend: frontend,
		DataDir: *dataDir, OutPath: *outPath,
		Headless: *headless, Show: *show, ShowCraft: *showCraft,
		DebugPort: *debugPort, ApplyUpdate: *applyUpdate,
		WaitPID: *waitPID, CleanupUpdate: *cleanupUpdate,
	})
}

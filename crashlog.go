package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime/debug"
	"time"
)

// keepCrashLog sends everything that would otherwise vanish into
// <data>\crash.log. The app has no console, so without this:
//
//   - Go's crash report (an unrecovered panic, a fatal error such as a
//     concurrent map write, an access violation in Windows code) is lost,
//     and Windows does not record Go crashes either;
//   - Wails and its WebView2 bridge print an error and then call os.Exit(1)
//     in several places, which is not a crash and leaves nothing behind.
//
// Standard output, standard error and the log package all go to the file.
func keepCrashLog(dataDir string) {
	path := filepath.Join(dataDir, "crash.log")
	flag := os.O_APPEND | os.O_CREATE | os.O_WRONLY
	if fi, err := os.Stat(path); err == nil && fi.Size() > 1<<20 {
		flag = os.O_TRUNC | os.O_CREATE | os.O_WRONLY
	}
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return
	}
	f, err := os.OpenFile(path, flag, 0o644)
	if err != nil {
		return
	}
	fmt.Fprintf(f, "\r\n=== start %s, version %s, pid %d\r\n", time.Now().Format("2006-01-02 15:04:05"), version, os.Getpid())
	_ = debug.SetCrashOutput(f, debug.CrashOptions{})
	os.Stdout = f
	os.Stderr = f
	log.SetOutput(f)
	log.SetFlags(log.LstdFlags)
}

// logQuitRequest is Wails' ShouldQuit: every path that ends the app through
// App.Quit passes here. The stack says who asked.
func logQuitRequest() bool {
	log.Printf("quit requested by:\n%s", debug.Stack())
	return true
}

// logQuitMessages records the window messages that make Wails quit (sent to
// its hidden main-thread window) or that Windows sends at logoff/shutdown.
func logQuitMessages(hwnd uintptr, msg uint32, wParam, lParam uintptr) (uintptr, bool) {
	switch msg {
	case 0x0010, 0x0002, 0x0011, 0x0016: // WM_CLOSE, WM_DESTROY, WM_QUERYENDSESSION, WM_ENDSESSION
		log.Printf("window message 0x%04x to hwnd 0x%x (wParam %d)", msg, hwnd, wParam)
	}
	return 0, false
}

// logAppError is Wails' ErrorHandler: it is called before Wails exits on a
// fatal error, and for every WebView2 error.
func logAppError(err error) {
	log.Printf("wails error: %v", err)
}

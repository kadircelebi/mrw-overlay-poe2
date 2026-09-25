package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows/registry"
)

// ChromiumBrowser is an installed Chromium-based browser the extension can
// be loaded into (unpacked, from its extensions page in developer mode).
type ChromiumBrowser struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// Page is the browser's own extensions page ("edge://extensions").
	Page    string `json:"page"`
	Default bool   `json:"default"`
	exe     string
}

var chromiumCandidates = []struct {
	id, name, exe, page, progID string
}{
	{"edge", "Microsoft Edge", "msedge.exe", "edge://extensions", "MSEdgeHTM"},
	{"chrome", "Google Chrome", "chrome.exe", "chrome://extensions", "ChromeHTML"},
	{"brave", "Brave", "brave.exe", "brave://extensions", "BraveHTML"},
	{"opera", "Opera", "opera.exe", "opera://extensions", "OperaStable"},
	{"vivaldi", "Vivaldi", "vivaldi.exe", "vivaldi://extensions", "VivaldiHTM"},
}

// ChromiumBrowsers lists the Chromium browsers found through Windows' "App
// Paths" registration, the default browser marked.
func (s *AppService) ChromiumBrowsers() []ChromiumBrowser {
	defaultProgID := ""
	if k, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\Shell\Associations\UrlAssociations\https\UserChoice`, registry.QUERY_VALUE); err == nil {
		defaultProgID, _, _ = k.GetStringValue("ProgId")
		k.Close()
	}
	out := []ChromiumBrowser{}
	for _, c := range chromiumCandidates {
		exe := appPath(c.exe)
		if exe == "" {
			continue
		}
		out = append(out, ChromiumBrowser{ID: c.id, Name: c.name, Page: c.page, Default: strings.HasPrefix(defaultProgID, c.progID), exe: exe})
	}
	return out
}

// OpenExtensionsPage opens a browser on its extensions page. A browser only
// opens its internal pages when started with one, not through a link.
func (s *AppService) OpenExtensionsPage(id string) error {
	for _, b := range s.ChromiumBrowsers() {
		if b.ID == id {
			return exec.Command(b.exe, b.Page).Start()
		}
	}
	return errors.New("browser not found")
}

// OpenLinkIn opens the pending connection link in a chosen browser, started
// directly with the link: the extension may live in a browser other than
// Windows' default, and opening through the default handler failed silently.
func (s *AppService) OpenLinkIn(id string) error {
	link := s.BrowserLinkState().URL
	if link == "" {
		return errors.New("no connection is waiting; press Connect again")
	}
	for _, b := range s.ChromiumBrowsers() {
		if b.ID == id {
			if err := exec.Command(b.exe, link).Start(); err != nil {
				return err
			}
			s.armExtensionWait()
			return nil
		}
	}
	return errors.New("browser not found")
}

// OpenBrowserExtensionFolder writes the extension out and shows its folder,
// which is what "Load unpacked" asks for.
func (s *AppService) OpenBrowserExtensionFolder() (string, error) {
	dir, err := s.PrepareBrowserExtension()
	if err != nil {
		return "", err
	}
	return dir, exec.Command("explorer.exe", dir).Start()
}

func appPath(exe string) string {
	for _, root := range []registry.Key{registry.CURRENT_USER, registry.LOCAL_MACHINE} {
		k, err := registry.OpenKey(root, `Software\Microsoft\Windows\CurrentVersion\App Paths\`+exe, registry.QUERY_VALUE)
		if err != nil {
			continue
		}
		path, _, err := k.GetStringValue("")
		k.Close()
		path = strings.Trim(path, `"`)
		if err == nil && path != "" && filepath.IsAbs(path) {
			if _, statErr := os.Stat(path); statErr == nil {
				return path
			}
		}
	}
	return ""
}

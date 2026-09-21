package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"poe2filter/internal/collector"
	"poe2filter/internal/gamesounds"
	"poe2filter/internal/i18n"
)

var soundExts = map[string]bool{".mp3": true, ".wav": true, ".ogg": true}

// ListSounds returns the sound files in the PoE2 filter folder, which is where
// the game looks for CustomAlertSound files.
func (s *AppService) ListSounds() []string {
	entries, err := os.ReadDir(s.meta.GameDir)
	if err != nil {
		return []string{}
	}
	out := []string{}
	for _, e := range entries {
		if !e.IsDir() && soundExts[strings.ToLower(filepath.Ext(e.Name()))] {
			out = append(out, e.Name())
		}
	}
	sort.Strings(out)
	return out
}

// PreviewSound plays a sound file from the filter folder.
func (s *AppService) PreviewSound(name string) error {
	base := filepath.Base(name)
	if base != name || !soundExts[strings.ToLower(filepath.Ext(base))] {
		return errors.New(i18n.T("err.soundInvalid"))
	}
	path := filepath.Join(s.meta.GameDir, base)
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf(i18n.T("err.soundNotFound"), base)
	}
	return playSound(path)
}

// GameSoundsReady reports whether the game's own alert sounds have been
// downloaded, so the panel knows to offer "play" or "download".
func (s *AppService) GameSoundsReady() bool {
	return gamesounds.Ready(s.meta.DataDir)
}

// DownloadGameSounds fetches the alert sounds the game plays for 1-6 so they
// can be listened to in the app. It is only ever called from the button in the
// panel; nothing downloads them on its own.
func (s *AppService) DownloadGameSounds() (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	n, err := gamesounds.Download(ctx, collector.NewHTTPClient(), s.meta.DataDir, collector.UserAgent)
	if err != nil {
		return n, fmt.Errorf(i18n.T("err.soundDownload"), err)
	}
	return n, nil
}

// PreviewGameSound plays one of the downloaded alert sounds (1-6).
func (s *AppService) PreviewGameSound(n int) error {
	if n < 1 || n > gamesounds.Count {
		return errors.New(i18n.T("err.soundInvalid"))
	}
	path := gamesounds.Path(s.meta.DataDir, n)
	if _, err := os.Stat(path); err != nil {
		return errors.New(i18n.T("err.gameSoundsMissing"))
	}
	return playSound(path)
}

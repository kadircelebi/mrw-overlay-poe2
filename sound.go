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

// AddSound copies a sound the user picks into the filter folder, which is the
// only place the game reads CustomAlertSound files from. It returns the name
// the file ended up with, or "" if the dialog was cancelled.
func (s *AppService) AddSound() (string, error) {
	path, err := s.pickFile("Ses", "*.mp3;*.wav")
	if err != nil || path == "" {
		return "", err
	}
	return copySoundInto(path, s.meta.GameDir)
}

// copySoundInto puts one sound file in the filter folder and returns the name
// the game will know it by.
func copySoundInto(path, gameDir string) (string, error) {
	name := filepath.Base(path)
	if !soundExts[strings.ToLower(filepath.Ext(name))] {
		return "", errors.New(i18n.T("err.soundInvalid"))
	}
	if gameDir == "" {
		return "", errors.New(i18n.T("err.gameDir"))
	}
	dest := filepath.Join(gameDir, name)
	// Picking a file that already lives there is not an error, but copying it
	// onto itself would truncate it.
	if same, _ := sameFile(path, dest); same {
		return name, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(dest, data, 0o644); err != nil {
		return "", fmt.Errorf(i18n.T("err.soundCopy"), err)
	}
	return name, nil
}

// sameFile reports whether two paths point at the same file on disk.
func sameFile(a, b string) (bool, error) {
	ai, err := os.Stat(a)
	if err != nil {
		return false, err
	}
	bi, err := os.Stat(b)
	if err != nil {
		return false, err
	}
	return os.SameFile(ai, bi), nil
}

// GameSoundsReady reports whether the game's own alert sounds have been
// downloaded, so the panel knows to offer "play" or "download".
func (s *AppService) GameSoundsReady() bool {
	return gamesounds.Ready(s.meta.DataDir)
}

// DownloadGameSounds fetches the alert sounds the game plays so they can be
// listened to in the app. It is only ever called from the button in the
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

// PreviewGameSound plays one of the downloaded alert sounds.
func (s *AppService) PreviewGameSound(id string) error {
	if !gamesounds.Known(id) {
		return errors.New(i18n.T("err.soundInvalid"))
	}
	path := gamesounds.Path(s.meta.DataDir, id)
	if _, err := os.Stat(path); err != nil {
		return errors.New(i18n.T("err.gameSoundsMissing"))
	}
	return playSound(path)
}

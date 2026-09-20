package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

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

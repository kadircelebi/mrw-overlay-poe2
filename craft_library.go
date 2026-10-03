package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
)

// The craft library: crafts the player saved (or that were saved for them
// when a new craft started), kept as one JSON file in the app's data folder
// so the GitHub and Microsoft Store builds share it and nothing is lost when
// the window's own storage is cleared. The craft page owns the format; the
// app only checks it is a JSON array of a sane size.

const craftLibraryFile = "craft_library.json"

// maxCraftLibrary bounds the file: a long craft with its history is a few
// hundred kilobytes, and the page keeps at most a hundred entries.
const maxCraftLibrary = 16 << 20

var craftLibraryMu sync.Mutex

func (s *AppService) craftLibraryPath() string {
	return filepath.Join(s.meta.DataDir, craftLibraryFile)
}

// CraftLibrary returns the saved crafts as JSON text ("[]" when there are none).
func (s *AppService) CraftLibrary() (string, error) {
	craftLibraryMu.Lock()
	defer craftLibraryMu.Unlock()
	data, err := os.ReadFile(s.craftLibraryPath())
	if errors.Is(err, os.ErrNotExist) {
		return "[]", nil
	}
	if err != nil {
		return "", err
	}
	var list []json.RawMessage
	if json.Unmarshal(data, &list) != nil {
		// A damaged file is kept aside rather than overwritten by the next save.
		_ = os.Rename(s.craftLibraryPath(), s.craftLibraryPath()+".damaged")
		return "[]", nil
	}
	return string(data), nil
}

// SaveCraftLibrary replaces the saved crafts with data, a JSON array.
func (s *AppService) SaveCraftLibrary(data string) error {
	if len(data) > maxCraftLibrary {
		return errors.New("craft library too large")
	}
	var list []json.RawMessage
	if err := json.Unmarshal([]byte(data), &list); err != nil {
		return errors.New("craft library is not a JSON array")
	}
	craftLibraryMu.Lock()
	defer craftLibraryMu.Unlock()
	path := s.craftLibraryPath()
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(data), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

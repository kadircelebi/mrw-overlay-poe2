// Package gamesounds makes the game's own alert sounds audible inside the app.
//
// A filter can only ask the game to play a sound by id, and the audio itself
// lives inside Path of Exile 2's FMOD banks, which nothing outside the game can
// read. So a user picking "game sound 3" had no way of knowing what they chose.
// The 26 sounds ship with the app and are written out on first use, because the
// player back end takes a file path.
//
// The audio belongs to Grinding Gear Games and is not covered by this project's
// licence; see NOTICE. It is only ever played as a preview — the filter still
// writes PlayAlertSound, and in game the game plays its own audio.
package gamesounds

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"

	"poe2filter/internal/prices"
)

// files holds the sounds. TestEveryIDHasAFile keeps this in step with IDs.
//
//go:embed files/*.mp3
var files embed.FS

// IDs are the alert sounds a filter can ask for, in the order the community
// numbers them: 1 to 16, then the ten named currency sounds that tools show as
// 17 to 26. They mirror filter.GameSounds.
var IDs = []string{
	"1", "2", "3", "4", "5", "6", "7", "8",
	"9", "10", "11", "12", "13", "14", "15", "16",
	"ShAlchemy", "ShBlessed", "ShChaos", "ShFusing", "ShGeneral",
	"ShRegal", "ShVaal", "ShDivine", "ShExalted", "ShMirror",
}

// Count is how many alert sounds there are.
var Count = len(IDs)

// Dir is where the sounds are written for playback. It is deliberately not the
// filter folder: these are for listening in the app, not for the game, and they
// must not show up among the user's own custom sounds.
func Dir(dataDir string) string { return filepath.Join(dataDir, "sounds") }

// Path is the file for one alert sound id.
func Path(dataDir, id string) string {
	return filepath.Join(Dir(dataDir), name(id))
}

func name(id string) string { return "AlertSound" + id + ".mp3" }

// Known reports whether id is one of the game's alert sounds.
func Known(id string) bool {
	for _, x := range IDs {
		if x == id {
			return true
		}
	}
	return false
}

// Ensure writes the sound out if it is not on disk yet and returns its path.
// A file left behind by an older version, which downloaded them instead, is
// reused as it is.
func Ensure(dataDir, id string) (string, error) {
	if !Known(id) {
		return "", fmt.Errorf("unknown alert sound %q", id)
	}
	path := Path(dataDir, id)
	if info, err := os.Stat(path); err == nil && info.Size() > 0 {
		return path, nil
	}
	data, err := files.ReadFile("files/" + name(id))
	if err != nil {
		return "", err
	}
	if err := prices.WriteFileAtomic(path, data); err != nil {
		return "", err
	}
	return path, nil
}

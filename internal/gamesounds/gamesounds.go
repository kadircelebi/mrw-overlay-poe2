// Package gamesounds makes the game's own alert sounds audible inside the app.
//
// A filter can only ask the game to play a sound by id; the files live inside
// Path of Exile 2's FMOD banks, which nothing outside the game can read. So a
// user picking "game sound 3" had no way of knowing what they chose. FilterBlade
// publishes the same six sounds as plain mp3 for its own preview, and the app
// fetches them once, on request, into the data folder. They are only ever played
// back as a preview: the filter still writes PlayAlertSound, and the game plays
// its own audio.
//
// Nothing is downloaded unless the user asks for it, and nothing is shipped with
// the app: the audio belongs to Grinding Gear Games.
package gamesounds

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"poe2filter/internal/prices"
)

// IDs are the alert sounds a filter can ask for, in the order the community
// numbers them: 1 to 16, then the ten named currency sounds that tools show as
// 17 to 26. They mirror filter.GameSounds; the file names FilterBlade publishes
// use the same ids.
var IDs = []string{
	"1", "2", "3", "4", "5", "6", "7", "8",
	"9", "10", "11", "12", "13", "14", "15", "16",
	"ShAlchemy", "ShBlessed", "ShChaos", "ShFusing", "ShGeneral",
	"ShRegal", "ShVaal", "ShDivine", "ShExalted", "ShMirror",
}

// Count is how many sounds there are to download.
var Count = len(IDs)

// sourceURL is a variable only so the tests can point it at a local server.
var sourceURL = "https://www.filterblade.xyz/assets/sounds/AlertSound%s.mp3"

// Guard rails for what we accept as a sound file. The real ones are 29-59 KB.
const (
	minSize = 4 << 10
	maxSize = 4 << 20
)

// Dir is where the downloaded previews are kept. It is deliberately not the
// filter folder: these files are for listening in the app, not for the game,
// and they must not show up in the custom sound list.
func Dir(dataDir string) string { return filepath.Join(dataDir, "sounds") }

// Path is the file for one alert sound id.
func Path(dataDir, id string) string {
	return filepath.Join(Dir(dataDir), "AlertSound"+id+".mp3")
}

// Known reports whether id is one of the game's alert sounds.
func Known(id string) bool {
	for _, x := range IDs {
		if x == id {
			return true
		}
	}
	return false
}

// Ready reports whether every sound has been downloaded.
func Ready(dataDir string) bool { return len(Missing(dataDir)) == 0 }

// Missing lists the sounds that still have to be fetched.
func Missing(dataDir string) []string {
	var out []string
	for _, id := range IDs {
		if info, err := os.Stat(Path(dataDir, id)); err != nil || info.Size() < minSize {
			out = append(out, id)
		}
	}
	return out
}

// Download fetches the sounds that are missing and returns how many arrived.
// One failure stops the run: a half-finished set is reported as an error so the
// user can retry, while the files that did arrive stay on disk.
func Download(ctx context.Context, c *http.Client, dataDir, userAgent string) (int, error) {
	got := 0
	for _, id := range Missing(dataDir) {
		data, err := fetch(ctx, c, fmt.Sprintf(sourceURL, id), userAgent)
		if err != nil {
			return got, err
		}
		if err := prices.WriteFileAtomic(Path(dataDir, id), data); err != nil {
			return got, err
		}
		got++
	}
	return got, nil
}

func fetch(ctx context.Context, c *http.Client, url, userAgent string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, url)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxSize+1))
	if err != nil {
		return nil, err
	}
	if err := checkMP3(data); err != nil {
		return nil, fmt.Errorf("%s: %w", url, err)
	}
	return data, nil
}

// checkMP3 keeps an error page or a redirect to a login screen from being
// written out as if it were audio.
func checkMP3(data []byte) error {
	if len(data) < minSize || len(data) > maxSize {
		return fmt.Errorf("unexpected size (%d bytes)", len(data))
	}
	if string(data[:3]) == "ID3" {
		return nil
	}
	// A frame sync: eleven set bits, so 0xFF followed by 0xE0 or more.
	if data[0] == 0xFF && data[1]&0xE0 == 0xE0 {
		return nil
	}
	return fmt.Errorf("this is not an mp3 file")
}

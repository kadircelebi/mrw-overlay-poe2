package overlay

import (
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"
)

// The game logs each area it builds:
//
//	2026/10/03 22:40:26 17384312 2caa229f [DEBUG Client 32476] Generating level 79 area "MapWaywardIsle" with seed 817132385
var areaRE = regexp.MustCompile(`Generating level (\d+) area "([^"]+)"(?: with seed (\d+))?`)

// Area is one area the game built: its level, id ("MapWaywardIsle") and
// seed. A map is one instance: the same id with another seed is another map.
type Area struct {
	Level int    `json:"level"`
	ID    string `json:"id"`
	Seed  string `json:"seed,omitempty"`
}

// Rest tells hideouts and towns, where nothing drops.
func (a Area) Rest() bool { return isRestArea(a.ID) }

// Same tells whether a and b are the same instance.
func (a Area) Same(b Area) bool { return a.ID == b.ID && a.Seed == b.Seed }

// ParseArea reads an area line of Client.txt.
func ParseArea(line string) (Area, bool) {
	m := areaRE.FindStringSubmatch(line)
	if m == nil {
		return Area{}, false
	}
	n, err := strconv.Atoi(m[1])
	if err != nil {
		return Area{}, false
	}
	return Area{Level: n, ID: m[2], Seed: m[3]}, true
}

// areaWindow is how far back LastArea looks; a session writes an area line
// at every zone change, and the log is hundreds of MB.
const areaWindow = 1 << 20

// LastArea returns the level and id of the last area the player went to in
// the current game session, from the end of Client.txt. Hideouts and towns
// are skipped: an item in the inventory came from the area before them.
// Only these two values are taken from the log.
func LastArea(path string) (int, string) {
	a := LastMap(path)
	return a.Level, a.ID
}

// LastMap is LastArea with the area's seed.
func LastMap(path string) Area {
	return lastArea(logTail(path), false)
}

// CurrentArea is the area the player is in now, hideouts and towns
// included: the last one the current game session built (ok false when
// the log names none).
func CurrentArea(path string) (Area, bool) {
	a := lastArea(logTail(path), true)
	return a, a.ID != ""
}

// logTail is the end of the game's log (areaWindow bytes).
func logTail(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return ""
	}
	start := max(0, info.Size()-areaWindow)
	buf := make([]byte, info.Size()-start)
	if _, err := f.ReadAt(buf, start); err != nil && err != io.EOF {
		return ""
	}
	return string(buf)
}

// lastArea is the last area the log's current session built; rest also
// takes hideouts and towns.
func lastArea(log string, rest bool) Area {
	var last Area
	for _, line := range strings.Split(log, "\n") {
		if strings.Contains(line, sessionStart) {
			last = Area{}
			continue
		}
		if a, ok := ParseArea(line); ok && (rest || !a.Rest()) {
			last = a
		}
	}
	return last
}

// isRestArea tells hideouts and towns, where nothing drops.
func isRestArea(id string) bool {
	l := strings.ToLower(id)
	return strings.Contains(l, "hideout") || strings.Contains(l, "town")
}

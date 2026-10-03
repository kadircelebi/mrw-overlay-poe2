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
var areaRE = regexp.MustCompile(`Generating level (\d+) area "([^"]+)"`)

// areaWindow is how far back LastArea looks; a session writes an area line
// at every zone change, and the log is hundreds of MB.
const areaWindow = 1 << 20

// LastArea returns the level and id of the last area the player went to in
// the current game session, from the end of Client.txt. Hideouts and towns
// are skipped: an item in the inventory came from the area before them.
// Only these two values are taken from the log.
func LastArea(path string) (int, string) {
	f, err := os.Open(path)
	if err != nil {
		return 0, ""
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return 0, ""
	}
	start := max(0, info.Size()-areaWindow)
	buf := make([]byte, info.Size()-start)
	if _, err := f.ReadAt(buf, start); err != nil && err != io.EOF {
		return 0, ""
	}
	return lastArea(string(buf))
}

func lastArea(log string) (int, string) {
	level, area := 0, ""
	for _, line := range strings.Split(log, "\n") {
		if strings.Contains(line, sessionStart) {
			level, area = 0, ""
			continue
		}
		m := areaRE.FindStringSubmatch(line)
		if m == nil || isRestArea(m[2]) {
			continue
		}
		if n, err := strconv.Atoi(m[1]); err == nil {
			level, area = n, m[2]
		}
	}
	return level, area
}

// isRestArea tells hideouts and towns, where nothing drops.
func isRestArea(id string) bool {
	l := strings.ToLower(id)
	return strings.Contains(l, "hideout") || strings.Contains(l, "town")
}

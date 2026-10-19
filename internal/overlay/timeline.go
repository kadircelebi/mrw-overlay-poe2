package overlay

import (
	"bufio"
	"io"
	"os"
	"strings"
	"time"
)

// The game's log tells, to the second, when each area was built, when the
// game marked the player away from the keyboard and when the game started:
//
//	2026/10/19 01:39:10 … : AFK mode is now ON. Autoreply "This player is AFK."
//	2026/10/19 01:50:54 … : AFK mode is now OFF.
//
// The farm's times (in maps, in the hideout, away) are cut at these. The
// log is read on from where the last read stopped, kept with the farm's
// sessions, so it is not read again from the start.

const (
	afkOn  = "] : AFK mode is now ON"
	afkOff = "] : AFK mode is now OFF"
)

// TimeMark is one such line: an area entered (Area set), AFK going on or
// off (AFK, with Area nil) or the game starting (Open; Prev is the time of
// the log's line before it, zero when not known).
type TimeMark struct {
	At   time.Time
	Area *Area
	AFK  bool
	Open bool
	Prev time.Time
}

// LogPos is where a read of the game's log stopped: the file, the offset
// of the next line and the time of the last line read.
type LogPos struct {
	Path   string    `json:"path"`
	Offset int64     `json:"offset"`
	Last   time.Time `json:"last,omitzero"`
}

// catchUpLimit is how far behind the log's end a read goes on from; past
// it (the app was closed while the game ran long) it starts at the current
// game session's first line.
const catchUpLimit = 64 << 20

// ReadMarks reads the marks of the log at path from pos on, whole lines
// only, and returns them with where to go on from. A pos in another file,
// past the log's end or too far behind starts again (Again) at the current
// game session's first line, or near the end when that is not found close
// enough; the time of the last line read stays from pos.
func ReadMarks(path string, pos LogPos) (marks []TimeMark, next LogPos, again bool, err error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, pos, false, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, pos, false, err
	}
	size := info.Size()
	if pos.Path != path || pos.Offset > size || size-pos.Offset > catchUpLimit {
		again = true
		pos = LogPos{Path: path, Offset: max(0, size-recentWindow), Last: pos.Last}
		if off, _, ok := sessionLine(f, size); ok && size-off <= catchUpLimit {
			pos.Offset = off
		}
	}
	r := bufio.NewReaderSize(io.NewSectionReader(f, pos.Offset, size-pos.Offset), 64<<10)
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			break // the rest is not a whole line yet: read next time
		}
		pos.Offset += int64(len(line))
		line = strings.TrimRight(line, "\r\n")
		at, ok := logTime(line)
		if !ok {
			continue
		}
		switch {
		case strings.Contains(line, sessionStart):
			marks = append(marks, TimeMark{At: at, Open: true, Prev: pos.Last})
		case strings.Contains(line, afkOn):
			marks = append(marks, TimeMark{At: at, AFK: true})
		case strings.Contains(line, afkOff):
			marks = append(marks, TimeMark{At: at})
		default:
			if a, ok := ParseArea(line); ok {
				marks = append(marks, TimeMark{At: at, Area: &a})
			}
		}
		pos.Last = at
	}
	return marks, pos, again, nil
}

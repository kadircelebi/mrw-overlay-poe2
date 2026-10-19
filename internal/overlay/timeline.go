package overlay

import (
	"io"
	"os"
	"strings"
	"sync"
	"time"
)

// The game's log tells, to the second, when each area was built and when
// the game marked the player away from the keyboard:
//
//	2026/10/19 01:39:10 … : AFK mode is now ON. Autoreply "This player is AFK."
//	2026/10/19 01:50:54 … : AFK mode is now OFF.
//
// The farm's times (in maps, in the hideout, away) are cut at these.

const (
	afkOn  = "] : AFK mode is now ON"
	afkOff = "] : AFK mode is now OFF"
)

// TimeMark is one such line: an area entered (Area set) or AFK going on or
// off (AFK, with Area nil).
type TimeMark struct {
	At   time.Time
	Area *Area
	AFK  bool
}

// Timeline keeps the marks of the current game session, reading only what
// the log added since the last look.
type Timeline struct {
	mu     sync.Mutex
	path   string
	offset int64
	began  time.Time
	last   time.Time
	marks  []TimeMark
}

// Marks returns the current game session's marks of the log at path, when
// it began and the time of the log's latest line.
func (t *Timeline) Marks(path string) (marks []TimeMark, began, last time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()
	f, err := os.Open(path)
	if err != nil {
		return nil, time.Time{}, time.Time{}
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, time.Time{}, time.Time{}
	}
	size := info.Size()
	if path != t.path || size < t.offset {
		// Another log, or the game began it again: from the session's
		// first line.
		t.path, t.marks, t.began, t.last = path, nil, time.Time{}, time.Time{}
		t.offset = max(0, size-recentWindow)
		if off, at, ok := sessionLine(f, size); ok {
			t.offset, t.began = off, at
		}
	}
	if size > t.offset {
		buf := make([]byte, size-t.offset)
		if _, err := f.ReadAt(buf, t.offset); err != nil && err != io.EOF {
			return t.copy()
		}
		// Only whole lines; the rest is read next time.
		end := strings.LastIndexByte(string(buf), '\n') + 1
		t.add(string(buf[:end]))
		t.offset += int64(end)
	}
	return t.copy()
}

func (t *Timeline) copy() ([]TimeMark, time.Time, time.Time) {
	return append([]TimeMark(nil), t.marks...), t.began, t.last
}

// add takes the marks of whole lines text; a new session starts over.
func (t *Timeline) add(text string) {
	for _, line := range strings.Split(text, "\n") {
		at, ok := logTime(line)
		if !ok {
			continue
		}
		t.last = at
		switch {
		case strings.Contains(line, sessionStart):
			t.marks, t.began = nil, at
		case strings.Contains(line, afkOn):
			t.marks = append(t.marks, TimeMark{At: at, AFK: true})
		case strings.Contains(line, afkOff):
			t.marks = append(t.marks, TimeMark{At: at})
		default:
			if a, ok := ParseArea(line); ok {
				t.marks = append(t.marks, TimeMark{At: at, Area: &a})
			}
		}
	}
}

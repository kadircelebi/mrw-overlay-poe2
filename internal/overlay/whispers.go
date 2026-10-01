package overlay

import (
	"bufio"
	"io"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
)

// A whisper arrives in Client.txt as
//
//	2026/10/01 23:21:14 673065906 3ef23348 [INFO Client 23484] @From <TAG> Name: text
//
// with the guild tag only when the sender has one.
var whisperFromRE = regexp.MustCompile(`\] @From (?:<[^>]*> )?([^:\s][^:]*?): `)

// WhisperFrom returns the sender of a whisper line of Client.txt.
func WhisperFrom(line string) (string, bool) {
	m := whisperFromRE.FindStringSubmatch(line)
	if m == nil {
		return "", false
	}
	return strings.TrimSpace(m[1]), true
}

// Whispers follows the end of the game's Client.txt for the player who
// whispered last. Only that name is kept, in memory; nothing else of the
// chat log is read into the app or stored.
type Whispers struct {
	mu     sync.Mutex
	path   string
	last   string
	cancel chan struct{}
}

// Last is the player who whispered last since the log was first followed.
func (w *Whispers) Last() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.last
}

// Follow starts reading path from its current end, unless it already
// follows that file. An empty path stops following.
func (w *Whispers) Follow(path string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if path == w.path {
		return
	}
	if w.cancel != nil {
		close(w.cancel)
		w.cancel = nil
	}
	w.path = path
	if path == "" {
		return
	}
	stop := make(chan struct{})
	w.cancel = stop
	go w.tail(path, stop)
}

func (w *Whispers) tail(path string, stop chan struct{}) {
	var offset int64 = -1 // -1: start at the end, the log is hundreds of MB
	tick := time.NewTicker(500 * time.Millisecond)
	defer tick.Stop()
	for {
		offset = w.read(path, offset)
		select {
		case <-stop:
			return
		case <-tick.C:
		}
	}
}

// sessionStart is the line the game writes each time it starts.
const sessionStart = "***** LOG FILE OPENING *****"

// recentWindow is how far back the first look goes: a session's latest
// whisper is near the end, and the log is hundreds of MB.
const recentWindow = 512 << 10

// recent takes the last whisper of the current game session from the end
// of the log, so one that arrived before the log was followed still counts.
// Whispers from an earlier session do not.
func (w *Whispers) recent(f *os.File, size int64) {
	start := max(0, size-recentWindow)
	buf := make([]byte, size-start)
	if _, err := f.ReadAt(buf, start); err != nil && err != io.EOF {
		return
	}
	last := ""
	for _, line := range strings.Split(string(buf), "\n") {
		if strings.Contains(line, sessionStart) {
			last = ""
		} else if name, ok := WhisperFrom(line); ok {
			last = name
		}
	}
	if last != "" {
		w.mu.Lock()
		w.last = last
		w.mu.Unlock()
	}
}

// read handles the lines added since offset and returns the new offset.
func (w *Whispers) read(path string, offset int64) int64 {
	f, err := os.Open(path)
	if err != nil {
		return offset
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return offset
	}
	size := info.Size()
	if offset < 0 { // first look: the whisper may have come just before
		w.recent(f, size)
		return size
	}
	if size < offset { // the log was cleared
		offset = 0
	}
	if size == offset {
		return offset
	}
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return offset
	}
	r := bufio.NewReader(f)
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			// A line still being written is read again next time.
			return offset
		}
		offset += int64(len(line))
		if name, ok := WhisperFrom(line); ok {
			w.mu.Lock()
			w.last = name
			w.mu.Unlock()
		}
	}
}

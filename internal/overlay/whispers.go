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

// merchantLine is how the game logs a line of Ange, the merchant who sells
// the player's items ("Ange: I have a buyer!"). The line names no item or
// price; it only tells that the sales history may have changed.
const merchantLine = "] Ange: "

// Whispers follows the end of the game's Client.txt for the player who
// whispered last, the time the game started, whether the merchant spoke and
// which area the game built. Only that is kept, in memory; nothing else of
// the chat log is read into the app or stored.
type Whispers struct {
	mu     sync.Mutex
	path   string
	last   string
	start  time.Time
	cancel chan struct{}
	// onMerchant, when set, is called for every line of the merchant.
	onMerchant func()
	// onArea, when set, is called for every area the game builds.
	onArea func(Area)
}

// SetOnMerchant sets what to call when the merchant speaks (nil: nothing).
func (w *Whispers) SetOnMerchant(f func()) {
	w.mu.Lock()
	w.onMerchant = f
	w.mu.Unlock()
}

// SetOnArea sets what to call when the game builds an area (nil: nothing).
func (w *Whispers) SetOnArea(f func(Area)) {
	w.mu.Lock()
	w.onArea = f
	w.mu.Unlock()
}

func (w *Whispers) area(a Area) {
	w.mu.Lock()
	on := w.onArea
	w.mu.Unlock()
	if on != nil {
		on(a)
	}
}

// SessionStart is when the game was last started, as its log tells (zero
// before the log was followed).
func (w *Whispers) SessionStart() time.Time {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.start
}

// logTime reads the local time a Client.txt line starts with.
func logTime(line string) (time.Time, bool) {
	if len(line) < 19 {
		return time.Time{}, false
	}
	t, err := time.ParseInLocation("2006/01/02 15:04:05", line[:19], time.Local)
	return t, err == nil
}

func (w *Whispers) merchant() {
	w.mu.Lock()
	on := w.onMerchant
	w.mu.Unlock()
	if on != nil {
		on()
	}
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

// sessionScanLimit is how far back sessionBegan looks for the session's
// first line.
const sessionScanLimit = 256 << 20

// sessionBegan finds the time of the last "LOG FILE OPENING" line before
// end, reading the log backwards a chunk at a time (zero when none is
// found within sessionScanLimit).
func sessionBegan(f *os.File, end int64) time.Time {
	_, at, _ := sessionLine(f, end)
	return at
}

// sessionLine finds where the last "LOG FILE OPENING" line before end
// begins and its time, as sessionBegan.
func sessionLine(f *os.File, end int64) (int64, time.Time, bool) {
	const chunk = 4 << 20
	overlap := int64(len(sessionStart) + 64)
	for hi := end; hi > 0 && end-hi < sessionScanLimit; hi -= chunk {
		lo := max(0, hi-chunk)
		buf := make([]byte, min(end, hi+overlap)-lo)
		if _, err := f.ReadAt(buf, lo); err != nil && err != io.EOF {
			return 0, time.Time{}, false
		}
		text := string(buf)
		if i := strings.LastIndex(text, sessionStart); i >= 0 {
			// The line starts with its time.
			lineStart := strings.LastIndex(text[:i], "\n") + 1
			at, ok := logTime(text[lineStart:])
			return lo + int64(lineStart), at, ok
		}
	}
	return 0, time.Time{}, false
}

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
	var began time.Time
	for _, line := range strings.Split(string(buf), "\n") {
		if strings.Contains(line, sessionStart) {
			last = ""
			if at, ok := logTime(line); ok {
				began = at
			}
		} else if name, ok := WhisperFrom(line); ok {
			last = name
		}
	}
	if began.IsZero() && start > 0 {
		// A long session began further back than the window.
		began = sessionBegan(f, start)
	}
	w.mu.Lock()
	if last != "" {
		w.last = last
	}
	if !began.IsZero() {
		w.start = began
	}
	w.mu.Unlock()
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
		} else if strings.Contains(line, sessionStart) {
			if at, ok := logTime(line); ok {
				w.mu.Lock()
				w.start, w.last = at, ""
				w.mu.Unlock()
			}
		} else if strings.Contains(line, merchantLine) {
			w.merchant()
		} else if a, ok := ParseArea(line); ok {
			w.area(a)
		}
	}
}

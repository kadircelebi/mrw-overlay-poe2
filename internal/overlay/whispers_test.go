package overlay

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWhisperFrom(t *testing.T) {
	for line, want := range map[string]string{
		"2026/10/01 23:21:14 673065906 3ef23348 [INFO Client 23484] @From SomeExile: a":                          "SomeExile",
		"2026/10/01 22:13:19 668990593 3ef23348 [INFO Client 33056] @From <GUILD> Tag_Name: Hi, I would like to": "Tag_Name",
		"2026/10/01 19:38:36 659708031 3ef23348 [INFO Client 33056] @To SomeExile: dcye bak":                     "",
		"2026/10/01 19:38:36 659708031 3ef23348 [INFO Client 33056] #Global: @From Fake: no":                     "",
	} {
		got, _ := WhisperFrom(line)
		if got != want {
			t.Errorf("%q: got %q, want %q", line, got, want)
		}
	}
}

// Old whispers already in the log do not count; new ones do, a half-written
// line waits for its end, and a cleared log starts over.
func TestWhispersFollowTheEndOfTheLog(t *testing.T) {
	path := filepath.Join(t.TempDir(), "Client.txt")
	write := func(s string, flag int) {
		f, err := os.OpenFile(path, flag|os.O_WRONLY|os.O_CREATE, 0o644)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if _, err := f.WriteString(s); err != nil {
			t.Fatal(err)
		}
	}
	write("x [INFO Client 1] @From OldFriend: hi\n2026/10/01 23:19:44 ***** LOG FILE OPENING *****\n", os.O_TRUNC)
	var w Whispers
	off := w.read(path, -1)
	if w.Last() != "" {
		t.Fatal("a whisper from an earlier game session counted")
	}
	write("x [INFO Client 1] @From Buyer: wtb\nx [INFO Client 1] @From Half", os.O_APPEND)
	off = w.read(path, off)
	if w.Last() != "Buyer" {
		t.Fatalf("last = %q", w.Last())
	}
	write(": done\n", os.O_APPEND)
	off = w.read(path, off)
	if w.Last() != "Half" {
		t.Fatalf("last = %q", w.Last())
	}
	write("x [INFO Client 1] @From Fresh: new log\n", os.O_TRUNC)
	w.read(path, off)
	if w.Last() != "Fresh" {
		t.Fatalf("after the log was cleared: %q", w.Last())
	}
}

// A whisper that came in this game session before the log was followed,
// such as while the app restarted, is the last whisperer.
func TestWhispersPickUpThisSessionsLastWhisper(t *testing.T) {
	path := filepath.Join(t.TempDir(), "Client.txt")
	log := "x [INFO Client 1] @From Earlier: old session\n" +
		"2026/10/01 23:19:44 ***** LOG FILE OPENING *****\n" +
		"x [INFO Client 1] @From First: a\n" +
		"x [INFO Client 1] @To First: hi\n" +
		"x [INFO Client 1] @From <TAG> Second: b\n" +
		"x [INFO Client 1] Generating level 80 area\n"
	if err := os.WriteFile(path, []byte(log), 0o644); err != nil {
		t.Fatal(err)
	}
	var w Whispers
	w.read(path, -1)
	if w.Last() != "Second" {
		t.Fatalf("last = %q", w.Last())
	}
}

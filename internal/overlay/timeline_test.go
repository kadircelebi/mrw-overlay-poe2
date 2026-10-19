package overlay

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadMarksStartsAtTheSessionAndGoesOnFromWhereItStopped(t *testing.T) {
	path := filepath.Join(t.TempDir(), "Client.txt")
	write := func(text string, appendTo bool) {
		flag := os.O_CREATE | os.O_WRONLY | os.O_TRUNC
		if appendTo {
			flag = os.O_CREATE | os.O_WRONLY | os.O_APPEND
		}
		f, err := os.OpenFile(path, flag, 0o644)
		if err != nil {
			t.Fatal(err)
		}
		f.WriteString(text)
		f.Close()
	}
	write(`2026/10/18 20:00:00 ***** LOG FILE OPENING *****
2026/10/18 20:01:00 1 a [DEBUG Client 1] Generating level 79 area "MapOld" with seed 1
2026/10/19 09:00:00 ***** LOG FILE OPENING *****
2026/10/19 09:01:00 1 a [DEBUG Client 1] Generating level 65 area "HideoutShrine" with seed 1
2026/10/19 09:02:00 1 a [INFO Client 1] : AFK mode is now ON. Autoreply "This player is AFK."
2026/10/19 09:03:00 1 a [INFO Client 1] @From Someone: AFK mode is now ON
2026/10/19 09:04:00 1 a [INFO Client 1] : AFK mode is now OFF.
2026/10/19 09:05:00 1 a [DEBUG Client 1] Generating level 79 area "MapCenotes" with seed 22
2026/10/19 09:06:00 1 a [DEBUG Client 1] Generating level 79 area "MapHalf`, false)
	marks, pos, again, err := ReadMarks(path, LogPos{})
	if err != nil || !again || len(marks) != 5 || pos.Last.Minute() != 5 {
		t.Fatalf("first read: %d marks, again %v, pos %+v, %v", len(marks), again, pos, err)
	}
	if !marks[0].Open || marks[0].At.Hour() != 9 || marks[1].Area == nil || marks[1].Area.ID != "HideoutShrine" || !marks[2].AFK || marks[3].AFK || marks[3].Area != nil || marks[4].Area.ID != "MapCenotes" {
		t.Fatalf("marks %+v", marks)
	}
	write("Way\" with seed 3\r\n", true)
	marks, pos, again, _ = ReadMarks(path, pos)
	if again || len(marks) != 1 || marks[0].Area.ID != "MapHalfWay" || marks[0].Area.Seed != "3" {
		t.Fatalf("the line finished later: %+v again %v", marks, again)
	}
	write("2026/10/19 11:00:00 ***** LOG FILE OPENING *****\n", true)
	marks, pos, _, _ = ReadMarks(path, pos)
	if len(marks) != 1 || !marks[0].Open || marks[0].Prev.Minute() != 6 {
		t.Fatalf("the game started again: %+v", marks)
	}
	if marks, _, _, _ := ReadMarks(path, pos); len(marks) != 0 {
		t.Fatalf("nothing new, read again: %+v", marks)
	}
	// A log that no longer reaches pos (another file in its place) starts
	// at its session, keeping the last line's time.
	write("2026/10/19 12:00:00 ***** LOG FILE OPENING *****\n", false)
	marks, _, again, _ = ReadMarks(path, pos)
	if !again || len(marks) != 1 || marks[0].Prev.Hour() != 11 {
		t.Fatalf("a shorter log: %+v again %v", marks, again)
	}
}

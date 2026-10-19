package overlay

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTimelineReadsTheSessionAndThenOnlyWhatIsAdded(t *testing.T) {
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
	var tl Timeline
	marks, began, last := tl.Marks(path)
	if len(marks) != 4 || began.Hour() != 9 || last.Minute() != 5 {
		t.Fatalf("first look: %d marks, began %v, last %v", len(marks), began, last)
	}
	if marks[0].Area == nil || marks[0].Area.ID != "HideoutShrine" || !marks[1].AFK || marks[2].AFK || marks[2].Area != nil || marks[3].Area.ID != "MapCenotes" {
		t.Fatalf("marks %+v", marks)
	}
	write(`Way" with seed 3
`, true)
	if marks, _, _ := tl.Marks(path); len(marks) != 5 || marks[4].Area.ID != "MapHalfWay" {
		t.Fatalf("the line finished later: %+v", marks)
	}
	write("2026/10/19 11:00:00 ***** LOG FILE OPENING *****\n", true)
	if marks, began, _ := tl.Marks(path); len(marks) != 0 || began.Hour() != 11 {
		t.Fatalf("a new session: %d marks, began %v", len(marks), began)
	}
}

package app

import (
	"image"
	"testing"
)

// A click falls on the row whose band holds it; bands split halfway between
// the lines, and a click right of the panel or far left of it is no choice.
func TestPickExpeditionRow(t *testing.T) {
	client := image.Rect(100, 50, 2660, 1490)
	view := ExpeditionView{PanelRight: 700, Rows: []ExpeditionPrice{
		{Name: "A", X: 300, Y: 400, W: 300, H: 20},
		{Name: "C", X: 320, Y: 520, W: 280, H: 20},
		{Name: "B", X: 310, Y: 460, W: 290, H: 20},
	}}
	at := func(cx, cy int) int { return pickExpeditionRow(view, client, client.Min.X+cx, client.Min.Y+cy) }
	cases := []struct {
		cx, cy int
		want   string
	}{
		{400, 410, "A"}, // on the text
		{400, 385, "A"}, // above it, inside the band
		{250, 465, "B"}, // over the icons left of the text
		{690, 485, "B"}, // just above the halfway line to C
		{400, 505, "C"}, // just below it
		{400, 495, "B"},
		{400, 555, "C"},
		{400, 600, ""}, // below the last band
		{800, 470, ""}, // right of the panel
		{20, 470, ""},  // far left
	}
	for _, c := range cases {
		got := ""
		if i := at(c.cx, c.cy); i >= 0 {
			got = view.Rows[i].Name
		}
		if got != c.want {
			t.Errorf("click %d,%d: got %q, want %q", c.cx, c.cy, got, c.want)
		}
	}
	if i := pickExpeditionRow(ExpeditionView{}, client, 400, 400); i != -1 {
		t.Errorf("no rows: %d", i)
	}
}

// The box grows a line for each of the map's choices, up to the limit and
// a line for the rest.
func TestExpeditionStripSize(t *testing.T) {
	if w, h := expeditionStripSize(0); w != expeditionStripWidth || h != expeditionStripHeight {
		t.Errorf("no names %dx%d", w, h)
	}
	_, h2 := expeditionStripSize(2)
	_, h8 := expeditionStripSize(expeditionStripNameRows)
	_, h9 := expeditionStripSize(expeditionStripNameRows + 1)
	_, h20 := expeditionStripSize(20)
	if h2-expeditionStripHeight != 4+2*expeditionStripRowHeight || h9 != h8+expeditionStripRowHeight || h20 != h9 {
		t.Errorf("heights %d %d %d %d", h2, h8, h9, h20)
	}
}

func TestMapName(t *testing.T) {
	for id, want := range map[string]string{
		"MapWaywardIsle":    "Wayward Isle",
		"MapCrypt":          "Crypt",
		"ExpeditionLogbook": "Expedition Logbook",
		"Map_Hidden_Grotto": "Hidden Grotto",
	} {
		if got := mapName(id); got != want {
			t.Errorf("%s: %q, want %q", id, got, want)
		}
	}
}

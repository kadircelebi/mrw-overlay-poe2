package filter

import (
	"strings"
	"testing"
)

// The game rejects the whole filter over one bad look line ("Line 187:
// Unable to read minimap icon data" in 2.10.0, from a palette without a beam
// colour). Every group with every app palette, and with its own default, must
// write only lines the game reads.
func TestEveryLookLineIsValid(t *testing.T) {
	colours := map[string]bool{"Red": true, "Green": true, "Blue": true, "Brown": true, "White": true, "Yellow": true,
		"Cyan": true, "Grey": true, "Orange": true, "Pink": true, "Purple": true}
	shapes := map[string]bool{"Circle": true, "Diamond": true, "Hexagon": true, "Square": true, "Star": true, "Triangle": true,
		"Cross": true, "Moon": true, "Raindrop": true, "Kite": true, "Pentagon": true, "UpsideDownHouse": true}
	themes := []string{""} // each group's own default
	for _, th := range ThemeList {
		themes = append(themes, th.ID)
	}
	for _, theme := range themes {
		cfg := DefaultConfig()
		cfg.Exotics = nsExoticBlocks
		cfg.Styles = map[string]string{}
		for _, g := range StyleGroups {
			if theme != "" {
				cfg.Styles[g.ID] = theme
			}
		}
		out, _ := GenerateDynamicFilterBlock(cfg, testSnapshot(), testBases, nil)
		for n, line := range strings.Split(out, "\n") {
			f := strings.Fields(line)
			if len(f) == 0 {
				continue
			}
			switch f[0] {
			case "MinimapIcon":
				if len(f) != 4 || !strings.Contains("012", f[1]) || !colours[f[2]] || !shapes[f[3]] {
					t.Errorf("theme %q line %d: %q", theme, n+1, line)
				}
			case "PlayEffect":
				if len(f) < 2 || len(f) > 3 || !colours[f[1]] || (len(f) == 3 && f[2] != "Temp") {
					t.Errorf("theme %q line %d: %q", theme, n+1, line)
				}
			}
		}
	}
}

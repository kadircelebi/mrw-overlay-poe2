package filter

import (
	"fmt"
	"strings"
	"testing"
)

func selfCheckKnowledge() (map[string]bool, map[string]string) {
	classes := map[string]bool{}
	for _, c := range []string{"Stackable Currency", "Wands", "Rings", "Amulets", "Belts", "Gloves", "Boots",
		"Helmets", "Body Armours", "Jewels", "Waystones", "Uncut Skill Gems"} {
		classes[strings.ToLower(c)] = true
	}
	bases := map[string]string{}
	for _, b := range []string{"Divine Orb", "Exalted Orb", "Gold", "Mirror of Kalandra", "Heavy Belt",
		"Waystone (Tier 15)"} {
		bases[strings.ToLower(b)] = b
	}
	for i := 0; len(bases) < selfCheckMinBases; i++ {
		bases[fmt.Sprintf("filler base %d", i)] = fmt.Sprintf("Filler Base %d", i)
	}
	return classes, bases
}

func TestSelfCheckKeepsAKnownBlockAsItIs(t *testing.T) {
	classes, bases := selfCheckKnowledge()
	block := "# comment\nShow\n    Class == \"Stackable Currency\"\n    BaseType == \"Divine Orb\" \"Exalted Orb\"\n    SetFontSize 45\n\n" +
		"Hide\n    Class \"Wand\"\n    BaseType \"Uncut Skill Gem\"\n    Rarity Normal\n"
	// "Uncut Skill Gem" is no item name; the base filter uses it loosely.
	got, res := SelfCheck(block, classes, bases, []string{"uncut skill gem", "uncut spirit gem"})
	if got != block || len(res.Unknown) != 0 || res.Dropped != 0 {
		t.Fatalf("changed a valid block: %+v\n%s", res, got)
	}
}

func TestSelfCheckRemovesUnknownValuesAndEmptyRules(t *testing.T) {
	classes, bases := selfCheckKnowledge()
	block := strings.Join([]string{
		"Show",
		`    Class == "Stackable Currency"`,
		`    BaseType == "Divine Orb" "Removed Orb"`,
		"    SetFontSize 45",
		"",
		"Hide",
		`    Class == "Removed Class"`,
		"    Rarity Normal Magic Rare",
		"",
		"Show",
		`    BaseType "Nonexistent"`,
		"",
		"Show",
		`    Class != "Removed Class"`,
		`    BaseType == "Gold"`,
		"",
	}, "\n")
	got, res := SelfCheck(block, classes, bases, nil)
	want := strings.Join([]string{
		"Show",
		`    Class == "Stackable Currency"`,
		`    BaseType == "Divine Orb"`,
		"    SetFontSize 45",
		"",
		"",
		"",
		"Show",
		`    BaseType == "Gold"`,
		"",
	}, "\n")
	if got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
	if res.Dropped != 2 || strings.Join(res.Unknown, "|") != `BaseType Nonexistent|BaseType Removed Orb|Class Removed Class` {
		t.Fatalf("result %+v", res)
	}
	// The dropped Hide must not have turned into "hide everything".
	if strings.Contains(got, "Rarity Normal Magic Rare") {
		t.Fatal("a rule lost its Class condition but stayed")
	}
}

func TestSelfCheckTrustsNothingWhenKnowledgeIsMissing(t *testing.T) {
	block := "Show\n    Class == \"Anything\"\n    BaseType == \"Whatever\"\n"
	if got, res := SelfCheck(block, map[string]bool{"wands": true}, map[string]string{"x": "X"}, nil); got != block || len(res.Unknown) != 0 {
		t.Fatalf("checked against too little knowledge: %+v", res)
	}
}

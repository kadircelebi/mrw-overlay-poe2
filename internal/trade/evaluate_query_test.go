package trade

import (
	"encoding/json"
	"strings"
	"testing"
)

func f64(v float64) *float64 { return &v }

// A Market search from scratch has no item: every filled field must still
// reach the trade query, in the shape the trade site expects.
func TestEvaluateQuerySendsEveryFilledField(t *testing.T) {
	q, err := buildEvaluateQuery(EvaluateRequest{
		Status: "available",
		Groups: []SelectedStatGroup{{
			Type: "count", Min: f64(1), Max: f64(2),
			Stats: []SelectedStat{{ID: "explicit.stat_1", Min: f64(10)}, {ID: "explicit.stat_2"}},
		}},
		Filters: []SelectedFilter{
			{Group: "type_filters", ID: "category", Option: "armour.helmet"},
			{Group: "equipment_filters", ID: "rune_sockets", Min: f64(2)},
			{Group: "trade_filters", ID: "price", Option: "divine", Max: f64(3)},
			{Group: "trade_filters", ID: "account", Input: " Seller#1234 "},
			{Group: "misc_filters", ID: "corrupted"}, // left empty: not sent
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(q)
	got := string(raw)
	for _, want := range []string{
		`"status":{"option":"available"}`,
		`"type":"count"`,
		`"value":{"min":1,"max":2}`,
		`{"id":"explicit.stat_1","value":{"min":10}}`,
		`{"id":"explicit.stat_2"}`,
		`"category":{"option":"armour.helmet"}`,
		`"rune_sockets":{"min":2}`,
		`"price":{"max":3,"option":"divine"}`,
		`"account":{"input":"Seller#1234"}`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("query lacks %s\n%s", want, got)
		}
	}
	if strings.Contains(got, "corrupted") || strings.Contains(got, `"type":"`+"\"") {
		t.Errorf("empty filter leaked into query: %s", got)
	}
	if _, ok := q["type"]; ok {
		t.Errorf("fresh search must not send a base type: %s", got)
	}
}

func TestEvaluateQueryNeedsACriterion(t *testing.T) {
	if _, err := buildEvaluateQuery(EvaluateRequest{Rarity: "rare", Filters: []SelectedFilter{{Group: "type_filters", ID: "rarity", Option: "rare"}}}); err == nil {
		t.Fatal("a rarity-only search should be refused")
	}
	for _, in := range []EvaluateRequest{
		{BaseType: "Expert Hubris Circlet"},
		{Stats: []SelectedStat{{ID: "explicit.stat_1"}}},
		{Filters: []SelectedFilter{{Group: "equipment_filters", ID: "rune_sockets", Min: f64(3)}}},
	} {
		if _, err := buildEvaluateQuery(in); err != nil {
			t.Errorf("%+v: %v", in, err)
		}
	}
}

// The game calls a stack of orbs "Rarity: Currency"; the trade API rejects
// that rarity, so the search must go out without one.
func TestEvaluateQueryDropsRaritiesTheTradeSiteRejects(t *testing.T) {
	q, err := buildEvaluateQuery(EvaluateRequest{BaseType: "Chaos Orb", Rarity: "currency",
		Filters: []SelectedFilter{{Group: "type_filters", ID: "rarity", Option: "currency"}}})
	if err != nil {
		t.Fatal(err)
	}
	if raw, _ := json.Marshal(q); strings.Contains(string(raw), "rarity") {
		t.Errorf("unknown rarity sent: %s", raw)
	}
	q, _ = buildEvaluateQuery(EvaluateRequest{BaseType: "Slipstrike Vest", Rarity: "Normal"})
	if raw, _ := json.Marshal(q); !strings.Contains(string(raw), `"rarity":{"option":"normal"}`) {
		t.Errorf("valid rarity lost: %s", raw)
	}
}

func TestEvaluateSortDefaultsAndValidation(t *testing.T) {
	cases := []struct {
		key, dir         string
		wantKey, wantDir string
		wantErr          bool
	}{
		{"", "", "price", "asc", false},
		{"price", "desc", "price", "desc", false},
		{"pdps", "", "pdps", "desc", false},
		{"stat.explicit.stat_1509134228", "asc", "stat.explicit.stat_1509134228", "asc", false},
		{"stat.pseudo.pseudo_total_life", "", "stat.pseudo.pseudo_total_life", "desc", false},
		{"statgroup.0", "", "statgroup.0", "desc", false},
		{"price\"}", "", "", "", true},
		{"dps", "sideways", "", "", true},
	}
	for _, c := range cases {
		key, dir, err := evaluateSort(EvaluateRequest{Sort: c.key, SortDir: c.dir})
		if (err != nil) != c.wantErr || key != c.wantKey || dir != c.wantDir {
			t.Errorf("evaluateSort(%q,%q) = %q,%q,%v", c.key, c.dir, key, dir, err)
		}
	}
}

func TestFetchEvaluatedRejectsBadIDs(t *testing.T) {
	c := NewInteractiveClient("Standard", 0.4)
	if _, err := c.FetchEvaluated(t.Context(), "abc", []string{"../x"}); err == nil {
		t.Fatal("expected invalid id error")
	}
	ids := make([]string, FetchPageSize+1)
	for i := range ids {
		ids[i] = "0123456789abcdef"
	}
	if _, err := c.FetchEvaluated(t.Context(), "abc", ids); err == nil {
		t.Fatal("expected page size error")
	}
}

func TestSplitModLineNarrowsEachAffixShare(t *testing.T) {
	var line evaluatedModLine
	if err := json.Unmarshal([]byte(`{"description":"137% increased [Evasion] and [EnergyShield|Energy Shield]","hash":"stat.explicit.stat_1999113824",
		"mods":[{"name":"Illusory","tier":"P1","magnitudes":[{"min":"92","max":"100"}]},{"name":"Trickster's","tier":"P1","magnitudes":[{"min":"39","max":"42"}]}]}`), &line); err != nil {
		t.Fatal(err)
	}
	parts := splitModLine(cleanTradeDescription(line.Description), line)
	if len(parts) != 2 || parts[0].Description != "95–98% increased Evasion and Energy Shield" || parts[1].Description != "39–42% increased Evasion and Energy Shield" || parts[1].Name != "Trickster's" {
		t.Fatalf("parts = %+v", parts)
	}
	// A sum at the top of both ranges pins each share.
	line.Description = "142% increased Evasion and Energy Shield"
	parts = splitModLine(line.Description, line)
	if parts[0].Description != "100% increased Evasion and Energy Shield" || parts[1].Description != "42% increased Evasion and Energy Shield" {
		t.Fatalf("pinned parts = %+v", parts)
	}
	// One affix: nothing to split.
	line.Mods = line.Mods[:1]
	if parts := splitModLine(line.Description, line); parts != nil {
		t.Fatalf("single affix split = %+v", parts)
	}
}

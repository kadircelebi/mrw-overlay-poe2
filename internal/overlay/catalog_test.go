package overlay

import (
	"encoding/json"
	"testing"
)

func TestCurrenciesFromStaticData(t *testing.T) {
	raw := `{"result":[{"id":"Currency","entries":[
		{"id":"divine","text":"Divine Orb","image":"/gen/image/abc/CurrencyModValues.png"},
		{"id":"sep","text":""},
		{"id":"exalted","text":"Exalted Orb","image":"https://example.test/ex.png"}]},
		{"id":"Misc","entries":[{"id":"divine","text":"Duplicate","image":"/x.png"}]}]}`
	var static response[staticGroup]
	if err := json.Unmarshal([]byte(raw), &static); err != nil {
		t.Fatal(err)
	}
	got := currenciesFrom(static.Result)
	if len(got) != 2 || got[0].Image != "https://web.poecdn.com/gen/image/abc/CurrencyModValues.png" || got[0].Text != "Divine Orb" || got[1].Image != "https://example.test/ex.png" {
		t.Fatalf("currencies = %+v", got)
	}
}

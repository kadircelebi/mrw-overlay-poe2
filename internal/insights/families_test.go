package insights

import "testing"

func TestFamiliesCollectWholeWordMembers(t *testing.T) {
	valid := map[string]string{
		"flesh catalyst":          "Flesh Catalyst",
		"refined xoph's catalyst": "Refined Xoph's Catalyst",
		"vaal catalysing infuser": "Vaal Catalysing Infuser",
		"essence of the body":     "Essence of the Body",
	}
	got := Families("cata", valid)
	if len(got) != 1 || got[0].Name != "Catalyst" || len(got[0].Members) != 2 {
		t.Fatalf("Families(cata) = %+v", got)
	}
	// One essence is not a family worth offering.
	if got := Families("ess", valid); len(got) != 0 {
		t.Fatalf("Families(ess) = %+v", got)
	}
}

package useragent

import (
	"strings"
	"testing"
)

func TestValueNamesVersionAndContact(t *testing.T) {
	Set("MrW-Overlay", "2.6.0")
	got := Value()
	if !strings.HasPrefix(got, "MrW-Overlay/2.6.0 ") || !strings.Contains(got, "contact: "+Contact) {
		t.Fatalf("User-Agent %q", got)
	}
}

package app

import (
	"strings"
	"testing"

	"poe2filter/internal/i18n"
)

func TestCommunityLinksOnlyOpenFixedAddresses(t *testing.T) {
	defer i18n.Set(i18n.Current())
	for _, kind := range []string{"", "https://evil.example/", "settings"} {
		if got := communityURL(kind); got != "" {
			t.Errorf("communityURL(%q) = %q, want empty", kind, got)
		}
	}
	for _, kind := range (&AppService{}).CommunityLinks() {
		if url := communityURL(kind); !strings.HasPrefix(url, "https://") {
			t.Errorf("%s link %q is not https", kind, url)
		}
	}
	i18n.Set(i18n.TR)
	if got := communityURL("contact"); got != siteURL+"tr/contact/" {
		t.Errorf("Turkish contact page = %q", got)
	}
	i18n.Set(i18n.EN)
	if got := communityURL("contact"); got != siteURL+"contact/" {
		t.Errorf("English contact page = %q", got)
	}
}

package main

import (
	"errors"

	"poe2filter/internal/i18n"
)

// Links in the main panel's community row. An empty URL hides its button, so
// a link goes live by filling it in here.
const (
	discordURL  = ""
	sponsorsURL = "" // https://github.com/sponsors/kadircelebi once GitHub approves the account
	siteURL     = "https://poe2.mrwproject.com/"
)

// communityURL returns the fixed address behind a community button, or "" when
// that button is not shown. The contact page follows the interface language.
func communityURL(kind string) string {
	switch kind {
	case "discord":
		return discordURL
	case "support":
		return sponsorsURL
	case "contact":
		if i18n.Current() == i18n.TR {
			return siteURL + "tr/contact/"
		}
		return siteURL + "contact/"
	}
	return ""
}

// CommunityLinks lists the community buttons that have an address, in order.
func (s *AppService) CommunityLinks() []string {
	var out []string
	for _, kind := range []string{"discord", "contact", "support"} {
		if communityURL(kind) != "" {
			out = append(out, kind)
		}
	}
	return out
}

// OpenCommunityLink opens one of the fixed community addresses in the default
// browser. The frontend only names the button; it never supplies a URL.
func (s *AppService) OpenCommunityLink(kind string) error {
	url := communityURL(kind)
	if url == "" {
		return errors.New("unknown community link")
	}
	return s.app.Browser.OpenURL(url)
}

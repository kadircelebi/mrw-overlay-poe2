//go:build !windows

package i18n

import (
	"os"
	"strings"
)

// detect reads the POSIX locale environment; used for tests and the CI build.
func detect() Lang {
	for _, k := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		if v := os.Getenv(k); v != "" {
			return fromTag(strings.SplitN(v, ".", 2)[0])
		}
	}
	return EN
}

func fromTag(tag string) Lang {
	t := strings.ToLower(strings.ReplaceAll(tag, "_", "-"))
	switch {
	case strings.HasPrefix(t, "tr"):
		return TR
	case t == "zh-tw" || t == "zh-hk" || t == "zh-mo" || strings.Contains(t, "hant"):
		return ZH
	default:
		return EN
	}
}

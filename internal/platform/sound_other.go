//go:build !windows

package platform

import (
	"errors"

	"poe2filter/internal/i18n"
)

func PlaySound(string, int, int) error { return errors.New(i18n.T("err.soundWindowsOnly")) }

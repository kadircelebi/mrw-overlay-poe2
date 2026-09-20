//go:build !windows

package main

import (
	"errors"

	"poe2filter/internal/i18n"
)

func playSound(string) error { return errors.New(i18n.T("err.soundWindowsOnly")) }

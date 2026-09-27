//go:build !windows

package main

import "errors"

// packageAUMID: MSIX packages exist only on Windows.
func packageAUMID() string { return "" }

func pushPackagedToast(aumid, title, body string) error {
	return errors.New("packaged toasts are Windows only")
}

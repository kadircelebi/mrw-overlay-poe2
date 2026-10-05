//go:build !windows

package platform

import "errors"

// PackageAUMID: MSIX packages exist only on Windows.
func PackageAUMID() string { return "" }

func PushPackagedToast(aumid, title, body string) error {
	return errors.New("packaged toasts are Windows only")
}

//go:build store

package platform

// StoreBuild is true in the Microsoft Store package (go build -tags store).
// The Store installs and updates the app itself, so the GitHub updater and
// its tray entry are switched off.
const StoreBuild = true

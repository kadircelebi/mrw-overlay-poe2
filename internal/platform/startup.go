// Package platform contains operating-system integration without Wails services.
package platform

// StartupStatus describes the startup controls Windows permits for this app.
type StartupStatus struct {
	Enabled   bool   `json:"enabled"`
	CanChange bool   `json:"canChange"`
	State     string `json:"state"`
}

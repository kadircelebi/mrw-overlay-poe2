package app

import "poe2filter/internal/platform"

// StartupStatus is read from Windows rather than saved in the filter config.
type StartupStatus = platform.StartupStatus

func (s *AppService) GetStartupStatus() (StartupStatus, error) { return platform.Startup(nil) }
func (s *AppService) SetStartupEnabled(enabled bool) (StartupStatus, error) {
	return platform.Startup(&enabled)
}
func (s *AppService) OpenStartupSettings() error { return platform.OpenStartupSettings() }

package main

// StartupStatus is read from Windows rather than saved in the filter config.
type StartupStatus struct {
	Enabled   bool   `json:"enabled"`
	CanChange bool   `json:"canChange"`
	State     string `json:"state"`
}

func (s *AppService) GetStartupStatus() (StartupStatus, error) { return startupStatus(nil) }
func (s *AppService) SetStartupEnabled(enabled bool) (StartupStatus, error) {
	return startupStatus(&enabled)
}

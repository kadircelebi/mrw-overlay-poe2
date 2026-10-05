//go:build !windows

package platform

func Startup(_ *bool) (StartupStatus, error) { return StartupStatus{State: "unsupported"}, nil }
func OpenStartupSettings() error             { return nil }

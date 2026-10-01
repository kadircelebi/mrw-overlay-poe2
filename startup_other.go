//go:build !windows

package main

func startupStatus(_ *bool) (StartupStatus, error) { return StartupStatus{State: "unsupported"}, nil }
func (s *AppService) OpenStartupSettings() error   { return nil }

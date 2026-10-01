//go:build windows

package main

import (
	"encoding/json"
	"os"
	"testing"
)

func TestPackagedStartupReadOnly(t *testing.T) {
	if packageAUMID() == "" {
		t.Skip("requires package identity")
	}
	status, err := startupStatus(nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("startup: %+v", status)
	if path := os.Getenv("MRW_STARTUP_PROBE_OUTPUT"); path != "" {
		data, _ := json.Marshal(status)
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestStartupStatesRespectWindowsControl(t *testing.T) {
	for _, tc := range []struct {
		state           int32
		enabled, change bool
	}{
		{0, false, true}, {1, false, false}, {2, true, true}, {3, false, false}, {4, true, false}, {99, false, false},
	} {
		got := startupState(tc.state)
		if got.Enabled != tc.enabled || got.CanChange != tc.change {
			t.Fatalf("state %d: %+v", tc.state, got)
		}
	}
}

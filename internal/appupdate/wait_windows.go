//go:build windows

package appupdate

import (
	"fmt"
	"time"

	"golang.org/x/sys/windows"
)

func waitForProcess(pid int, timeout time.Duration) error {
	handle, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		// The process disappeared between launch and OpenProcess.
		if err == windows.ERROR_INVALID_PARAMETER {
			return nil
		}
		return fmt.Errorf("could not wait for the old application: %w", err)
	}
	defer windows.CloseHandle(handle)
	result, err := windows.WaitForSingleObject(handle, uint32(timeout/time.Millisecond))
	if err != nil {
		return fmt.Errorf("could not wait for the old application: %w", err)
	}
	if result == uint32(windows.WAIT_TIMEOUT) {
		return fmt.Errorf("timed out waiting for process %d to close", pid)
	}
	if result != uint32(windows.WAIT_OBJECT_0) {
		return fmt.Errorf("unexpected wait result %d for process %d", result, pid)
	}
	return nil
}

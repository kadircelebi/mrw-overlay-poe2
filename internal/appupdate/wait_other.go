//go:build !windows

package appupdate

import (
	"fmt"
	"syscall"
	"time"
)

func waitForProcess(pid int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if err := syscall.Kill(pid, 0); err != nil {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("timed out waiting for process %d to close", pid)
}

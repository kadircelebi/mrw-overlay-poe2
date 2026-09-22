package appupdate

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Apply replaces target with the currently running staged executable. It is
// called before Wails starts, so this helper process does not take the normal
// single-instance lock.
func Apply(target, dataDir, outPath string, waitPID int) error {
	staged, err := os.Executable()
	if err != nil {
		return err
	}
	target, err = filepath.Abs(target)
	if err != nil {
		return err
	}
	staged, err = filepath.Abs(staged)
	if err != nil {
		return err
	}
	if strings.EqualFold(target, staged) || !strings.EqualFold(filepath.Ext(target), ".exe") {
		return errors.New("invalid application update target")
	}

	backup := target + ".old"
	newPath := target + ".new"
	_ = os.Remove(newPath)
	_ = os.Remove(backup)
	if waitPID > 0 {
		if err := waitForProcess(waitPID, 45*time.Second); err != nil {
			_ = exec.Command(target, runtimeArgs(dataDir, outPath, "")...).Start()
			return err
		}
	}

	// Antivirus software can briefly retain the image after the old process is
	// gone, so the rename still gets a bounded retry loop.
	deadline := time.Now().Add(45 * time.Second)
	for {
		err = os.Rename(target, backup)
		if err == nil {
			break
		}
		if time.Now().After(deadline) {
			_ = exec.Command(target, runtimeArgs(dataDir, outPath, "")...).Start()
			return fmt.Errorf("timed out waiting for the old application to close: %w", err)
		}
		time.Sleep(200 * time.Millisecond)
	}

	rollback := func() {
		_ = os.Remove(newPath)
		_ = os.Remove(target)
		_ = os.Rename(backup, target)
	}
	if err := copyFile(staged, newPath); err != nil {
		rollback()
		_ = exec.Command(target, runtimeArgs(dataDir, outPath, "")...).Start()
		return fmt.Errorf("could not stage the new application: %w", err)
	}
	if err := os.Rename(newPath, target); err != nil {
		rollback()
		_ = exec.Command(target, runtimeArgs(dataDir, outPath, "")...).Start()
		return fmt.Errorf("could not replace the application: %w", err)
	}

	cmd := exec.Command(target, runtimeArgs(dataDir, outPath, staged)...)
	if err := cmd.Start(); err != nil {
		rollback()
		_ = exec.Command(target, runtimeArgs(dataDir, outPath, "")...).Start()
		return fmt.Errorf("could not restart the updated application: %w", err)
	}

	// If the new build dies immediately, restore and reopen the previous one.
	// Otherwise the new process removes both staging and backup after startup.
	wait := make(chan error, 1)
	go func() { wait <- cmd.Wait() }()
	select {
	case <-time.After(8 * time.Second):
		return nil
	case err := <-wait:
		rollback()
		_ = exec.Command(target, runtimeArgs(dataDir, outPath, "")...).Start()
		if err == nil {
			return errors.New("the updated application exited during startup")
		}
		return fmt.Errorf("the updated application exited during startup: %w", err)
	}
}

func runtimeArgs(dataDir, outPath, cleanup string) []string {
	args := []string{"-data", dataDir}
	if outPath != "" {
		args = append(args, "-out", outPath)
	}
	if cleanup != "" {
		args = append(args, "-cleanup-update", cleanup)
	}
	return args
}

// CleanupAfterStart runs in the installed build. Waiting gives the staged
// helper time to exit before Windows is asked to remove its executable.
func CleanupAfterStart(staged, target string) {
	if staged == "" || target == "" {
		return
	}
	time.Sleep(12 * time.Second)
	_ = os.Remove(target + ".old")
	_ = os.Remove(staged)
	_ = os.Remove(filepath.Dir(staged))
}

func copyFile(source, destination string) error {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o755)
	if err != nil {
		return err
	}
	ok := false
	defer func() {
		_ = out.Close()
		if !ok {
			_ = os.Remove(destination)
		}
	}()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	if err := out.Sync(); err != nil {
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	ok = true
	return nil
}

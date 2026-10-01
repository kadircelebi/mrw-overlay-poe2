//go:build windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"syscall"
	"time"
	"unsafe"

	ole "github.com/go-ole/go-ole"
	"golang.org/x/sys/windows/registry"
)

const startupTaskID = "MrWOverlayStartup"
const startupRunName = "MrWOverlay"

// Slots follow the Windows SDK IStartupTask and IAsyncOperation ABI.
func startupCall(obj unsafe.Pointer, slot int, args ...uintptr) error {
	vt := *(*unsafe.Pointer)(obj)
	fn := *(*uintptr)(unsafe.Add(vt, uintptr(slot)*unsafe.Sizeof(uintptr(0))))
	hr, _, _ := syscall.SyscallN(fn, append([]uintptr{uintptr(obj)}, args...)...)
	if int32(hr) < 0 {
		return fmt.Errorf("Windows startup: HRESULT 0x%08X", uint32(hr))
	}
	return nil
}

func startupAwait(op unsafe.Pointer, result unsafe.Pointer) error {
	defer startupCall(op, 2)
	iid := ole.NewGUID("{00000036-0000-0000-C000-000000000046}") // IAsyncInfo
	var info unsafe.Pointer
	if err := startupCall(op, 0, uintptr(unsafe.Pointer(iid)), uintptr(unsafe.Pointer(&info))); err != nil {
		return err
	}
	defer startupCall(info, 2)
	deadline := time.Now().Add(10 * time.Second)
	for {
		var status int32
		if err := startupCall(info, 7, uintptr(unsafe.Pointer(&status))); err != nil {
			return err
		}
		if status == 1 {
			return startupCall(op, 8, uintptr(result))
		}
		if status != 0 {
			return fmt.Errorf("Windows startup operation failed (status %d)", status)
		}
		if time.Now().After(deadline) {
			_ = startupCall(info, 9)
			return fmt.Errorf("Windows startup operation timed out")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func startupState(state int32) StartupStatus {
	names := []string{"disabled", "disabledByUser", "enabled", "disabledByPolicy", "enabledByPolicy"}
	if state < 0 || int(state) >= len(names) {
		return StartupStatus{State: "unsupported"}
	}
	return StartupStatus{Enabled: state == 2 || state == 4, CanChange: state == 0 || state == 2, State: names[state]}
}

func startupStatus(change *bool) (StartupStatus, error) {
	if packageAUMID() == "" {
		if storeBuild {
			return StartupStatus{State: "packageRequired"}, nil
		}
		return unpackagedStartup(change)
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	// Dedicated MTA thread; balance even S_FALSE (already initialized).
	hr, _, _ := syscall.NewLazyDLL("combase.dll").NewProc("RoInitialize").Call(1)
	if int32(hr) < 0 {
		return StartupStatus{}, fmt.Errorf("RoInitialize: 0x%08X", uint32(hr))
	}
	defer syscall.NewLazyDLL("combase.dll").NewProc("RoUninitialize").Call()
	factory, err := ole.RoGetActivationFactory("Windows.ApplicationModel.StartupTask", ole.NewGUID("{ee5b60bd-a148-41a7-b26e-e8b88a1e62f8}"))
	if err != nil {
		return StartupStatus{}, err
	}
	defer factory.Release()
	id, err := ole.NewHString(startupTaskID)
	if err != nil {
		return StartupStatus{}, err
	}
	defer ole.DeleteHString(id)
	var op, task unsafe.Pointer
	if err = startupCall(unsafe.Pointer(factory), 7, uintptr(id), uintptr(unsafe.Pointer(&op))); err != nil {
		return StartupStatus{}, err
	}
	if err = startupAwait(op, unsafe.Pointer(&task)); err != nil {
		return StartupStatus{}, err
	}
	defer startupCall(task, 2)
	var state int32
	if err = startupCall(task, 8, uintptr(unsafe.Pointer(&state))); err != nil {
		return StartupStatus{}, err
	}
	status := startupState(state)
	if change != nil && status.CanChange && *change != status.Enabled {
		if *change {
			if err = startupCall(task, 6, uintptr(unsafe.Pointer(&op))); err == nil {
				err = startupAwait(op, unsafe.Pointer(&state))
			}
		} else {
			err = startupCall(task, 7)
		}
		if err != nil {
			return status, err
		}
		if err = startupCall(task, 8, uintptr(unsafe.Pointer(&state))); err != nil {
			return status, err
		}
		status = startupState(state)
	}
	return status, nil
}

// The standalone executable uses an opt-in HKCU Run entry. Packaged builds
// never take this path: Windows owns the manifest startup task instead.
func unpackagedStartup(change *bool) (StartupStatus, error) {
	// Respect Task Manager's opt-out for the standalone Run entry as well.
	approved, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Explorer\StartupApproved\Run`, registry.QUERY_VALUE)
	if err == nil {
		data, _, readErr := approved.GetBinaryValue(startupRunName)
		approved.Close()
		if readErr == nil && len(data) > 0 && data[0]&1 != 0 {
			return startupState(1), nil
		}
	}
	const path = `Software\Microsoft\Windows\CurrentVersion\Run`
	if change != nil {
		key, _, err := registry.CreateKey(registry.CURRENT_USER, path, registry.SET_VALUE)
		if err != nil {
			return StartupStatus{}, err
		}
		defer key.Close()
		if *change {
			exe, e := os.Executable()
			if e != nil {
				return StartupStatus{}, e
			}
			err = key.SetStringValue(startupRunName, `"`+exe+`"`)
		} else {
			err = key.DeleteValue(startupRunName)
			if err == registry.ErrNotExist {
				err = nil
			}
		}
		if err != nil {
			return StartupStatus{}, err
		}
	}
	key, err := registry.OpenKey(registry.CURRENT_USER, path, registry.QUERY_VALUE)
	if err == registry.ErrNotExist {
		return startupState(0), nil
	}
	if err != nil {
		return StartupStatus{}, err
	}
	defer key.Close()
	value, _, err := key.GetStringValue(startupRunName)
	if err == registry.ErrNotExist {
		return startupState(0), nil
	}
	if err != nil {
		return StartupStatus{}, err
	}
	if value != "" {
		return startupState(2), nil
	}
	return startupState(0), nil
}

func (s *AppService) OpenStartupSettings() error {
	return exec.Command("explorer.exe", "ms-settings:startupapps").Start()
}

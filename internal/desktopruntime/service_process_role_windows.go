//go:build windows

package desktopruntime

import (
	"context"
	"errors"
	"fmt"
	"golang.org/x/sys/windows"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

type processHandleFilter func(windows.Handle) (bool, error)

// A generation executable also hosts short-lived controllers and the Tunnel
// supervisor. Only the launch-core role for this root is a service process.
func coreProcessRole(root string) processHandleFilter {
	return func(handle windows.Handle) (bool, error) {
		commandLine, err := processCommandLine(handle)
		if err != nil {
			return false, err
		}
		args, err := windows.DecomposeCommandLine(commandLine)
		if err != nil {
			return false, err
		}
		if len(args) < 4 || args[1] != "service" || args[2] != "launch-core" {
			return false, nil
		}
		var targetRoot string
		if len(args) == 5 && args[3] == "--runtime-root" {
			targetRoot = args[4]
		}
		if len(args) == 4 && strings.HasPrefix(args[3], "--runtime-root=") {
			targetRoot = strings.TrimPrefix(args[3], "--runtime-root=")
		}
		// Product launchers pass an absolute root. A relative target cannot be
		// resolved using this controller's (possibly different) working directory.
		return filepath.IsAbs(targetRoot) && samePath(targetRoot, root), nil
	}
}

var queryProcessInformation = windows.NewLazySystemDLL("ntdll.dll").NewProc("NtQueryInformationProcess")

// Query the bounded command line on a held handle; never use WMI/PID text as
// termination authority. Unsupported/denied queries stay errors, not matches.
func processCommandLine(handle windows.Handle) (string, error) {
	const processCommandLineInformation = 60
	const maximumCommandLineBytes = 64*1024 + 256
	type unicodeString struct {
		Length, MaximumLength uint16
		Buffer                *uint16
	}
	if err := queryProcessInformation.Find(); err != nil {
		return "", fmt.Errorf("process command line query unavailable: %w", err)
	}
	data := make([]byte, maximumCommandLineBytes)
	var returned uint32
	status, _, _ := queryProcessInformation.Call(uintptr(handle), processCommandLineInformation, uintptr(unsafe.Pointer(&data[0])), uintptr(len(data)), uintptr(unsafe.Pointer(&returned)))
	if status != 0 {
		return "", fmt.Errorf("query command line status 0x%x", status)
	}
	header := (*unicodeString)(unsafe.Pointer(&data[0]))
	base := uintptr(unsafe.Pointer(&data[0]))
	start := uintptr(unsafe.Pointer(header.Buffer))
	if returned < uint32(unsafe.Sizeof(*header)) || returned > uint32(len(data)) || header.Length%2 != 0 || header.Length > header.MaximumLength || start < base+unsafe.Sizeof(*header) || start > base+uintptr(returned) || uintptr(header.Length) > base+uintptr(returned)-start {
		return "", errors.New("invalid bounded process command line")
	}
	value := windows.UTF16ToString(unsafe.Slice(header.Buffer, int(header.Length)/2))
	runtime.KeepAlive(data)
	return value, nil
}

// Core selection is separate from the installer's deliberately path-wide
// cleanup. An inaccessible same-name candidate is unknown, never stopped:
// another user's process or a PID recycled after the Core exited cannot be
// proven to be this root's Core and must not fail status or stop.
func selectedProcessIDsAtPath(binaryPath string, excluded map[uint32]struct{}, matches processHandleFilter) ([]uint32, error) {
	target, err := filepath.Abs(binaryPath)
	if err != nil {
		return nil, err
	}
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil, err
	}
	defer windows.CloseHandle(snapshot)
	entry := windows.ProcessEntry32{Size: uint32(unsafe.Sizeof(windows.ProcessEntry32{}))}
	if err := windows.Process32First(snapshot, &entry); err != nil {
		return nil, err
	}
	var selected []uint32
	for {
		_, skip := excluded[entry.ProcessID]
		if !skip && entry.ProcessID != uint32(os.Getpid()) && strings.EqualFold(windows.UTF16ToString(entry.ExeFile[:]), filepath.Base(target)) {
			handle, openErr := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, entry.ProcessID)
			if openErr != nil && !errors.Is(openErr, windows.ERROR_INVALID_PARAMETER) && !errors.Is(openErr, windows.ERROR_ACCESS_DENIED) {
				return nil, fmt.Errorf("inspect Core candidate %d: %w", entry.ProcessID, openErr)
			}
			if openErr == nil {
				matched, matchErr := matchesHeldProcess(handle, target, matches)
				windows.CloseHandle(handle)
				if matchErr != nil {
					return nil, fmt.Errorf("inspect Core candidate %d: %w", entry.ProcessID, matchErr)
				}
				if matched {
					selected = append(selected, entry.ProcessID)
				}
			}
		}
		if err := windows.Process32Next(snapshot, &entry); err != nil {
			if errors.Is(err, syscall.ERROR_NO_MORE_FILES) {
				break
			}
			return nil, err
		}
	}
	return selected, nil
}

func matchesHeldProcess(handle windows.Handle, binaryPath string, matches processHandleFilter) (bool, error) {
	state, err := windows.WaitForSingleObject(handle, 0)
	if err != nil {
		return false, err
	}
	if state == windows.WAIT_OBJECT_0 {
		return false, nil
	}
	if state != uint32(windows.WAIT_TIMEOUT) {
		return false, fmt.Errorf("unknown process wait state %d", state)
	}
	path, err := queryProcessHandlePath(handle)
	if err == nil && !samePath(path, binaryPath) {
		return false, nil
	}
	var selected bool
	if err == nil {
		selected, err = matches(handle)
	}
	if err != nil {
		// A process may exit while its command line is queried. The same held
		// handle, not a recycled PID, must prove that normal race completed.
		if state, waitErr := windows.WaitForSingleObject(handle, 0); waitErr == nil && state == windows.WAIT_OBJECT_0 {
			return false, nil
		}
		return false, err
	}
	return selected, nil
}

func terminateProcessIDsMatching(ctx context.Context, binaryPath string, ids []uint32, matches processHandleFilter) error {
	for _, pid := range ids {
		if err := ctx.Err(); err != nil {
			return err
		}
		handle, err := windows.OpenProcess(windows.PROCESS_TERMINATE|windows.SYNCHRONIZE|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
		if errors.Is(err, windows.ERROR_INVALID_PARAMETER) {
			continue
		}
		if err != nil {
			return fmt.Errorf("open Core target %d: %w", pid, err)
		}
		err = func() error {
			defer windows.CloseHandle(handle)
			selected, err := matchesHeldProcess(handle, binaryPath, matches)
			if err != nil || !selected {
				return err
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			terminateErr := windows.TerminateProcess(handle, 0)
			for {
				state, waitErr := windows.WaitForSingleObject(handle, 50)
				if waitErr != nil {
					return waitErr
				}
				if state == windows.WAIT_OBJECT_0 {
					return nil
				}
				if state != uint32(windows.WAIT_TIMEOUT) {
					return fmt.Errorf("unknown Core exit state %d", state)
				}
				if terminateErr != nil {
					return terminateErr
				}
				if err := ctx.Err(); err != nil {
					return err
				}
			}
		}()
		if err != nil {
			return fmt.Errorf("stop Core target %d: %w", pid, err)
		}
	}
	return nil
}

func waitSelectedBinaryStopped(ctx context.Context, binaryPath string, excluded map[uint32]struct{}, timeout time.Duration, matches processHandleFilter) (bool, error) {
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		if err := ctx.Err(); err != nil {
			return false, err
		}
		ids, err := selectedProcessIDsAtPath(binaryPath, excluded, matches)
		if err != nil {
			return false, err
		}
		if len(ids) == 0 {
			return true, nil
		}
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		case <-deadline.C:
			return false, nil
		case <-ticker.C:
		}
	}
}

func stopSelectedBinaryProcesses(ctx context.Context, binaryPath string, excluded map[uint32]struct{}, timeout time.Duration, matches processHandleFilter) error {
	bounded, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	if err := bounded.Err(); err != nil {
		return err
	}
	ids, err := selectedProcessIDsAtPath(binaryPath, excluded, matches)
	if err != nil {
		return err
	}
	if err := terminateProcessIDsMatching(bounded, binaryPath, ids, matches); err != nil {
		return err
	}
	stopped, err := waitSelectedBinaryStopped(bounded, binaryPath, excluded, timeout, matches)
	if err != nil {
		return err
	}
	if !stopped {
		return fmt.Errorf("Core processes did not exit within %s: %s", timeout, binaryPath)
	}
	return nil
}

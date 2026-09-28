//go:build windows

package desktopruntime

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"golang.org/x/sys/windows"
	"net"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"syscall"
	"testing"
	"time"
	"unsafe"
)

type coreRoleFixture struct {
	command *exec.Cmd
	handle  windows.Handle
	done    chan error
}

func buildCoreRoleFixture(t *testing.T) string {
	t.Helper()
	binary := filepath.Join(t.TempDir(), "core-role-fixture.exe")
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "go", "build", "-o", binary, "./testdata/core-role")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("build isolated role fixture: %v\n%s", err, output)
	}
	return binary
}

func startCoreRoleFixture(t *testing.T, binary string, args ...string) coreRoleFixture {
	t.Helper()
	command := exec.Command(binary, args...)
	input, err := command.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	output, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err = command.Start(); err != nil {
		t.Fatal(err)
	}
	handle, err := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(command.Process.Pid))
	if err != nil {
		command.Process.Kill()
		command.Wait()
		t.Fatal(err)
	}
	value := coreRoleFixture{command, handle, make(chan error, 1)}
	handshake := make(chan error, 1)
	go func() {
		line, err := bufio.NewReader(output).ReadString('\n')
		if err == nil && line != "fixture-ready\n" {
			err = fmt.Errorf("invalid handshake %q", line)
		}
		handshake <- err
	}()
	select {
	case err = <-handshake:
	case <-time.After(5 * time.Second):
		err = fmt.Errorf("fixture handshake timeout")
	}
	if err != nil {
		input.Close()
		command.Process.Kill()
		command.Wait()
		windows.CloseHandle(handle)
		t.Fatal(err)
	}
	go func() { value.done <- command.Wait() }()
	t.Cleanup(func() {
		input.Close()
		command.Process.Kill()
		select {
		case <-value.done:
		case <-time.After(5 * time.Second):
			t.Error("fixture cleanup timed out")
		}
		windows.CloseHandle(handle)
	})
	return value
}

func assertCoreRoleFixtureAlive(t *testing.T, value coreRoleFixture, want bool) {
	t.Helper()
	state, err := windows.WaitForSingleObject(value.handle, 0)
	if err != nil {
		t.Fatal(err)
	}
	if alive := state == uint32(windows.WAIT_TIMEOUT); alive != want {
		t.Errorf("pid %d alive=%t want=%t", value.command.Process.Pid, alive, want)
	}
}

func TestCoreProcessRoleUsesCanonicalActionAndRoot(t *testing.T) {
	binary := buildCoreRoleFixture(t)
	root := t.TempDir()
	for _, tc := range []struct {
		name string
		args []string
		want bool
	}{
		{"Core", []string{"service", "launch-core", "--runtime-root", root}, true},
		{"flag equals", []string{"service", "launch-core", "--runtime-root=" + root}, true},
		{"different runtime", []string{"service", "launch-core", "--runtime-root", t.TempDir()}, false},
		{"sibling controller", []string{"tunnel", "stop", "--runtime-root", root}, false},
		{"supervisor", []string{"tunnel", "launch", "--runtime-root", root}, false},
		{"extra arguments", []string{"service", "launch-core", "--runtime-root", root, "extra"}, false},
		{"relative root", []string{"service", "launch-core", "--runtime-root", "."}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := startCoreRoleFixture(t, binary, tc.args...)
			got, err := coreProcessRole(root)(p.handle)
			if err != nil || got != tc.want {
				t.Fatalf("role=%t want=%t err=%v", got, tc.want, err)
			}
		})
	}
	if matched, err := coreProcessRole(root)(windows.Handle(0)); err == nil || matched {
		t.Fatalf("invalid handle matched=%t err=%v", matched, err)
	}
}

func TestCoreStopPreservesControllersSupervisorsAndOtherRoots(t *testing.T) {
	binary := buildCoreRoleFixture(t)
	root := t.TempDir()
	core := startCoreRoleFixture(t, binary, "service", "launch-core", "--runtime-root", root)
	controller := startCoreRoleFixture(t, binary, "tunnel", "stop", "--runtime-root", root)
	supervisor := startCoreRoleFixture(t, binary, "tunnel", "launch", "--runtime-root", root)
	other := startCoreRoleFixture(t, binary, "service", "launch-core", "--runtime-root", t.TempDir())
	manifest := Manifest{InstallRoot: root, AgentDockBinary: binary, PrivilegeMode: "standard"}
	for range 2 {
		ctx, cancel := context.WithTimeout(t.Context(), 8*time.Second)
		err := stopCore(ctx, manifest, root)
		cancel()
		if err != nil {
			t.Fatal(err)
		}
		assertCoreRoleFixtureAlive(t, core, false)
		for _, p := range []coreRoleFixture{controller, supervisor, other} {
			assertCoreRoleFixtureAlive(t, p, true)
		}
	}
}

// CI 36364901575 failed Core stop on a same-name PID that could not be opened.
// Another user's process or a PID recycled while the Core exits is never this
// root's Core; it must neither be stopped nor fail status or stop.
func TestCoreSelectionSkipsInaccessibleSameNameProcess(t *testing.T) {
	binary := buildCoreRoleFixture(t)
	root := t.TempDir()
	// An empty DACL denies the same user every process right; only exec.Cmd's
	// creator handle can still kill and wait for the fixture.
	descriptor, err := windows.SecurityDescriptorFromString("D:")
	if err != nil {
		t.Fatal(err)
	}
	other := exec.Command(binary, "service", "launch-core", "--runtime-root", t.TempDir())
	other.SysProcAttr = &syscall.SysProcAttr{ProcessAttributes: &syscall.SecurityAttributes{
		Length:             uint32(unsafe.Sizeof(syscall.SecurityAttributes{})),
		SecurityDescriptor: uintptr(unsafe.Pointer(descriptor)),
	}}
	input, err := other.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err = other.Start(); err != nil {
		t.Fatal(err)
	}
	runtime.KeepAlive(descriptor)
	done := make(chan error, 1)
	go func() { done <- other.Wait() }()
	t.Cleanup(func() {
		input.Close()
		other.Process.Kill()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("inaccessible fixture cleanup timed out")
		}
	})
	if handle, err := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(other.Process.Pid)); err == nil {
		windows.CloseHandle(handle)
		t.Skip("this token opens any process (SeDebugPrivilege enabled)")
	} else if !errors.Is(err, windows.ERROR_ACCESS_DENIED) {
		t.Fatalf("inaccessible fixture open: %v", err)
	}

	core := startCoreRoleFixture(t, binary, "service", "launch-core", "--runtime-root", root)
	ids, err := selectedProcessIDsAtPath(binary, nil, coreProcessRole(root))
	if err != nil || len(ids) != 1 || ids[0] != uint32(core.command.Process.Pid) {
		t.Fatalf("selected=%v err=%v", ids, err)
	}
	manifest := Manifest{InstallRoot: root, AgentDockBinary: binary, PrivilegeMode: "standard"}
	ctx, cancel := context.WithTimeout(t.Context(), 8*time.Second)
	defer cancel()
	if err := stopCore(ctx, manifest, root); err != nil {
		t.Fatal(err)
	}
	assertCoreRoleFixtureAlive(t, core, false)
	select {
	case err := <-done:
		done <- err
		t.Fatalf("inaccessible process was stopped: %v", err)
	default:
	}
}

func TestCoreTerminationRechecksRoleOnActualHandle(t *testing.T) {
	binary := buildCoreRoleFixture(t)
	root := t.TempDir()
	controller := startCoreRoleFixture(t, binary, "tunnel", "stop", "--runtime-root", root)
	err := terminateProcessIDsMatching(t.Context(), binary, []uint32{uint32(controller.command.Process.Pid)}, coreProcessRole(root))
	if err != nil {
		t.Fatal(err)
	}
	assertCoreRoleFixtureAlive(t, controller, true)
}

func TestServiceStatusDoesNotCountControllerAndKeepsUnhealthyCoreRunning(t *testing.T) {
	binary := buildCoreRoleFixture(t)
	root := t.TempDir()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "not ready", 503) }))
	defer server.Close()
	_, portText, err := net.SplitHostPort(server.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatal(err)
	}
	manifest := Manifest{SchemaVersion: 1, InstallRoot: root, AgentDockBinary: binary, PrivilegeMode: "standard", TunnelMode: "none", Host: "127.0.0.1", Port: port, LocalMCPURL: fmt.Sprintf("http://127.0.0.1:%d/mcp", port), StartupValueName: "AgentDockRoleTest-" + filepath.Base(root)}
	if err := Save(filepath.Join(root, "runtime.json"), manifest); err != nil {
		t.Fatal(err)
	}
	startCoreRoleFixture(t, binary, "tunnel", "stop", "--runtime-root", root)
	status, err := platformServiceStatus(t.Context(), root)
	if err != nil || status.Running || status.Healthy {
		t.Fatalf("controller status=%+v err=%v", status, err)
	}
	startCoreRoleFixture(t, binary, "service", "launch-core", "--runtime-root", root)
	status, err = platformServiceStatus(t.Context(), root)
	if err != nil || !status.Running || status.Healthy {
		t.Fatalf("unhealthy Core status=%+v err=%v", status, err)
	}
}

func TestCoreTerminationRefusesCancelledAndUnprovenTargets(t *testing.T) {
	binary := buildCoreRoleFixture(t)
	root := t.TempDir()
	core := startCoreRoleFixture(t, binary, "service", "launch-core", "--runtime-root", root)
	ids := []uint32{uint32(core.command.Process.Pid)}
	cancelled, cancel := context.WithCancel(t.Context())
	cancel()
	if err := terminateProcessIDsMatching(cancelled, binary, ids, coreProcessRole(root)); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled target: %v", err)
	}
	assertCoreRoleFixtureAlive(t, core, true)
	if err := terminateProcessIDsMatching(t.Context(), binary+".different", ids, coreProcessRole(root)); err != nil {
		t.Fatal(err)
	}
	assertCoreRoleFixtureAlive(t, core, true)
	denied := errors.New("role unavailable")
	if err := terminateProcessIDsMatching(t.Context(), binary, ids, func(windows.Handle) (bool, error) { return false, denied }); !errors.Is(err, denied) {
		t.Fatalf("unknown role became success: %v", err)
	}
	assertCoreRoleFixtureAlive(t, core, true)
	if matched, err := matchesHeldProcess(0, binary, coreProcessRole(root)); err == nil || matched {
		t.Fatalf("unavailable handle accepted: %t %v", matched, err)
	}
}

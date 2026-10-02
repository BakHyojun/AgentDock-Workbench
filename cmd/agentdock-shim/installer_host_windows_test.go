//go:build windows

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/uvwt/agentdock/internal/desktopruntime"
	"github.com/uvwt/agentdock/internal/executioncompat"
	"github.com/uvwt/agentdock/internal/fs/processlock"
	processctl "github.com/uvwt/agentdock/internal/process"
	"github.com/uvwt/agentdock/internal/updateengine"
)

func TestInstallerHostEntryRequiresExactTrayTaskAndRoot(t *testing.T) {
	root := t.TempDir()
	for _, tc := range []struct {
		name string
		args []string
		want bool
	}{
		{"task", []string{"--run-core-task", "--runtime-root", root}, true},
		{"case", []string{"--RUN-CORE-TASK", "--RUNTIME-ROOT", root}, true},
		{"different root", []string{"--run-core-task", "--runtime-root", t.TempDir()}, false},
		{"relative root", []string{"--run-core-task", "--runtime-root", "."}, false},
		{"missing root", []string{"--run-core-task"}, false},
		{"extra UI flag", []string{"--run-core-task", "--runtime-root", root, "--background"}, false},
		{"flag in management", []string{"--version", "--run-core-task", "--runtime-root", root}, false},
		{"background", []string{"--background"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := installerHostEntry(root, true, tc.args); got != tc.want {
				t.Fatalf("host entry=%v want=%v", got, tc.want)
			}
		})
	}
	if installerHostEntry(root, false, []string{"--run-core-task", "--runtime-root", root}) {
		t.Fatal("Core shim accepted a tray-only host contract")
	}
}

// Real production shims exercise the entire admission/dispatch path. Only the
// generation children are finite fixtures: no Setup, scheduler, Core server,
// production configuration or credentials participate in this test.
func TestInstallerTrialScheduledTaskEntryThroughRealShim(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()
	shim := filepath.Join(t.TempDir(), "tray-shim.exe")
	build := exec.CommandContext(ctx, filepath.Join(runtime.GOROOT(), "bin", "go.exe"),
		"build", "-p", "2", "-ldflags", "-H=windowsgui", "-o", shim, "./cmd/agentdock-shim")
	build.Dir = filepath.Join("..", "..")
	processctl.Configure(build)
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build production shim: %v\n%s", err, output)
	}
	shimBytes, err := os.ReadFile(shim)
	if err != nil {
		t.Fatal(err)
	}
	testExe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	fixtureBytes, err := os.ReadFile(testExe)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, entry, phase                      string
		committed, unowned, mismatch, wrongRoot bool
		wantReached                             bool
	}{
		{name: "existing task start", entry: "--run-core-task", phase: "start", wantReached: true},
		{name: "existing task health", entry: "--run-core-task", phase: "health", wantReached: true},
		{name: "native host", entry: taskCoreHostFlag, phase: "start", wantReached: true},
		{name: "committed task", entry: "--run-core-task", committed: true, wantReached: true},
		{name: "abandoned owner", entry: "--run-core-task", phase: "start", unowned: true},
		{name: "wrong id", entry: "--run-core-task", phase: "start", mismatch: true},
		{name: "commit phase", entry: "--run-core-task", phase: "commit"},
		{name: "different task root", entry: "--run-core-task", phase: "start", wrongRoot: true},
		{name: "different native root", entry: taskCoreHostFlag, phase: "start", wrongRoot: true},
		{name: "background UI", entry: "--background", phase: "start"},
		{name: "management with task flag", entry: "--version", phase: "start"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			layout, err := updateengine.NewWindowsLayout(root)
			if err != nil {
				t.Fatal(err)
			}
			version := "v1.1.8103"
			for _, entry := range []struct {
				path string
				data []byte
			}{
				{layout.TrayShim(), shimBytes}, {layout.CoreShim(), shimBytes},
				{layout.GenerationTray(version), fixtureBytes}, {layout.GenerationCore(version), fixtureBytes},
			} {
				if err := os.MkdirAll(filepath.Dir(entry.path), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(entry.path, entry.data, 0700); err != nil {
					t.Fatal(err)
				}
			}
			home := filepath.Join(root, "home")
			policy := filepath.Join(home, "execution", "permissions", "policy.json")
			if err := os.MkdirAll(filepath.Dir(policy), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(policy, []byte(`{"schema_version":1}`), 0600); err != nil {
				t.Fatal(err)
			}
			manifest := desktopruntime.Manifest{SchemaVersion: desktopruntime.SchemaVersion,
				InstallRoot: root, AgentDockHome: home, AgentDockDefaultDir: root,
				AgentDockBinary: layout.CoreShim(), TrayBinary: layout.TrayShim(),
				PrivilegeMode: "elevated", AgentDockTaskName: "fixture-never-registered",
				Host: "127.0.0.1", Port: 8765, LocalMCPURL: "http://127.0.0.1:8765/mcp", TunnelMode: "none", InstallChannel: "setup"}
			if err := desktopruntime.Save(filepath.Join(root, "runtime.json"), manifest); err != nil {
				t.Fatal(err)
			}
			store, err := updateengine.NewStore(root)
			if err != nil {
				t.Fatal(err)
			}
			state := updateengine.StateTrial
			if tc.committed {
				state = updateengine.StateCommitted
			}
			active := updateengine.ActiveVersion{SchemaVersion: 1, ActiveVersion: version,
				State: state, TransactionID: "task-fixture", UpdatedAt: time.Now().UTC()}
			if err := store.WriteActive(active); err != nil {
				t.Fatal(err)
			}
			install := filepath.Join(root, "install")
			if err := os.MkdirAll(install, 0700); err != nil {
				t.Fatal(err)
			}
			id := active.TransactionID
			if tc.mismatch {
				id = "wrong-owner"
			}
			transaction, err := json.Marshal(map[string]any{"schema_version": 1, "action": "install",
				"transaction_id": id, "state": "trial", "phase": tc.phase, "target_version": version,
				"install_root": root, "runtime_root": root})
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(install, "transaction.json"), transaction, 0600); err != nil {
				t.Fatal(err)
			}
			if !tc.unowned {
				lock, err := processlock.Acquire(ctx, filepath.Join(install, "transaction.lock"))
				if err != nil {
					t.Fatal(err)
				}
				defer lock.Release()
			}
			requestedRoot := root
			if tc.wrongRoot {
				requestedRoot = t.TempDir()
			}
			args := []string{tc.entry, "--runtime-root", requestedRoot}
			if tc.entry == "--version" {
				args = append([]string{"--version", "--run-core-task"}, args[1:]...)
			}
			command := exec.CommandContext(ctx, layout.TrayShim(), args...)
			command.Env = append(os.Environ(), "AGENTDOCK_SHIM_GENERATION_FIXTURE="+root)
			processctl.Configure(command)
			output, runErr := command.CombinedOutput()
			var exitErr *exec.ExitError
			if !errors.As(runErr, &exitErr) {
				t.Fatalf("expected finite child exit: %v\n%s", runErr, output)
			}
			wantExit := 1
			if tc.wantReached {
				wantExit = 7
			}
			if exitErr.ExitCode() != wantExit {
				t.Fatalf("exit=%d want=%d\n%s", exitErr.ExitCode(), wantExit, output)
			}
			_, markerErr := os.Stat(filepath.Join(root, "fixture-core-reached"))
			if (markerErr == nil) != tc.wantReached {
				t.Fatalf("Core reached=%v want=%v\n%s", markerErr == nil, tc.wantReached, output)
			}
			if !tc.wantReached && !strings.Contains(string(output), "trial") && !strings.Contains(string(output), "does not match stable entry root") {
				t.Fatalf("unexpected rejection: %s", output)
			}
			if got, err := store.ReadActive(); err != nil || got.State != state || got.TransactionID != active.TransactionID {
				t.Fatalf("host modified the pointer: %+v %v", got, err)
			}
		})
	}
}

// Copied test binaries stand in for generation children only. The parent shims
// are separately built production executables. This fixture never starts Core.
func init() {
	root := os.Getenv("AGENTDOCK_SHIM_GENERATION_FIXTURE")
	if root == "" {
		return
	}
	executable, err := os.Executable()
	if err != nil {
		os.Exit(90)
	}
	if !sameWindowsPath(filepath.Dir(filepath.Dir(filepath.Dir(executable))), root) {
		os.Exit(91)
	}
	if len(os.Args) == 3 && os.Args[1] == "version" && os.Args[2] == "--json" {
		_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"version": "1.1.8103", "execution_policy_version": executioncompat.PolicyVersion})
		os.Exit(0)
	}
	if filepath.Base(executable) == updateengine.GenerationCoreName {
		if len(os.Args) != 5 || os.Args[1] != "service" || os.Args[2] != "launch-core" || os.Args[3] != "--runtime-root" || !sameWindowsPath(os.Args[4], root) {
			os.Exit(92)
		}
		if err := os.WriteFile(filepath.Join(root, "fixture-core-reached"), []byte("finite child"), 0600); err != nil {
			os.Exit(93)
		}
		time.Sleep(150 * time.Millisecond) // Leave time for the real parent's Job attachment.
		os.Exit(7)
	}
	command := exec.Command(filepath.Join(root, "bin", updateengine.StableCoreShimName), "service", "launch-core", "--runtime-root", root)
	processctl.Configure(command)
	output, runErr := command.CombinedOutput()
	if runErr != nil {
		fmt.Fprint(os.Stderr, string(output))
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			os.Exit(exitErr.ExitCode())
		}
		os.Exit(94)
	}
	os.Exit(0)
}

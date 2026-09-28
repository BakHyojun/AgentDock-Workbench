package scripts

import (
	"os"
	"strings"
	"testing"
)

func TestWindowsTaskRollbackRetainsRuntimeOwner(t *testing.T) {
	data, err := os.ReadFile("../install/install.ps1")
	if err != nil {
		t.Fatal(err)
	}
	source := strings.ReplaceAll(string(data), "\r\n", "\n")
	start := strings.Index(source, "$restoreTaskActionResult = Start-ElevatedAgentDockTaskAction")
	if start < 0 {
		t.Fatal("task rollback call missing")
	}
	end := strings.Index(source[start:], "if (-not $restoreTaskActionResult.Started)")
	if end < 0 {
		t.Fatal("task rollback result must be checked")
	}
	call := source[start : start+end]
	for _, want := range []string{"-Action restore", "-RuntimeRoot $runtimeDir", "-TaskUser $taskUser", "-BackupDirectory $taskBackupDirectory"} {
		if !strings.Contains(call, want) {
			t.Errorf("rollback lost original owner binding: %s", want)
		}
	}
}

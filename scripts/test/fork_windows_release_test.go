package scripts

import (
	"os"
	"strings"
	"testing"
)

// Setup refuses an x64 payload without the pinned rg component because the
// installer engine accepts its absence for legacy payloads.
func TestWindowsSetupRequiresBundledRgAfterExtraction(t *testing.T) {
	data, err := os.ReadFile("../install/install.ps1")
	if err != nil {
		t.Fatal(err)
	}
	source := string(data)
	extract := strings.Index(source, "Expand-AgentDockReleaseArchive -ArchivePath $archivePath -DestinationPath $extractDir")
	check := strings.Index(source, "Assert-AgentDockBundledRgPayload -ExtractDir $extractDir -Architecture $architecture")
	firstUse := strings.Index(source, "& $sourceBinary ")
	if extract < 0 || check < extract || firstUse < check {
		t.Fatalf("Setup must check the rg component after extraction and before using the new Core: %d %d %d", extract, check, firstUse)
	}
}

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

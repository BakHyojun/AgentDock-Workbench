package scripts

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWindowsForkDistributionTargets(t *testing.T) {
	const repository = "eerraa/AgentDock-Workbench"
	for _, item := range []struct {
		path    string
		markers []string
	}{
		{"internal/selfupdate/update.go", []string{"https://api.github.com/repos/" + repository + "/releases/latest"}},
		{"scripts/install/install.ps1", []string{"https://github.com/" + repository + "/releases/latest/download", "https://github.com/" + repository + "/releases/download/$normalizedVersion"}},
		{"packaging/windows/AgentDock.iss", []string{"AppPublisherURL=https://github.com/" + repository, "AppSupportURL=https://github.com/" + repository + "/issues", "AppUpdatesURL=https://github.com/" + repository + "/releases"}},
	} {
		t.Run(item.path, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join("..", "..", filepath.FromSlash(item.path)))
			if err != nil {
				t.Fatal(err)
			}
			for _, marker := range item.markers {
				if !strings.Contains(string(data), marker) {
					t.Errorf("fork distribution destination missing: %s", marker)
				}
			}
			// Upstream renamed its repository; reject every upstream owner address.
			if strings.Contains(strings.ToLower(string(data)), "a-m-o-r-f-a-t-i/") {
				t.Error("Windows fork can still resolve an upstream update payload")
			}
		})
	}
}

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

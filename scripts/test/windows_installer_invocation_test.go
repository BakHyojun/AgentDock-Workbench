package scripts

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Only parsed pure functions and the actual rollback call are exercised. The
// process launcher is a test double; installer top-level code is never run.
func TestWindowsInstallerRollbackAndDownloadInvocation(t *testing.T) {
	pwsh, err := exec.LookPath("pwsh")
	if err != nil {
		t.Skip("PowerShell unavailable for isolated invocation contract")
	}
	source, err := os.ReadFile("../install/install.ps1")
	if err != nil {
		t.Fatal(err)
	}
	text := strings.ReplaceAll(string(source), "\r\n", "\n")
	start := strings.Index(text, "$restoreTaskActionResult = Start-ElevatedAgentDockTaskAction")
	if start < 0 {
		t.Fatal("missing actual rollback invocation")
	}
	end := strings.Index(text[start:], "if (-not $restoreTaskActionResult.Started)")
	if end < 0 {
		t.Fatal("missing rollback result boundary")
	}
	fixture := t.TempDir()
	installer := filepath.Join(fixture, "source.ps1")
	if err := os.WriteFile(installer, source, 0600); err != nil {
		t.Fatal(err)
	}
	script := `param([string]$Source)
$ErrorActionPreference='Stop'
$tokens=$null; $errors=$null
$ast=[Management.Automation.Language.Parser]::ParseFile($Source,[ref]$tokens,[ref]$errors)
if($errors.Count){throw 'Installer parse failed'}
foreach($name in @('Start-ElevatedAgentDockTaskAction','Get-ReleaseBaseUrl')) {
    $functions=@($ast.FindAll({param($node) $node -is [Management.Automation.Language.FunctionDefinitionAst] -and $node.Name -eq $name},$false))
    if($functions.Count -ne 1){throw 'Unexpected function inventory'}
    . ([scriptblock]::Create($functions[0].Extent.Text))
}
$script:launches=0
function Start-Process {
    param($FilePath,$ArgumentList,$Verb,$WindowStyle,[switch]$Wait,[switch]$PassThru)
    $script:launches++
    $script:actual=@{path=$FilePath;arguments=$ArgumentList;verb=$Verb;wait=[bool]$Wait}
    [pscustomobject]@{ExitCode=0}
}
$runtimeDir='C:\isolated 한글\runtime root'
$taskBackupDirectory='C:\isolated 한글\original backup'
$sourceTrayBinary='C:\isolated 한글\verified payload\agentdock-tray.exe'
$taskUser=[pscustomobject]@{Sid='S-1-5-21-1-2-3-1000';Name='Fixture\Owner'}
` + text[start:start+end] + `
if($script:launches -ne 1 -or -not $restoreTaskActionResult.Started -or -not $restoreTaskActionResult.Succeeded){throw 'Rollback invocation was not preserved'}
foreach($part in @('--task-admin restore','--runtime-root "'+$runtimeDir+'"','--backup-directory "'+$taskBackupDirectory+'"','--user-sid "'+$taskUser.Sid+'"')) {
    if(-not $script:actual.arguments.Contains($part)){throw ('Missing exact rollback argument: '+$part)}
}
if($script:actual.path -ne $sourceTrayBinary -or $script:actual.verb -ne 'RunAs' -or -not $script:actual.wait){throw 'Native current-user administrative launcher changed'}
$env:AGENTDOCK_RELEASE_BASE_URL=$null
if((Get-ReleaseBaseUrl latest) -ne 'https://github.com/eerraa/AgentDock-Workbench/releases/latest/download'){throw 'Wrong latest destination'}
foreach($version in @('1.2.3','v1.2.3')) {
    if((Get-ReleaseBaseUrl $version) -ne 'https://github.com/eerraa/AgentDock-Workbench/releases/download/v1.2.3'){throw 'Wrong versioned destination'}
}
$env:AGENTDOCK_RELEASE_BASE_URL='https://fixture.invalid/explicit/'
if((Get-ReleaseBaseUrl latest) -ne 'https://fixture.invalid/explicit'){throw 'Explicit caller override changed'}
Write-Output 'PASS: exact rollback owner and fork URL resolution; no installer or process launched.'
`
	path := filepath.Join(fixture, "invocation.ps1")
	if err := os.WriteFile(path, []byte(script), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, pwsh, "-NoLogo", "-NoProfile", "-NonInteractive", "-File", path, "-Source", installer)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("isolated invocation failed: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), "PASS: exact rollback owner") {
		t.Fatalf("no contract receipt: %s", output)
	}
}

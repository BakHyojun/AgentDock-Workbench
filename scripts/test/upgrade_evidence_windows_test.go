package scripts

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Load only the real script's pure reporting functions. A PowerShell function
// replaces the installer process; no Setup, registry, credentials or service
// operation from the script's top-level body is invoked.
func TestUpgradeEvidencePreservesFailureIdentityAndRejectsUnreachedFault(t *testing.T) {
	shell, err := exec.LookPath("powershell.exe")
	if err != nil {
		t.Fatal("Windows PowerShell is required for this Windows contract:", err)
	}
	source, err := filepath.Abs("test-windows-upgrade-isolated.ps1")
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	wrapper := filepath.Join(root, "evidence-contract.ps1")
	if err := os.WriteFile(wrapper, append([]byte{0xef, 0xbb, 0xbf}, []byte(upgradeEvidenceProbe)...), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, shell, "-NoLogo", "-NoProfile", "-NonInteractive", "-File", wrapper, "-Source", source, "-Root", root)
	// A PowerShell 7 parent exports its own module paths. Resolve only the
	// built-in Windows PowerShell modules for this isolated 5.1 child.
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(strings.ToLower(entry), "psmodulepath=") {
			command.Env = append(command.Env, entry)
		}
	}
	command.Env = append(command.Env, "PSModulePath="+filepath.Join(filepath.Dir(shell), "Modules"))
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("isolated evidence contract: %v\n%s", err, output)
	}
	var result struct {
		Assertions        int  `json:"assertions"`
		InstallerExecuted bool `json:"installer_executed"`
		MockCalls         int  `json:"mock_calls"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(output))), &result); err != nil {
		t.Fatalf("invalid contract report: %v\n%s", err, output)
	}
	if result.Assertions < 25 || result.InstallerExecuted || result.MockCalls != 5 {
		t.Fatalf("incomplete evidence contract: %+v", result)
	}
	t.Logf("%d assertions; %d inert installer outcomes; no installer or user settings invoked", result.Assertions, result.MockCalls)
}

func TestUpgradeEvidenceRemainsAttachedToFailedAndSuccessfulCIArtifacts(t *testing.T) {
	workflow, err := os.ReadFile("../../.github/workflows/windows-package.yml")
	if err != nil {
		t.Fatal(err)
	}
	text := string(workflow)
	if strings.Count(text, "dist/windows-package/upgrade-evidence/*") != 2 {
		t.Fatal("redacted upgrade diagnostics must be preserved in both successful and failed CI artifacts")
	}
	script, err := os.ReadFile("test-windows-upgrade-isolated.ps1")
	if err != nil {
		t.Fatal(err)
	}
	for _, retained := range []string{
		"Run-Installer $BaselineArchive $BaselineChannel $BaselineInstallerPath 'baseline'",
		"$baselineVersion=Read-Health ''", "$transaction.state -ne 'rolled_back'",
		"Run-Installer $Archive 'setup' $InstallerPath 'upgrade'", "Assert-Preserved",
		"Get-ScheduledTask -TaskName 'AgentDock'", "finally{",
		"if($exportFailed -and $currentPhase -eq 'completed'){throw",
	} {
		if !strings.Contains(string(script), retained) {
			t.Errorf("evidence changes removed an existing acceptance or cleanup boundary: %s", retained)
		}
	}
}

const upgradeEvidenceProbe = `param([string]$Source,[string]$Root)
$ErrorActionPreference='Stop'
Set-StrictMode -Version Latest
$tokens=$null;$errors=$null
$ast=[Management.Automation.Language.Parser]::ParseFile($Source,[ref]$tokens,[ref]$errors)
if($errors.Count){throw 'Source script does not parse'}
foreach($name in @('Save-Report','Protect-InstallerDiagnostic','Export-UpgradeDiagnostics','Run-Installer')) {
    $functions=@($ast.FindAll({param($node) $node -is [Management.Automation.Language.FunctionDefinitionAst] -and $node.Name -ceq $name},$false))
    if($functions.Count -ne 1){throw "Expected one source function $name"}
    . ([scriptblock]::Create($functions[0].Extent.Text))
}
$assertions=0
function Check([bool]$Value,[string]$Message){$script:assertions++;if(-not $Value){throw $Message}}
function Refused([scriptblock]$Action,[string]$Message){
    $refused=$false
    try {& $Action} catch {if(-not $_.Exception.Message.Contains($Message)){throw};$refused=$true}
    Check $refused ('Expected rejection: '+$Message)
}
$utf8=[Text.UTF8Encoding]::new($false)
$testRoot=Join-Path $Root 'synthetic';$EvidenceDirectory=Join-Path $Root 'export'
[IO.Directory]::CreateDirectory($testRoot)|Out-Null
$ReportPath=Join-Path $Root 'report.json'
$allPassed=$false;$baselineVersion='';$targetVersion='';$SeedStaleEnvironment=$false;$currentPhase='preflight'
$installerRuns=New-Object Collections.Generic.List[object];$cases=New-Object Collections.Generic.List[object];$diagnosticEvidence=@()
$CloudflaredBinary=Join-Path $Root 'inert-payload.bin';[IO.File]::WriteAllText($CloudflaredBinary,'fixture payload',$utf8)
$fixtureScript=Join-Path $Root 'inert-script.ps1';[IO.File]::WriteAllText($fixtureScript,'# never executed',$utf8)
$installDir=Join-Path $testRoot 'bin';$names=@('fixture-core','fixture-tray','fixture-tunnel');$port=0
$mockExit=0;$mockText='synthetic outcome';$mockCalls=0;$mutate=''
function powershell.exe {
    $script:mockCalls++
    Write-Output $script:mockText
    if($script:mutate){[IO.File]::WriteAllText($script:mutate,'changed input',$script:utf8)}
    $global:LASTEXITCODE=$script:mockExit
}
Run-Installer $CloudflaredBinary 'script' $fixtureScript 'baseline'
$receipt=Get-Content $ReportPath -Raw|ConvertFrom-Json
Check ($receipt.phase -eq 'baseline' -and -not $receipt.passed -and $receipt.baseline -eq '') 'Process success cannot claim baseline health'
Check ($receipt.installer_runs.Count -eq 1 -and $receipt.installer_runs[0].exit_code -eq 0) 'Baseline process outcome missing'
Check ($receipt.installer_runs[0].inputs_unchanged) 'Stable inputs not recorded'
Check ($receipt.installer_runs[0].script_sha256 -ceq (Get-FileHash $fixtureScript).Hash.ToLowerInvariant()) 'Script provenance is not actual bytes'
Check ($receipt.installer_runs[0].archive_sha256 -ceq (Get-FileHash $CloudflaredBinary).Hash.ToLowerInvariant()) 'Archive provenance is not actual bytes'
$mockExit=1;$mockText='unrelated baseline setup error'
Refused {Run-Installer $CloudflaredBinary 'setup' $fixtureScript 'injected-rollback' $true} 'failed before reaching the required injected trial boundary'
$receipt=Get-Content $ReportPath -Raw|ConvertFrom-Json
Check ($receipt.phase -eq 'injected-rollback' -and -not $receipt.passed) 'Failure phase was not retained'
Check ($receipt.installer_runs[1].exit_code -eq 1 -and $receipt.installer_runs[1].expected_failure) 'Failed injection attempt missing'
$mockText='isolated-test: fail after Engine trial, before commit'
Run-Installer $CloudflaredBinary 'setup' $fixtureScript 'injected-rollback' $true
Check ($installerRuns.Count -eq 3) 'Reached fault was not recorded'
Check (-not (Get-Content $ReportPath -Raw|ConvertFrom-Json).passed) 'Reached injected fault alone claimed successful rollback'
$mockExit=0
Refused {Run-Installer $CloudflaredBinary 'setup' $fixtureScript 'injected-rollback' $true} 'unexpectedly succeeded'
$mutate=$fixtureScript
Refused {Run-Installer $CloudflaredBinary 'setup' $fixtureScript 'upgrade'} 'inputs changed while the installer was running'
$receipt=Get-Content $ReportPath -Raw|ConvertFrom-Json
Check (-not $receipt.installer_runs[4].inputs_unchanged -and -not $receipt.passed) 'Changed input incorrectly accepted'
$secret='fixture-private-value'
$raw="original failure 原文 한국어"+[Environment]::NewLine
foreach($label in @('Bearer Token:','BearerToken=','AuthToken=','OAuth login password:','OAuthPassword=','OAuthTokenSecret=','TunnelToken=','Authorization: Bearer','Authorization=Basic')) {$raw+=$label+' '+$secret+[Environment]::NewLine}
[IO.File]::WriteAllText((Join-Path $testRoot 'baseline.log'),$raw,[Text.Encoding]::Unicode)
[IO.File]::WriteAllText((Join-Path $testRoot 'baseline.ini'),('Success=false'+[Environment]::NewLine+'Code=baseline-failure'+[Environment]::NewLine+'BearerToken='+$secret),$utf8)
[IO.File]::WriteAllText((Join-Path $testRoot 'upgrade.log'),('x'*1048577),$utf8)
[IO.File]::WriteAllText((Join-Path $testRoot 'same-version-repair.log'),('x'*524289),$utf8)
[IO.File]::WriteAllText((Join-Path $testRoot 'same-version-repair.ini'),'',$utf8)
[IO.File]::WriteAllText((Join-Path $testRoot 'unrelated-secret.txt'),$secret,$utf8)
$before=@{};Get-ChildItem $testRoot -File|ForEach-Object {$before[$_.Name]=(Get-FileHash $_.FullName).Hash}
$records=@(Export-UpgradeDiagnostics)
Check ($records.Count -eq 6) 'Evidence inventory is not the known phase/file allowlist'
Check (@($records|Where-Object exported).Count -eq 4) 'Unexpected export or limit behavior'
Check (-not (Test-Path (Join-Path $EvidenceDirectory 'unrelated-secret.txt'))) 'Unrelated file exported'
Check (-not (Test-Path (Join-Path $EvidenceDirectory 'upgrade.log'))) 'Oversized log exported'
Check (-not (Test-Path (Join-Path $EvidenceDirectory 'same-version-repair.log'))) 'Oversized text exported'
Check (($records|Where-Object name -eq 'upgrade.log').reason -eq 'log_exceeds_1MiB_limit') 'Log-size omission is not explicit'
Check ($null -eq ($records|Where-Object name -eq 'upgrade.log').source_sha256) 'Oversized log must not be fully read or assigned an invented digest'
Check (($records|Where-Object name -eq 'same-version-repair.log').reason -eq 'text_exceeds_524288_character_limit') 'Character-size omission is not explicit'
foreach($record in $records|Where-Object exported) {
    $path=Join-Path $EvidenceDirectory $record.name
    Check (-not ([IO.File]::ReadAllText($path)).Contains($secret)) ('Secret exported: '+$record.name)
    Check ($record.redacted_sha256 -ceq (Get-FileHash $path).Hash.ToLowerInvariant()) 'Exported bytes do not match receipt'
    Check ($record.source_sha256 -ceq $before[$record.name].ToLowerInvariant()) 'Source digest not preserved'
}
Check (([IO.File]::ReadAllText((Join-Path $EvidenceDirectory 'baseline.log'))).Contains('original failure 原文 한국어')) 'Diagnostic text or encoding lost'
foreach($name in $before.Keys){Check ((Get-FileHash (Join-Path $testRoot $name)).Hash -eq $before[$name]) 'Original evidence modified'}
Refused {Export-UpgradeDiagnostics|Out-Null} 'will not be overwritten'
$diagnosticEvidence=$records;Save-Report
$receipt=Get-Content $ReportPath -Raw|ConvertFrom-Json
Check ($receipt.diagnostic_evidence.Count -eq 6 -and -not $receipt.passed) 'Final evidence changed failed result'
[ordered]@{assertions=$assertions;mock_calls=$mockCalls;installer_executed=$false}|ConvertTo-Json -Compress
`

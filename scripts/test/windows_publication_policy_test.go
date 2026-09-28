package scripts

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

func runWindowsPolicyFixture(t *testing.T, files map[string]string, args ...string) string {
	t.Helper()
	pwsh, err := exec.LookPath("pwsh")
	if err != nil {
		t.Skip("PowerShell unavailable for isolated release-policy execution")
	}
	root := t.TempDir()
	for relative, data := range files {
		path := filepath.Join(root, filepath.FromSlash(relative))
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, pwsh, append([]string{"-NoLogo", "-NoProfile", "-NonInteractive", "-File", filepath.Join(root, "fixture.ps1")}, args...)...)
	command.Dir = root
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("isolated policy fixture: %v\n%s", err, output)
	}
	return string(output)
}

func TestWindowsReleaseAcceptanceRequiresEveryVerifiedBoundary(t *testing.T) {
	data, err := os.ReadFile("verify-windows-release-assets.ps1")
	if err != nil {
		t.Fatal(err)
	}
	output := runWindowsPolicyFixture(t, map[string]string{"verifier.ps1": string(data), "fixture.ps1": `
$ErrorActionPreference='Stop'
Set-StrictMode -Version Latest
$tokens=$null;$errors=$null
$ast=[Management.Automation.Language.Parser]::ParseFile((Join-Path $PWD 'verifier.ps1'),[ref]$tokens,[ref]$errors)
if($errors.Count){throw 'Verifier syntax invalid'}
$functions=@($ast.FindAll({param($node) $node -is [Management.Automation.Language.FunctionDefinitionAst] -and $node.Name -eq 'Assert-WindowsReleaseAcceptance'},$false))
if($functions.Count -ne 1){throw 'Single existing acceptance owner is required'}
. ([scriptblock]::Create($functions[0].Extent.Text))
$commit='0123456789012345678901234567890123456789'
$good=@{schema_version=1;repository='eerraa/AgentDock-Workbench';version='1.1.8101';commit=$commit;resolved_commit=$commit;linux_tested_commit=$commit;native_architecture='amd64';baseline_version='1.1.8100';production_touched=$false}
$gates=@('automated_regression','metadata_checksums','linux_backend','linux_race','native_recovery','offscreen_layout','korean_validation','installation_tests','upgrade_recovery')
foreach($gate in $gates){$good[$gate]='passed'}
Assert-WindowsReleaseAcceptance ([pscustomobject]$good) $good.version $commit
$checks=1
foreach($key in @($good.Keys)) {
    foreach($value in @($null,'not_run','failed','skipped')) {
        $bad=$good.Clone();$bad[$key]=$value;$rejected=$false
        try {Assert-WindowsReleaseAcceptance ([pscustomobject]$bad) $good.version $commit} catch {$rejected=$true}
        if(-not $rejected){throw ('Unverified field accepted: '+$key)}
        $checks++
    }
}
foreach($value in @($true,'false',0,1)) {
    $bad=$good.Clone();$bad.production_touched=$value;$rejected=$false
    try {Assert-WindowsReleaseAcceptance ([pscustomobject]$bad) $good.version $commit} catch {$rejected=$true}
    if(-not $rejected){throw 'Ambiguous production safety field accepted'}
    $checks++
}
foreach($version in @('1.1.7','1.1.8','1.1.6100','1.1.8100')) {
    $bad=$good.Clone();$bad.version=$version;$rejected=$false
    try {Assert-WindowsReleaseAcceptance ([pscustomobject]$bad) $version $commit} catch {$rejected=$true}
    if(-not $rejected){throw ('Upstream-baseline or reused fork version accepted: '+$version)}
    $checks++
}
Write-Output ('PASS: '+$checks+' acceptance checks; no executable, installer, network or release operation.')
`})
	if !strings.Contains(output, "PASS:") {
		t.Fatal(output)
	}
	t.Log(strings.TrimSpace(output))
}

func TestWindowsPublicationUsesImmutableAssetsAndActualDownloads(t *testing.T) {
	var workflow identityWorkflow
	if err := yaml.Unmarshal([]byte(readWorkflow(t, "windows-package.yml")), &workflow); err != nil {
		t.Fatal(err)
	}
	publisher := ""
	for _, step := range workflow.Jobs["publish"].Steps {
		if step.Env["RELEASE_TAG"] != "" {
			publisher = step.Run
		}
	}
	if publisher == "" {
		t.Fatal("existing publisher entrypoint is missing")
	}
	for _, scenario := range []string{"new", "partial-draft", "published", "lookup-failure", "foreign-draft", "changed-asset", "missing-digest", "unexpected-asset", "download-corruption", "moved-tag", "wrong-main", "upload-failure", "publication-failure"} {
		t.Run(scenario, func(t *testing.T) {
			output := runWindowsPolicyFixture(t, map[string]string{"publisher.ps1": publisher, "fixture.ps1": publicationFixture}, scenario)
			if !strings.Contains(output, "PASS: "+scenario) {
				t.Fatalf("missing policy receipt: %s", output)
			}
		})
	}
}

// The actual workflow body runs against inert file fixtures and in-process
// Git/GitHub test doubles. No GitHub credential or network request is used.
const publicationFixture = `param([string]$Scenario)
$ErrorActionPreference='Stop'
$env:RELEASE_TAG='v1.1.8101';$env:RELEASE_VERSION='1.1.8101'
$env:RELEASE_COMMIT='0123456789012345678901234567890123456789'
$env:RELEASE_PRERELEASE='false';$env:GITHUB_REPOSITORY='eerraa/AgentDock-Workbench'
$env:RUNNER_TEMP=Join-Path $PWD 'tmp';$env:GITHUB_STEP_SUMMARY=Join-Path $PWD 'summary.md'
$global:publication_root=Join-Path $PWD 'dist/windows-package/release'
New-Item -ItemType Directory -Path $global:publication_root,$env:RUNNER_TEMP,(Join-Path $PWD 'docs/releases'),(Join-Path $PWD 'scripts/test') -Force | Out-Null
$names=@('AgentDockSetup-amd64.exe','AgentDockSetup-amd64.exe.sha256','agentdock_windows_amd64.zip','agentdock_windows_amd64.zip.sha256','install.ps1','install.ps1.sha256','build-report.json','build-report.json.sha256','verification-scope.json','verification-scope.json.sha256')
foreach($name in $names){[IO.File]::WriteAllText((Join-Path $global:publication_root $name),'inert fixture '+$name)}
[IO.File]::WriteAllText((Join-Path $PWD 'docs/releases/v1.1.8101.md'),'Fixture release note')
# The separate acceptance test above exercises the real verifier policy. Here
# the verifier records invocations so this test isolates publication semantics.
[IO.File]::WriteAllText((Join-Path $PWD 'scripts/test/verify-windows-release-assets.ps1'),'$global:validationCount++')
$global:validationCount=0
$global:publication_creates=0;$global:publication_uploads=0;$global:publication_edits=0;$global:publication_downloads=0;$global:publication_remote=$null;$global:publication_uploaded=@()
$global:publication_trace=[Collections.Generic.List[string]]::new()
function Asset([string]$name) {
    $file=Get-Item -LiteralPath (Join-Path $global:publication_root $name)
    return @{name=$name;state='uploaded';size=$file.Length;digest=('sha256:'+(Get-FileHash -LiteralPath $file.FullName).Hash.ToLowerInvariant())}
}
function Release([bool]$draft) {
    return @{id=123;tag_name=$env:RELEASE_TAG;name=('AgentDock Workbench '+$env:RELEASE_VERSION);body=('<!-- agentdock-workbench-source:'+$env:RELEASE_COMMIT+' -->');draft=$draft;prerelease=$false;assets=@();html_url='https://fixture.invalid/release'}
}
if($Scenario -in @('partial-draft','published','foreign-draft','changed-asset','missing-digest','unexpected-asset')) {
    $global:publication_remote=Release ($Scenario -ne 'published')
    $global:publication_remote.assets=@((Asset $names[0]))
    if($Scenario -eq 'foreign-draft'){$global:publication_remote.body='another source'}
    if($Scenario -eq 'changed-asset'){$global:publication_remote.assets[0].size++}
    if($Scenario -eq 'missing-digest'){$global:publication_remote.assets[0].Remove('digest')}
    if($Scenario -eq 'unexpected-asset'){$global:publication_remote.assets[0].name='extra.exe'}
}
function git {
    param([Parameter(ValueFromRemainingArguments=$true)][object[]]$Arguments)
    # Native applications receive each array element as a separate argument.
    # Preserve that boundary in the PowerShell test double as well.
    $Arguments=@($Arguments | ForEach-Object { $_ })
    $global:LASTEXITCODE=0
    if($Arguments[0] -eq 'rev-list'){return $env:RELEASE_COMMIT}
    if($Arguments[0] -ne 'ls-remote'){throw 'Unexpected Git operation'}
    if($Arguments -contains '--tags') {
        $sha=$env:RELEASE_COMMIT
        if($Scenario -eq 'moved-tag'){$sha='abcdefabcdefabcdefabcdefabcdefabcdefabcd'}
        return ($sha+[char]9+'refs/tags/'+$env:RELEASE_TAG)
    }
    $sha=$env:RELEASE_COMMIT
    if($Scenario -eq 'wrong-main'){$sha='abcdefabcdefabcdefabcdefabcdefabcdefabcd'}
    return ($sha+[char]9+'refs/heads/main')
}
function gh {
    param([Parameter(ValueFromRemainingArguments=$true)][object[]]$Arguments)
    # Native applications receive each array element as a separate argument.
    # Preserve that boundary in the PowerShell test double as well.
    $Arguments=@($Arguments | ForEach-Object { $_ })
    $global:LASTEXITCODE=0
    $global:publication_trace.Add(($Arguments|ConvertTo-Json -Compress))
    if($Arguments -contains '--clobber'){throw 'Destructive upload option'}
    if($Arguments[0] -eq 'api') {
        if($Scenario -eq 'lookup-failure'){$global:LASTEXITCODE=1;return 'unknown'}
        if($Arguments[1].EndsWith('/latest')){return ($global:publication_remote|ConvertTo-Json -Depth 8 -Compress)}
        if($null -eq $global:publication_remote){return '[[]]'}
        return ('[['+($global:publication_remote|ConvertTo-Json -Depth 8 -Compress)+']]')
    }
    if($Arguments[0] -ne 'release'){throw 'Unexpected GitHub operation'}
    switch($Arguments[1]) {
        'create' {$global:publication_creates++;$global:publication_remote=Release $true;return}
        'upload' {
            $global:publication_uploads++
            foreach($path in $Arguments[3..($Arguments.IndexOf('--repo')-1)]) {
                $name=[IO.Path]::GetFileName($path)
                if(@($global:publication_remote.assets|Where-Object {$_.name -ceq $name}).Count){throw 'Existing asset was uploaded again'}
                $global:publication_uploaded+=$name;$global:publication_remote.assets+=Asset $name
                if($Scenario -eq 'upload-failure'){$global:LASTEXITCODE=1;return}
            }
            return
        }
        'download' {
            $global:publication_downloads++;$directory=$Arguments[$Arguments.IndexOf('--dir')+1]
            foreach($asset in $global:publication_remote.assets){Copy-Item -LiteralPath (Join-Path $global:publication_root $asset.name) -Destination (Join-Path $directory $asset.name)}
            if($Scenario -eq 'download-corruption'){[IO.File]::AppendAllText((Join-Path $directory $names[0]),'corrupted')}
            return
        }
        'edit' {
            $global:publication_edits++
            if($global:publication_downloads -ne 1){throw 'Publication preceded actual draft download'}
            if($Scenario -eq 'publication-failure'){$global:LASTEXITCODE=1;return}
            foreach($flag in @('--draft=false','--prerelease=false','--latest')){if($Arguments -notcontains $flag){throw 'Formal publication flag missing'}}
            $global:publication_remote.draft=$false;$global:publication_remote.prerelease=$false;return
        }
        default {throw 'Unexpected release operation'}
    }
}
$failed=$false;$message=''
try {& (Join-Path $PWD 'publisher.ps1')} catch {$failed=$true;$message=$_.Exception.Message}
$success=$Scenario -in @('new','partial-draft')
if($failed -eq $success){throw ('Unexpected '+$Scenario+' outcome: '+$message+'; remote='+($global:publication_remote|ConvertTo-Json -Depth 8 -Compress)+'; calls='+($global:publication_trace -join ';'))}
$expectedErrors=@{
    'published'='already published release is immutable'; 'lookup-failure'='Release lookup failed';
    'foreign-draft'='not bound to this source'; 'changed-asset'='different or incomplete';
    'unexpected-asset'='Unexpected or duplicate'; 'download-corruption'='Downloaded bytes differ';
    'moved-tag'='Remote release tag moved'; 'wrong-main'='canonical remote main';
    'upload-failure'='Asset upload did not complete'; 'publication-failure'='Publication result is unknown'
}
if($expectedErrors.ContainsKey($Scenario) -and -not $message.Contains($expectedErrors[$Scenario])) {
    throw ('The intended rejection boundary was not reached: '+$Scenario+': '+$message)
}
if($success) {
    if($global:publication_downloads -ne 2 -or $global:publication_edits -ne 1 -or $global:validationCount -ne 3 -or $global:publication_remote.draft){throw 'Publication skipped verification'}
    if(-not (Test-Path 'dist/windows-package/publication-verification.json')){throw 'Actual download evidence missing'}
    if($Scenario -eq 'partial-draft' -and ($global:publication_creates -ne 0 -or $global:publication_uploaded.Count -ne 9)){throw 'Draft resume did not preserve existing bytes'}
} elseif($Scenario -notin @('download-corruption','upload-failure','publication-failure')) {
    if($global:publication_creates+$global:publication_uploads+$global:publication_edits -ne 0){throw 'Unsafe state reached a mutating release call'}
} elseif($Scenario -ne 'publication-failure' -and $global:publication_edits -ne 0){throw 'Incomplete bytes reached publication'}
Write-Output ('PASS: '+$Scenario+'; isolated publication policy only, no actual network or release mutation.')
`

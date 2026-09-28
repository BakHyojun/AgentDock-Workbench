#requires -Version 7.0
[CmdletBinding()]
param([Parameter(Mandatory=$true)][string] $CacheDirectory)
Set-StrictMode -Version Latest
$ErrorActionPreference='Stop'
$prepare=Join-Path $PSScriptRoot '..\..\packaging\windows\prepare-bundled-rg.ps1'
$spec=Get-Content (Join-Path $PSScriptRoot '..\..\internal\bundledrg\windows-amd64.json') -Raw | ConvertFrom-Json
$source=Join-Path $CacheDirectory $spec.asset
if (-not(Test-Path -LiteralPath $source)) {throw 'Required official archive fixture is missing; packaging tests do not skip.'}
$root=Join-Path ([IO.Path]::GetTempPath()) ('agentdock-rg-package-test-'+[guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $root | Out-Null
# A cache-only test must never accidentally download or invoke another process.
function Invoke-WebRequest {throw 'Network is forbidden in this packaging regression.'}
function Expect-Failure([scriptblock]$Action,[string]$Message) {
    try {& $Action | Out-Null} catch {
        if ($_.Exception.Message -notlike ('*'+$Message+'*')) {throw "Unexpected failure: $($_.Exception.Message)"}
        return
    }
    throw "Expected packaging failure containing: $Message"
}
try {
    $good=Join-Path $root 'good'
    & $prepare -Destination $good -CacheDirectory $CacheDirectory | Out-Null
    & $prepare -Destination $good -VerifyOnly | Out-Null
    if ((Get-FileHash (Join-Path $good 'rg.exe')).Hash.ToLowerInvariant() -ne $spec.files[0].sha256) {throw 'Prepared executable does not match the official pin.'}
    foreach ($case in @('tampered-archive','partial-archive')) {
        $cache=Join-Path $root $case
        New-Item -ItemType Directory -Path $cache | Out-Null
        $bytes=[IO.File]::ReadAllBytes($source)
        if($case -eq 'partial-archive') {$bytes=$bytes[0..127]} else {$bytes[-1]=$bytes[-1] -bxor 1}
        [IO.File]::WriteAllBytes((Join-Path $cache $spec.asset),$bytes)
        $target=Join-Path $root ($case+'-target')
        Expect-Failure {& $prepare -Destination $target -CacheDirectory $cache} 'Cached ripgrep archive'
        if(Test-Path $target){throw 'Rejected archive produced a component directory.'}
    }
    foreach ($case in @('changed-executable','wrong-architecture','missing-licence','changed-licence')) {
        $target=Join-Path $root $case
        Copy-Item -LiteralPath $good -Destination $target -Recurse
        switch($case) {
            'changed-executable' {$path=Join-Path $target 'rg.exe'; $bytes=[IO.File]::ReadAllBytes($path); $bytes[-1]=$bytes[-1] -bxor 1; [IO.File]::WriteAllBytes($path,$bytes)}
            'wrong-architecture' {$path=Join-Path $target 'rg.exe'; $bytes=[IO.File]::ReadAllBytes($path); $offset=[BitConverter]::ToInt32($bytes,0x3c); $bytes[$offset+4]=0x64; $bytes[$offset+5]=0xaa; [IO.File]::WriteAllBytes($path,$bytes)}
            'missing-licence' {Remove-Item -LiteralPath (Join-Path $target 'LICENSE-MIT')}
            'changed-licence' {[IO.File]::WriteAllText((Join-Path $target 'COPYING'),'changed')}
        }
        $message=if($case -eq 'missing-licence') {'LICENSE-MIT'} elseif($case -eq 'changed-licence') {'Incomplete or redirected'} else {'SHA-256 mismatch'}
        Expect-Failure {& $prepare -Destination $target -VerifyOnly} $message
    }
    Add-Type -AssemblyName System.IO.Compression.FileSystem
    $payload=Join-Path $root 'payload 한글 with spaces'
    $component=Join-Path $payload 'share\agentdock\bin'
    New-Item -ItemType Directory -Path (Split-Path $component) -Force | Out-Null
    Copy-Item -LiteralPath $good -Destination $component -Recurse
    $goodArchive=Join-Path $root 'valid package 한글.zip'
    [IO.Compression.ZipFile]::CreateFromDirectory($payload,$goodArchive)
    $verified=& $prepare -ArchivePath $goodArchive
    if ($verified.version -ne $spec.version -or $verified.files -ne 5 -or $verified.platform -ne 'windows/amd64') {throw 'ZIP verification did not report the pinned component.'}
    $zipChecks=1
    foreach ($case in @('missing-component','partial-component','changed-rg','changed-manifest','duplicate-rg','case-collision','redirected-rg','unexpected-component')) {
        $zipPath=Join-Path $root ($case+'.zip')
        Copy-Item -LiteralPath $goodArchive -Destination $zipPath
        $zip=[IO.Compression.ZipFile]::Open($zipPath,[IO.Compression.ZipArchiveMode]::Update)
        try {
            $entryName='share/agentdock/bin/rg.exe'
            switch ($case) {
                'missing-component' {foreach($entry in @($zip.Entries)) {$entry.Delete()}}
                'partial-component' {$zip.GetEntry('share/agentdock/bin/COPYING').Delete()}
                'changed-rg' {
                    $entry=$zip.GetEntry($entryName); $stream=$entry.Open()
                    try {$null=$stream.Seek(-1,[IO.SeekOrigin]::End); $original=$stream.ReadByte(); $null=$stream.Seek(-1,[IO.SeekOrigin]::End); $stream.WriteByte([byte]($original -bxor 1))} finally {$stream.Dispose()}
                }
                'changed-manifest' {
                    $entry=$zip.GetEntry('share/agentdock/bin/manifest.json'); $stream=$entry.Open()
                    try {$stream.WriteByte([byte][char]'!')} finally {$stream.Dispose()}
                }
                'duplicate-rg' {$null=$zip.CreateEntry($entryName)}
                'case-collision' {$null=$zip.CreateEntry('share/agentdock/bin/RG.EXE')}
                'redirected-rg' {$zip.GetEntry($entryName).ExternalAttributes=([int]0xa1ff -shl 16)}
                'unexpected-component' {$null=$zip.CreateEntry('share/agentdock/bin/extra.exe')}
            }
        } finally {$zip.Dispose()}
        $message=switch ($case) {
            'changed-rg' {'ZIP SHA-256 mismatch'}
            'changed-manifest' {'ZIP SHA-256 mismatch'}
            'redirected-rg' {'Invalid bundled ripgrep ZIP entry'}
            default {'ZIP inventory'}
        }
        Expect-Failure {& $prepare -ArchivePath $zipPath} $message
        $zipChecks++
    }
    # Exercise the real packaging entrypoints' connection to this component owner.
    $repository=(Resolve-Path (Join-Path $PSScriptRoot '..\..')).Path
    foreach ($relative in @('packaging\windows\build-windows-release.ps1','packaging\windows\build-windows-offline-setup.ps1','scripts\test\verify-windows-release-assets.ps1')) {
        $script=Get-Content -LiteralPath (Join-Path $repository $relative) -Raw
        if (-not $script.Contains('prepare-bundled-rg.ps1') -or -not $script.Contains('-ArchivePath')) {throw "Unconnected component verification: $relative"}
    }
    Write-Host "Bundled ripgrep packaging: 8 directory checks, $zipChecks ZIP checks, 3 packaging connections passed; 0 skips."
    [pscustomobject]@{directory_checks=8;zip_checks=$zipChecks;packaging_connections=3;failed=0;skipped=0;network='forbidden';setup_invoked=$false}
} finally {
    Remove-Item -LiteralPath $root -Recurse -Force
}

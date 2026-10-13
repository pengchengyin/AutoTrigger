param([string]$Executable = '', [string]$Fixture = '', [switch]$Gui, [switch]$Archive)
$ErrorActionPreference = 'Stop'
$repoRoot = Split-Path -Parent (Split-Path -Parent $PSScriptRoot)
if (-not $Executable) { $Executable = Join-Path $repoRoot 'dist\AutoTrigger.exe' }
if (-not $Fixture) { $Fixture = Join-Path $repoRoot 'dist\test\wizard-fixture.exe' }
$Executable = (Resolve-Path $Executable).Path
$Fixture = (Resolve-Path $Fixture).Path
$root = Join-Path ([IO.Path]::GetTempPath()) ('AT-smoke-' + [Guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $root | Out-Null
$installed = Join-Path $root 'apps'
New-Item -ItemType Directory -Path $installed | Out-Null
$env:AT_FIXTURE_DEST = $installed
$config = @{
    attempt_interval_sec = 0
    max_attempts_per_file = 1
    timeout_sec = 60
    archive = @{ require_sevenzip = $false }
    ocr = @{ enabled = $false }
    install_verify = @{ watch_dirs = @($installed); watch_exclude = @(); settle_sec = 0; settle_max_sec = 2; quiet_polls = 1 }
    gui_automation = @{ enabled = [bool]$Gui }
}
$configPath = Join-Path $root 'config.json'
[IO.File]::WriteAllText($configPath, ($config | ConvertTo-Json -Depth 10), (New-Object Text.UTF8Encoding $false))
if (-not $Gui) {
    $localFixture = Join-Path $root 'fixture.exe'
    Copy-Item -LiteralPath $Fixture -Destination $localFixture
    $launcher = Join-Path $root 'install.cmd'
    [IO.File]::WriteAllText($launcher, ('@echo off' + "`r`n" + '"' + $localFixture + '" --silent-fixture' + "`r`n"), [Text.Encoding]::ASCII)
} else { $launcher = $Fixture }
if ($Archive) {
    if ($Gui) { throw 'Use separate GUI and archive smoke runs' }
    $archivePath = Join-Path $root 'fixture.zip'
    Compress-Archive -LiteralPath $launcher -DestinationPath $archivePath
    $launcher = $archivePath
}
$resultPath = Join-Path $root 'result.json'
try {
    $toolArgs = @('--input', $launcher, '--config', $configPath, '--output', $resultPath, '--no-ocr')
    if (-not $Archive) { $toolArgs += '--no-archive' }
    & $Executable @toolArgs
    if ($LASTEXITCODE -ne 0) { throw "Tool returned $LASTEXITCODE" }
    $result = Get-Content -LiteralPath $resultPath -Raw | ConvertFrom-Json
    $file = $result.results[0]
    if ($file.final_status -ne 'install_completed') { [Console]::Error.WriteLine(($file | ConvertTo-Json -Depth 20)); throw "Unexpected status: $($file.final_status) $($file.notes)" }
    if (-not (Test-Path (Join-Path $installed 'FixtureApp\FixtureApp.exe'))) { throw 'Fixture artifact missing' }
    if ($Gui -and $file.attempts[0].gui_actions.Count -lt 3) { throw 'GUI click chain incomplete' }
    [Console]::Out.WriteLine(($result | ConvertTo-Json -Depth 30 -Compress))
} finally {
    Remove-Item Env:AT_FIXTURE_DEST -ErrorAction SilentlyContinue
    Remove-Item -LiteralPath $root -Recurse -Force -ErrorAction SilentlyContinue
}

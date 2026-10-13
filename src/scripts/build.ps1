param([switch]$Release, [string]$Go = 'go')
$ErrorActionPreference = 'Stop'
Set-Location (Split-Path -Parent $PSScriptRoot)
$repoRoot = Split-Path -Parent (Get-Location).Path
$runtimeDir = Join-Path $repoRoot 'dist'
$testDir = Join-Path $runtimeDir 'test'
$executable = Join-Path $runtimeDir 'AutoTrigger.exe'
New-Item -ItemType Directory -Force -Path $testDir | Out-Null
if ((& $Go version) -notmatch 'go1\.20\.') { throw 'Go 1.20.x is required for Windows 7 support' }
$env:CGO_ENABLED = '0'
function Invoke-Go { param([string[]]$Arguments) & $Go @Arguments; if ($LASTEXITCODE -ne 0) { throw "Go command failed: $Arguments" } }
$unformatted = & $Go fmt ./...
if ($unformatted) { throw "Formatting changed files: $unformatted" }
Invoke-Go -Arguments @('vet', './...')
Invoke-Go -Arguments @('test', './...', ('-coverprofile=' + (Join-Path $testDir 'coverage.out')))
Invoke-Go -Arguments @('run', './tools/schema-gen')
Invoke-Go -Arguments @('run', './tools/icon-gen')
if ($Release) { Invoke-Go -Arguments @('run', './tools/template-gen', '--masters', 'masters', '--out', 'internal/resources/assets/builtin/templates', '--release') }
$env:GOOS = 'windows'; $env:GOARCH = 'amd64'
Invoke-Go -Arguments @('build', '-trimpath', '-ldflags=-s -w', '-o', $executable, './cmd/autotrigger')
Invoke-Go -Arguments @('vet', './...')
$env:GOOS = 'linux'; Invoke-Go -Arguments @('build', './...')
$env:GOOS = 'darwin'; Invoke-Go -Arguments @('build', './...')
Remove-Item Env:GOOS
Invoke-Go -Arguments @('run', './tools/pe-check', $executable)
if ($Release) {
    & $executable --self-test templates
    if ($LASTEXITCODE -ne 0) { throw 'Template source validation failed' }
    if (-not (Get-Command golangci-lint -ErrorAction SilentlyContinue)) { throw 'Release requires golangci-lint v1.54.x' }
    & golangci-lint run
    if ($LASTEXITCODE -ne 0) { throw 'Lint failed' }
}
Write-Host ('Built ' + $executable)

param([switch]$Release, [string]$Go = 'go')
$ErrorActionPreference = 'Stop'
Set-Location (Split-Path -Parent $PSScriptRoot)
if ((& $Go version) -notmatch 'go1\.20\.') { throw 'Go 1.20.x is required for Windows 7 support' }
$env:CGO_ENABLED = '0'
function Invoke-Go { param([string[]]$Arguments) & $Go @Arguments; if ($LASTEXITCODE -ne 0) { throw "Go command failed: $Arguments" } }
$unformatted = & $Go fmt ./...
if ($unformatted) { throw "Formatting changed files: $unformatted" }
Invoke-Go -Arguments @('vet', './...')
Invoke-Go -Arguments @('test', './...', '-coverprofile=coverage.out')
Invoke-Go -Arguments @('run', './tools/schema-gen')
Invoke-Go -Arguments @('run', './tools/icon-gen')
if ($Release) { Invoke-Go -Arguments @('run', './tools/template-gen', '--masters', 'masters', '--out', 'internal/resources/assets/builtin/templates', '--release') }
New-Item -ItemType Directory -Force -Path dist | Out-Null
$env:GOOS = 'windows'; $env:GOARCH = 'amd64'
Invoke-Go -Arguments @('build', '-trimpath', '-ldflags=-s -w', '-o', 'dist/AutoTrigger.exe', './cmd/autotrigger')
Invoke-Go -Arguments @('vet', './...')
$env:GOOS = 'linux'; Invoke-Go -Arguments @('build', './...')
$env:GOOS = 'darwin'; Invoke-Go -Arguments @('build', './...')
Remove-Item Env:GOOS
Invoke-Go -Arguments @('run', './tools/pe-check', 'dist/AutoTrigger.exe')
$config = & .\dist\AutoTrigger.exe --dump-config
if ($LASTEXITCODE -ne 0) { throw 'Configuration export failed' }
[IO.File]::WriteAllText((Join-Path (Get-Location) 'dist/config.example.json'), ($config -join "`n"), (New-Object Text.UTF8Encoding $false))
if ($Release) {
    & .\dist\AutoTrigger.exe --self-test templates
    if ($LASTEXITCODE -ne 0) { throw 'Template source validation failed' }
    if (-not (Get-Command golangci-lint -ErrorAction SilentlyContinue)) { throw 'Release requires golangci-lint v1.54.x' }
    & golangci-lint run
    if ($LASTEXITCODE -ne 0) { throw 'Lint failed' }
}
Write-Host 'Built dist/AutoTrigger.exe'

[CmdletBinding()]
param([switch]$Offline, [switch]$SkipInstaller)
$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
$keys = @('GOTOOLCHAIN','CGO_ENABLED','GOPROXY','GOSUMDB','LIVE','MYGO_OUTPUT_PROBE','NM_SESSION','NM_FRAMES','NM_LOGIN_FRAME','SPEECH_ROOT','SPEECH_MODEL','SPEECH_WAV','SHERPA_ONNX_DIR')
$previous = @{}
foreach ($key in $keys) { $previous[$key] = [Environment]::GetEnvironmentVariable($key, 'Process') }
Push-Location -LiteralPath $root
try {
    $env:GOTOOLCHAIN = 'local'
    $env:CGO_ENABLED = '0'
    # Keep ordinary validation offline even if the caller has enabled live/UI-render tests.
    foreach ($key in @('LIVE','MYGO_OUTPUT_PROBE','NM_SESSION','NM_FRAMES','NM_LOGIN_FRAME','SPEECH_ROOT','SPEECH_MODEL','SPEECH_WAV','SHERPA_ONNX_DIR')) { [Environment]::SetEnvironmentVariable($key, $null, 'Process') }
    if ($Offline) { $env:GOPROXY = 'off'; $env:GOSUMDB = 'off' }
    $required = ((Get-Content -LiteralPath 'go.mod' | Where-Object { $_ -match '^go ' }) -split '\s+')[1]
    $actual = go env GOVERSION
    if ($LASTEXITCODE -ne 0 -or $actual -ne "go$required") { throw "Use Go $required on PATH; found $actual" }
    # ./... skips the vendored audio, UI and Windows text regressions.
    go test github.com/egoist/mygo/audio github.com/egoist/mygo/ui github.com/egoist/mygo/internal/text
    if ($LASTEXITCODE -ne 0) { throw 'MyGo dependency tests failed' }
    & (Join-Path $PSScriptRoot 'check-account-callbacks.ps1')
    go test ./...
    if ($LASTEXITCODE -ne 0) { throw 'Tests failed' }
    go tool mygo vet .
    if ($LASTEXITCODE -ne 0) { throw 'MyGo checks failed' }
    $buildArgs = @('-platform', 'windows/amd64')
    if ($SkipInstaller) { $buildArgs += '-skip-installer' }
    go tool mygo build @buildArgs
    if ($LASTEXITCODE -ne 0) { throw 'Windows build failed' }
    Write-Host 'Built in build/windows-amd64. Review RELEASING.md before distribution.'
} finally {
    Pop-Location
    foreach ($key in $keys) { [Environment]::SetEnvironmentVariable($key, $previous[$key], 'Process') }
}

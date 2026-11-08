[CmdletBinding()]
param(
    [Parameter(Mandatory=$true)][string]$Compiler,
    [ValidateRange(2,32)][int]$Threads = 2,
    [switch]$Offline
)
$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
$compilerPath = (Resolve-Path -LiteralPath $Compiler).Path
$library = & $compilerPath --print-file-name libsynchronization.a
if ($LASTEXITCODE -ne 0 -or -not (Test-Path -LiteralPath $library)) { throw 'Compiler lacks the required MinGW synchronization library' }
$keys=@('PATH','CC','CGO_ENABLED','GOTOOLCHAIN','GOMAXPROCS','GORACE','GOPROXY','GOSUMDB','LIVE','MYGO_OUTPUT_PROBE','NM_SESSION','NM_FRAMES','NM_LOGIN_FRAME','SPEECH_ROOT','SPEECH_MODEL','SPEECH_WAV','SHERPA_ONNX_DIR')
$previous=@{}
foreach($key in $keys){$previous[$key]=[Environment]::GetEnvironmentVariable($key,'Process')}
Push-Location -LiteralPath $root
try {
    $env:PATH=(Split-Path -Parent $compilerPath)+';'+$env:PATH
    $env:CC='"'+$compilerPath+'"'
    $env:CGO_ENABLED='1'; $env:GOTOOLCHAIN='local'; $env:GOMAXPROCS="$Threads"; $env:GORACE='halt_on_error=1'
    foreach($key in @('LIVE','MYGO_OUTPUT_PROBE','NM_SESSION','NM_FRAMES','NM_LOGIN_FRAME','SPEECH_ROOT','SPEECH_MODEL','SPEECH_WAV','SHERPA_ONNX_DIR')){[Environment]::SetEnvironmentVariable($key,$null,'Process')}
    if($Offline){$env:GOPROXY='off';$env:GOSUMDB='off'}
    go test -race github.com/egoist/mygo/audio github.com/egoist/mygo/ui -count=5 -timeout=2m
    if($LASTEXITCODE -ne 0){throw 'Audio backend race checks failed'}
    & (Join-Path $PSScriptRoot 'check-account-callbacks.ps1') -Race
    go test -race ./... -count=1 -timeout=15m
    if($LASTEXITCODE -ne 0){throw 'Race checks failed'}
} finally {
    Pop-Location
    foreach($key in $keys){[Environment]::SetEnvironmentVariable($key,$previous[$key],'Process')}
}

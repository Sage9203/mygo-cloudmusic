[CmdletBinding()]
param([ValidateRange(1,20)][int]$Count=5,[switch]$Race)
$ErrorActionPreference='Stop'
$root=Split-Path -Parent $PSScriptRoot
$scratch=Join-Path ([IO.Path]::GetTempPath()) ('mygo-callback-test-'+[guid]::NewGuid().ToString('N'))
$keys=@('GOTOOLCHAIN','GOPROXY','GOSUMDB','CGO_ENABLED','GOFLAGS')
$saved=@{}
foreach($key in $keys){$saved[$key]=[Environment]::GetEnvironmentVariable($key,'Process')}
try {
 New-Item -ItemType Directory -Path $scratch | Out-Null
 foreach($name in @('internal','vendor','icon','go.mod','go.sum')){Copy-Item -LiteralPath (Join-Path $root $name) -Destination $scratch -Recurse}
 $app=Join-Path $scratch 'internal/app/app.go'
 $text=[IO.File]::ReadAllText($app).Replace("`r`n","`n")
 $start=$text.IndexOf('func (a *app) post(fn func()) {')
 $end=$text.IndexOf('func (a *app) redraw()', $start)
 if($start -lt 0 -or $end -lt 0){throw 'Cannot locate UI dispatch boundary'}
 [IO.File]::WriteAllText($app,$text.Substring(0,$start)+"func (a *app) post(fn func()) { callbackQueue <- fn }`n`n"+$text.Substring($end))
 Copy-Item -LiteralPath (Join-Path $PSScriptRoot 'regression/account_callbacks_test.go.txt') -Destination (Join-Path $scratch 'internal/app/account_callbacks_test.go')
 $env:GOTOOLCHAIN='local';$env:GOPROXY='off';$env:GOSUMDB='off';$env:CGO_ENABLED=if($Race){'1'}else{'0'};$env:GOFLAGS='-mod=vendor'
 $raceArgs=@();if($Race){$raceArgs=@('-race')}
 Push-Location -LiteralPath $scratch
 try {go test @raceArgs ./internal/app -run '^TestAccountCallback' "-count=$Count" -v -timeout=3m;if($LASTEXITCODE -ne 0){throw 'Account callback regressions failed'}} finally {Pop-Location}
} finally {
 foreach($key in $keys){[Environment]::SetEnvironmentVariable($key,$saved[$key],'Process')}
 $expected=[IO.Path]::GetFullPath([IO.Path]::GetTempPath()).TrimEnd('\')+'\'
 $resolved=[IO.Path]::GetFullPath($scratch)
 if(!$resolved.StartsWith($expected,[StringComparison]::OrdinalIgnoreCase) -or [IO.Path]::GetFileName($resolved) -notlike 'mygo-callback-test-*'){throw 'Unsafe cleanup path'}
 if(Test-Path -LiteralPath $resolved){Remove-Item -LiteralPath $resolved -Recurse -Force}
}

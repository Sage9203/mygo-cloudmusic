[CmdletBinding()]
param()
$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
$keys = @('GOTOOLCHAIN','GOOS','GOARCH','CGO_ENABLED')
$previous = @{}
foreach ($key in $keys) { $previous[$key] = [Environment]::GetEnvironmentVariable($key, 'Process') }
Push-Location -LiteralPath $root
try {
    $env:GOTOOLCHAIN='local'; $env:GOOS='windows'; $env:GOARCH='amd64'; $env:CGO_ENABLED='0'
    $lines = @(go list -deps -f '{{if .Module}}{{.Module.Path}}|{{.Module.Version}}|{{.Module.Dir}}{{end}}' .)
    if ($LASTEXITCODE -ne 0) { throw 'Cannot resolve Windows application dependencies' }
    $inventory = @()
    $bundle = [Text.StringBuilder]::new()
    foreach ($line in ($lines | Where-Object { $_ } | Sort-Object -Unique)) {
        $parts = $line -split '\|'
        $moduleDir = $parts[2]
        $vendored = Join-Path $root ('vendor/' + $parts[0])
        if (Test-Path -LiteralPath $vendored) { $moduleDir = $vendored }
        $files = @(Get-ChildItem -LiteralPath $moduleDir -File | Where-Object { $_.Name -match '^(UNLICENSE|LICENSE|LICENCE|COPYING|NOTICE|COPYRIGHT)' })
        $licenses = @()
        foreach ($file in $files) {
            $licenses += @{file=$file.Name; sha256=(Get-FileHash -LiteralPath $file.FullName -Algorithm SHA256).Hash.ToLowerInvariant()}
            [void]$bundle.AppendLine("Module: $($parts[0]) $($parts[1]) / $($file.Name)")
            [void]$bundle.AppendLine([IO.File]::ReadAllText($file.FullName))
            [void]$bundle.AppendLine()
        }
        if ($files.Count -eq 0) { Write-Warning "No root notice found: $($parts[0])" }
        $inventory += @{module=$parts[0]; version=$parts[1]; scope='Windows amd64 application dependency graph'; rootLicenseFiles=$licenses; review='Module root notices; additional component notices are in third_party/NOTICE.md'}
    }
    # Curated component notices cover licenses inside modules and embedded data/assets.
    foreach ($file in (Get-ChildItem -LiteralPath (Join-Path $root 'third_party') -Recurse -File | Where-Object { $_.Name -match '^(UNLICENSE|LICENSE|LICENCE|COPYING|NOTICE|COPYRIGHT)' } | Sort-Object FullName)) {
        $relative = $file.FullName.Substring($root.Length + 1).Replace('\','/')
        [void]$bundle.AppendLine("Component notice: $relative")
        [void]$bundle.AppendLine([IO.File]::ReadAllText($file.FullName))
        [void]$bundle.AppendLine()
    }
    $encoding = [Text.UTF8Encoding]::new($false)
    [IO.File]::WriteAllText((Join-Path $root 'THIRD-PARTY-LICENSES.txt'), $bundle.ToString(), $encoding)
    [IO.File]::WriteAllText((Join-Path $root 'docs/dependency-inventory.json'), ($inventory | ConvertTo-Json -Depth 6), $encoding)
    Write-Host "Collected root notices for $($inventory.Count) modules, including this application."
} finally {
    Pop-Location
    foreach ($key in $keys) { [Environment]::SetEnvironmentVariable($key, $previous[$key], 'Process') }
}
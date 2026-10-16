#requires -Version 5
# EuroKey check script: vet + unit/UI tests + build.
#
#   powershell -File scripts/check-eurokey.ps1            # vet + tests + build
#   powershell -File scripts/check-eurokey.ps1 -NoUi      # skip settings-window tests
#   powershell -File scripts/check-eurokey.ps1 -BuildOnly # just build the exe
#
# The UI tests open the real settings window; they self-skip when there is no
# interactive desktop session.

param(
    [switch]$NoUi,
    [switch]$BuildOnly
)

$ErrorActionPreference = "Stop"
$root = Split-Path $PSScriptRoot -Parent
Push-Location $root
try {
    if (-not $BuildOnly) {
        Write-Host "== go vet =="
        go vet ./...
        if ($LASTEXITCODE -ne 0) { throw "go vet failed" }

        Write-Host "== go test =="
        $env:GOWORK = "off"
        $skip = if ($NoUi) { "TestUi" } else { "" }
        if ($skip) {
            go test ./... -count=1 -skip $skip
        } else {
            go test ./... -count=1
        }
        if ($LASTEXITCODE -ne 0) { throw "go test failed" }
    }

    Write-Host "== build =="
    go build -ldflags "-H windowsgui -s -w" -o eurokey.exe .
    if ($LASTEXITCODE -ne 0) { throw "go build failed" }

    Write-Host "`nPASS: EuroKey checks OK"
}
finally {
    Pop-Location
}

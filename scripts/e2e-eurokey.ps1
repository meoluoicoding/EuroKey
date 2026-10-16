#requires -Version 5
# EuroKey end-to-end typing test: builds the app, then drives a real browser
# with rod while typing through the OS keyboard hook.
#
#   powershell -File scripts/e2e-eurokey.ps1            # language from eurokey.ini
#   powershell -File scripts/e2e-eurokey.ps1 -Lang de   # force Deutsch
#
# Prerequisites:
#   - EuroKey must NOT be running (single-instance; the test sets
#     EUROKEY_TEST_MAGIC at launch, which cannot be done on a live instance).
#   - An interactive desktop session and Chrome/Edge.

param(
    [ValidateSet("", "de", "fr", "es")]
    [string]$Lang = ""
)

$ErrorActionPreference = "Stop"
$root = Split-Path $PSScriptRoot -Parent
Push-Location $root
try {
    if (Get-Process eurokey -ErrorAction SilentlyContinue) {
        throw "EuroKey is running. Exit it from the tray first, then rerun."
    }

    Write-Host "== build =="
    go build -ldflags "-H windowsgui -s -w" -o eurokey.exe .
    if ($LASTEXITCODE -ne 0) { throw "go build failed" }

    $env:EUROKEY_E2E = "1"
    if ($Lang) { $env:EUROKEY_E2E_LANG = $Lang }

    Push-Location e2e
    try {
        Write-Host "== e2e (rod + SendInput) =="
        go test -run TestEuroKeyTyping -v -count=1
        if ($LASTEXITCODE -ne 0) { throw "e2e failed (see e2e-fail.png)" }
    }
    finally { Pop-Location }

    Write-Host "`nPASS: EuroKey e2e OK"
}
finally {
    Pop-Location
}

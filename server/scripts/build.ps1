$ErrorActionPreference = "Stop"

$scriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$serverDir = Split-Path -Parent $scriptDir

Push-Location $serverDir
try {
    $goCommand = Get-Command go -ErrorAction SilentlyContinue
    if (-not $goCommand) {
        throw "go command not found. Please install Go and add it to PATH first."
    }

    & $goCommand.Source build -o ".\bin\hostdeck.exe" ".\cmd\hostdeck"
    npm --prefix "..\web" run build
}
finally {
    Pop-Location
}

$ErrorActionPreference = 'Stop'
$projectRoot = Split-Path -Parent $PSScriptRoot
$pidPath = Join-Path $projectRoot 'data\server.pid'
if (!(Test-Path -LiteralPath $pidPath)) { Write-Host 'No local server PID found.'; exit }
$serverProcess = Get-Process -Id ([int](Get-Content -LiteralPath $pidPath)) -ErrorAction SilentlyContinue
$binaryPath = Join-Path $projectRoot 'bin\leadhelper.exe'
if ($serverProcess -and $serverProcess.Path -eq $binaryPath) { Stop-Process -Id $serverProcess.Id }
Remove-Item -LiteralPath $pidPath
Write-Host 'LeadHelper local server stopped.'

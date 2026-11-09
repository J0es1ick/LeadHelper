$ErrorActionPreference = 'Stop'
$projectRoot = [System.IO.Path]::GetFullPath((Split-Path -Parent $PSScriptRoot))
$readyPath = Join-Path $projectRoot 'bin\leadhelper.ready.exe'
$binaryPath = Join-Path $projectRoot 'bin\leadhelper.exe'
$backupPath = Join-Path $projectRoot 'bin\leadhelper.previous.exe'
$frontendReady = Join-Path $projectRoot 'frontend\dist.ready'
$frontendLive = Join-Path $projectRoot 'frontend\dist'
if (!(Test-Path -LiteralPath $readyPath)) { throw 'Prepared update missing. Run stop.ps1, then start.ps1 -Rebuild.' }
# Validate the saved Windows credential before stopping the working service.
$credentialPath = Join-Path $projectRoot 'data\imap-credential.xml'
if (Test-Path -LiteralPath $credentialPath) {
    try { $credentialCheck = Import-Clixml -LiteralPath $credentialPath; $credentialCheck = $null }
    catch { throw 'Run this update from the Windows account that connected the mailbox. The existing service has not been stopped.' }
}
& (Join-Path $PSScriptRoot 'stop.ps1')
if (Test-Path -LiteralPath (Join-Path $frontendReady 'index.html')) {
    New-Item -ItemType Directory -Path $frontendLive -Force | Out-Null
    # Keep previous hashed assets for already-open tabs; publish the entry point last.
    foreach ($entry in Get-ChildItem -LiteralPath $frontendReady) {
        if ($entry.Name -ne 'index.html') { Copy-Item -LiteralPath $entry.FullName -Destination $frontendLive -Recurse -Force }
    }
    Copy-Item -LiteralPath (Join-Path $frontendReady 'index.html') -Destination (Join-Path $frontendLive 'index.html') -Force
}
if (Test-Path -LiteralPath $binaryPath) { Copy-Item -LiteralPath $binaryPath -Destination $backupPath -Force }
Copy-Item -LiteralPath $readyPath -Destination $binaryPath -Force
& (Join-Path $PSScriptRoot 'start.ps1')

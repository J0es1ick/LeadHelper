param([switch]$Docker, [switch]$Rebuild)
$ErrorActionPreference = 'Stop'
$projectRoot = Split-Path -Parent $PSScriptRoot
Set-Location -LiteralPath $projectRoot
if ($Docker) {
    docker compose up -d --build
    if ($LASTEXITCODE -ne 0) { throw 'Docker deployment failed. Start Docker Desktop and retry.' }
    Write-Host 'LeadHelper: http://localhost:8090'
    exit
}
$binaryPath = Join-Path $projectRoot 'bin\leadhelper.exe'
$frontendPath = Join-Path $projectRoot 'frontend'
$indexPath = Join-Path $frontendPath 'dist\index.html'
if ($Rebuild -or !(Test-Path -LiteralPath $indexPath)) {
    Push-Location -LiteralPath $frontendPath
    try {
        if (!(Test-Path -LiteralPath 'node_modules\typescript')) { npm ci --no-audit --no-fund; if ($LASTEXITCODE -ne 0) { throw 'npm ci failed' } }
        node node_modules/typescript/bin/tsc -b
        if ($LASTEXITCODE -ne 0) { throw 'TypeScript check failed' }
        node node_modules/vite/bin/vite.js build
        if ($LASTEXITCODE -ne 0) { throw 'Frontend build failed' }
    } finally { Pop-Location }
}
if ($Rebuild -or !(Test-Path -LiteralPath $binaryPath)) {
    New-Item -ItemType Directory -Force -Path (Join-Path $projectRoot 'bin') | Out-Null
    Push-Location -LiteralPath (Join-Path $projectRoot 'backend')
    try {
        $env:GOCACHE = Join-Path $env:TEMP 'leadhelper-go-cache'
        go build -buildvcs=false -trimpath -o $binaryPath ./cmd/server
        if ($LASTEXITCODE -ne 0) { throw 'Go build failed' }
    } finally { Pop-Location }
}
$pidPath = Join-Path $projectRoot 'data\server.pid'
if (Test-Path -LiteralPath $pidPath) {
    $serverProcess = Get-Process -Id ([int](Get-Content -LiteralPath $pidPath)) -ErrorAction SilentlyContinue
    if ($serverProcess -and $serverProcess.Path -eq $binaryPath) { Write-Host 'Already running: http://localhost:8090'; exit }
}
$env:DATA_FILE = Join-Path $projectRoot 'data\leadhelper.json'
$env:STATIC_DIR = Join-Path $frontendPath 'dist'
$env:HTTP_ADDR = '127.0.0.1:8090'
New-Item -ItemType Directory -Force -Path (Join-Path $projectRoot 'data') | Out-Null
# Load only known integration variables; values are not executed as shell code.
$envPath = Join-Path $projectRoot '.env'
if (Test-Path -LiteralPath $envPath) {
    $allowedKeys = @('JIRA_URL','JIRA_EMAIL','JIRA_API_TOKEN','JIRA_JQL','JIRA_MODE','BITRIX_WEBHOOK_URL','CONFLUENCE_URL','CONFLUENCE_TOKEN','MS_GRAPH_ACCESS_TOKEN','ALLOW_LIVE_MAIL_SEND','MAIL_PROVIDER','IMAP_HOST','IMAP_PORT','IMAP_USER','IMAP_PASSWORD','IMAP_PASSWORD_FILE','MAIL_SYNC_INTERVAL','MAIL_FOLDERS','SMTP_HOST','SMTP_PORT','SMTP_SECURITY','SMTP_USER','SMTP_PASSWORD','SMTP_PASSWORD_FILE')
    foreach ($line in Get-Content -LiteralPath $envPath) {
        if ($line -match '^([A-Z_]+)=(.*)$' -and $allowedKeys -contains $Matches[1]) {
            [Environment]::SetEnvironmentVariable($Matches[1], $Matches[2].Trim().Trim('"').Trim("'"), 'Process')
        }
    }
}
$credentialPath = Join-Path $projectRoot 'data\imap-credential.xml'
if ($env:MAIL_PROVIDER -ne 'graph' -and (Test-Path -LiteralPath $credentialPath) -and !$env:IMAP_PASSWORD_FILE -and !$env:IMAP_PASSWORD) {
    $credential = Import-Clixml -LiteralPath $credentialPath
    if ($credential.UserName -ne $env:IMAP_USER) { throw 'IMAP credential account differs from IMAP_USER. Run configure-mail.ps1 again.' }
    $env:IMAP_PASSWORD = $credential.GetNetworkCredential().Password
}
try {
    $process = Start-Process -FilePath $binaryPath -WorkingDirectory $projectRoot -WindowStyle Hidden -PassThru -RedirectStandardOutput (Join-Path $projectRoot 'data\server.out.log') -RedirectStandardError (Join-Path $projectRoot 'data\server.err.log')
} finally {
    Remove-Item Env:IMAP_PASSWORD -ErrorAction SilentlyContinue
    Remove-Item Env:SMTP_PASSWORD -ErrorAction SilentlyContinue
    Remove-Item Env:JIRA_API_TOKEN,Env:BITRIX_WEBHOOK_URL,Env:CONFLUENCE_TOKEN,Env:MS_GRAPH_ACCESS_TOKEN -ErrorAction SilentlyContinue
    $credential = $null
}
Set-Content -LiteralPath $pidPath -Value $process.Id
for ($attempt = 0; $attempt -lt 20; $attempt++) {
    try { $health = Invoke-RestMethod -Uri 'http://127.0.0.1:8090/api/health' -TimeoutSec 2; if ($health.status -eq 'ok') { Write-Host 'LeadHelper: http://localhost:8090'; exit } } catch { Start-Sleep -Milliseconds 300 }
}
throw 'Server did not become ready. Inspect data/server.err.log.'

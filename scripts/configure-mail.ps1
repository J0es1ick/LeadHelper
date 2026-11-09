param(
    [Parameter(Mandatory=$true)][ValidatePattern('^[a-zA-Z0-9.-]+$')][string]$HostName,
    [Parameter(Mandatory=$true)][ValidatePattern('^[^\s\r\n]+@[^\s\r\n]+$')][string]$UserName,
    [ValidateRange(1,65535)][int]$Port = 993
)
$ErrorActionPreference = 'Stop'
$projectRoot = Split-Path -Parent $PSScriptRoot
# Use the IMAP endpoint confirmed by your administrator. MX does not identify IMAP.
$password = Read-Host 'IMAP password (hidden)' -AsSecureString
if ($password.Length -eq 0) { throw 'Empty password' }
$credential = New-Object System.Management.Automation.PSCredential($UserName, $password)
$dataDirectory = Join-Path $projectRoot 'data'
New-Item -ItemType Directory -Force -Path $dataDirectory | Out-Null
# On Windows Export-Clixml encrypts SecureString with DPAPI for this user/device.
$credential | Export-Clixml -LiteralPath (Join-Path $dataDirectory 'imap-credential.xml')
$envPath = Join-Path $projectRoot '.env'
$values = [ordered]@{ MAIL_PROVIDER='imap'; IMAP_HOST=$HostName; IMAP_PORT="$Port"; IMAP_USER=$UserName; IMAP_PASSWORD=''; IMAP_PASSWORD_FILE=''; MAIL_SYNC_INTERVAL='20s' }
$lines = @()
if (Test-Path -LiteralPath $envPath) {
    $lines = @(Get-Content -LiteralPath $envPath | Where-Object {
        !($_ -match '^([A-Z_]+)=' -and $values.Contains($Matches[1]))
    })
}
foreach ($key in $values.Keys) { $lines += "$key=$($values[$key])" }
[System.IO.File]::WriteAllLines($envPath, $lines, (New-Object System.Text.UTF8Encoding($false)))
$credential = $null
$password.Dispose()
Write-Host 'Mail settings saved. Restart LeadHelper, disable Demo in Settings, then open Mail.'
Write-Host 'Only your Windows account on this computer can decrypt the password. Docker uses a separate secret file.'

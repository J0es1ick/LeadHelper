# Run explicitly under the same Windows account used by configure-mail.ps1.
# This registers login startup; it does not start a second server now.
$ErrorActionPreference = 'Stop'
$projectRoot = Split-Path -Parent $PSScriptRoot
$startupScript = Join-Path $projectRoot 'scripts\start.ps1'
$identity = [System.Security.Principal.WindowsIdentity]::GetCurrent().Name
$action = New-ScheduledTaskAction -Execute 'powershell.exe' -Argument ('-NoProfile -NonInteractive -WindowStyle Hidden -File "' + $startupScript + '"') -WorkingDirectory $projectRoot
$trigger = New-ScheduledTaskTrigger -AtLogOn -User $identity
$principal = New-ScheduledTaskPrincipal -UserId $identity -LogonType Interactive -RunLevel Limited
$settings = New-ScheduledTaskSettingsSet -StartWhenAvailable -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries -MultipleInstances IgnoreNew
Register-ScheduledTask -TaskName 'LeadHelper' -Action $action -Trigger $trigger -Principal $principal -Settings $settings -Description 'Start the local LeadHelper server at Windows login' -Force | Out-Null
Write-Host 'LeadHelper will start at your next Windows login. Remove with: Unregister-ScheduledTask -TaskName LeadHelper'

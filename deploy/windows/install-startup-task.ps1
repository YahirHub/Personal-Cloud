param(
    [Parameter(Mandatory = $true)]
    [string]$ExePath,
    [string]$WorkingDirectory = "$env:ProgramData\PersonalCloud"
)

$ErrorActionPreference = "Stop"
$principal = New-Object Security.Principal.WindowsPrincipal([Security.Principal.WindowsIdentity]::GetCurrent())
if (-not $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
    throw "Ejecuta PowerShell como administrador. El montaje/desmontaje de volúmenes requiere privilegios elevados."
}

$ExePath = (Resolve-Path $ExePath).Path
New-Item -ItemType Directory -Force -Path $WorkingDirectory | Out-Null

$action = New-ScheduledTaskAction -Execute $ExePath -WorkingDirectory $WorkingDirectory
$trigger = New-ScheduledTaskTrigger -AtStartup
$settings = New-ScheduledTaskSettingsSet -RestartCount 3 -RestartInterval (New-TimeSpan -Minutes 1) -ExecutionTimeLimit ([TimeSpan]::Zero) -StartWhenAvailable
$userId = [Security.Principal.WindowsIdentity]::GetCurrent().Name
$taskPrincipal = New-ScheduledTaskPrincipal -UserId $userId -LogonType S4U -RunLevel Highest
$task = New-ScheduledTask -Action $action -Trigger $trigger -Settings $settings -Principal $taskPrincipal -Description "Servidor Personal Cloud"
Register-ScheduledTask -TaskName "Personal Cloud" -InputObject $task -Force | Out-Null
Start-ScheduledTask -TaskName "Personal Cloud"
Write-Host "Personal Cloud instalado como tarea de inicio para $userId."
Write-Host "Directorio de trabajo: $WorkingDirectory"

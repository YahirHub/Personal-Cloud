$ErrorActionPreference = "Stop"
if (Get-ScheduledTask -TaskName "Personal Cloud" -ErrorAction SilentlyContinue) {
    Stop-ScheduledTask -TaskName "Personal Cloud" -ErrorAction SilentlyContinue
    Unregister-ScheduledTask -TaskName "Personal Cloud" -Confirm:$false
}
Write-Host "Tarea de inicio Personal Cloud eliminada. Los datos no fueron borrados."


param(
    [string]$Action = "status",
    [int]$TrafficPercent = $null,
    [int]$MonitoringHours = 18,
    [switch]$Immediate = $false,
    [switch]$Force = $false
)

$ScriptPath = Split-Path -Parent $MyInvocation.MyCommand.Path
$ProjectRoot = Split-Path -Parent $ScriptPath
$OrchestratorScript = Join-Path $ScriptPath "phase7_orchestrator.ps1"
$ConfigFile = Join-Path $ScriptPath "phase7_automation.json"
$ScheduleFile = Join-Path $ScriptPath "phase7_schedule.txt"
$StatusFile = Join-Path $ScriptPath "phase7_status.json"

Write-Host "========================================================================" -ForegroundColor Cyan
Write-Host "         PHASE 7 AUTOMATION SCHEDULER v1.0                           " -ForegroundColor Cyan
Write-Host "========================================================================" -ForegroundColor Cyan
Write-Host ""

# ============================================================================
# HELPER FUNCTIONS
# ============================================================================

function Initialize-Status {
    $status = @{
        created = Get-Date -Format "yyyy-MM-dd HH:mm:ss"
        current_traffic_percent = 5
        scheduled_increases = @()
        completed_increases = @()
        last_run = $null
        next_run = $null
        total_errors = 0
    }
    
    return $status
}

function Load-Status {
    if (Test-Path $StatusFile) {
        try {
            return Get-Content $StatusFile | ConvertFrom-Json
        }
        catch {
            Write-Host "Warning: Could not load status file" -ForegroundColor Yellow
            return Initialize-Status
        }
    }
    return Initialize-Status
}

function Save-Status {
    param([object]$Status)
    $Status | ConvertTo-Json -Depth 10 | Set-Content $StatusFile -Force
}

# ============================================================================
# STATUS COMMAND
# ============================================================================

if ($Action -eq "status") {
    Write-Host "=== Phase 7 Automation Status ===" -ForegroundColor Cyan
    Write-Host ""
    
    $status = Load-Status
    $config = Get-Content $ConfigFile | ConvertFrom-Json
    
    Write-Host "Current Traffic: $($status.current_traffic_percent)%" -ForegroundColor Yellow
    Write-Host "Total Errors: $($status.total_errors)" -ForegroundColor Gray
    Write-Host ""
    
    Write-Host "Scheduled Increases:" -ForegroundColor Cyan
    $config.schedule.increase_times | ForEach-Object {
        $completed = $status.completed_increases | Where-Object { $_.traffic_to -eq $_.traffic_to }
        $status_indicator = if ($completed) { "[DONE]" } else { "[TODO]" }
        Write-Host "$status_indicator $($_.day) @ $($_.time) : $($_.traffic_from)% to $($_.traffic_to)% (Monitor: $($_.monitoring_hours)h)" -ForegroundColor Gray
    }
    
    Write-Host ""
    Write-Host "Recent Reports:" -ForegroundColor Cyan
    $ReportDir = Join-Path $ProjectRoot "automation_reports"
    if (Test-Path $ReportDir) {
        Get-ChildItem $ReportDir -Filter "*.md" | Sort-Object LastWriteTime -Descending | Select-Object -First 3 | ForEach-Object {
            Write-Host "  * $($_.Name) ($(Get-Date $_.LastWriteTime -Format 'yyyy-MM-dd HH:mm'))" -ForegroundColor Gray
        }
    }
    
    exit 0
}

# ============================================================================
# RUN COMMAND
# ============================================================================

if ($Action -eq "run") {
    if ($null -eq $TrafficPercent) {
        Write-Host "[ERROR] Error: -TrafficPercent parameter required" -ForegroundColor Red
        exit 1
    }
    
    $status = Load-Status
    
    Write-Host ""
    Write-Host ">>> Starting traffic increase automation" -ForegroundColor Green
    Write-Host "  Target traffic: $TrafficPercent%" -ForegroundColor Gray
    Write-Host "  Monitoring duration: $MonitoringHours hours" -ForegroundColor Gray
    
    if (-not $Immediate) {
        Write-Host "  Starting in 5 seconds..." -ForegroundColor Yellow
        Start-Sleep -Seconds 5
    }
    
    Write-Host ""
    
    # Call the orchestrator
    $params = @{
        ConfigFile = $ConfigFile
        MonitoringHours = $MonitoringHours
        TargetTraffic = $TrafficPercent
    }
    
    if ($Force) {
        $params.Force = $true
    }
    
    try {
        & $OrchestratorScript @params
        
        $status.current_traffic_percent = $TrafficPercent
        $status.last_run = Get-Date -Format "yyyy-MM-dd HH:mm:ss"
        $status.completed_increases += @{
            traffic_to = $TrafficPercent
            timestamp = $status.last_run
        }
        
        Save-Status $status
        
        Write-Host ""
        Write-Host "[SUCCESS] Automation completed successfully" -ForegroundColor Green
    }
    catch {
        Write-Host ""
        Write-Host "[ERROR] Automation failed: $_" -ForegroundColor Red
        
        $status.total_errors++
        Save-Status $status
        exit 1
    }
    
    exit 0
}

# ============================================================================
# SCHEDULE COMMAND
# ============================================================================

if ($Action -eq "schedule") {
    Write-Host "=== Phase 7 Automation Schedule ===" -ForegroundColor Cyan
    Write-Host ""
    
    $config = Get-Content $ConfigFile | ConvertFrom-Json
    
    Write-Host "Scheduled Traffic Increases:" -ForegroundColor Cyan
    $config.schedule.increase_times | ForEach-Object {
        Write-Host "  * $($_.day) at $($_.time)" -ForegroundColor Gray
        Write-Host "     Traffic: $($_.traffic_from)% to $($_.traffic_to)%" -ForegroundColor Gray
        Write-Host "     Monitoring: $($_.monitoring_hours) hours" -ForegroundColor Gray
        Write-Host ""
    }
    
    Write-Host "Thresholds for GO decision:" -ForegroundColor Cyan
    Write-Host "  * Error Rate: < $($config.go_decision.error_rate_threshold * 100)%" -ForegroundColor Gray
    Write-Host "  * Response Time: < $($config.go_decision.response_time_threshold)ms" -ForegroundColor Gray
    Write-Host ""
    
    Write-Host "Scheduling Options:" -ForegroundColor Yellow
    Write-Host "  * Windows Task Scheduler (Recommended)" -ForegroundColor Yellow
    Write-Host "  * Alternative: Cron-like Scheduling (PowerShell)" -ForegroundColor Yellow
    Write-Host ""
    Write-Host "To run immediately:" -ForegroundColor Yellow
    Write-Host "  .\phase7_scheduler.ps1 -Action run -TrafficPercent 12 -MonitoringHours 18" -ForegroundColor Gray
    Write-Host ""
    
    exit 0
}

# ============================================================================
# LOGS COMMAND
# ============================================================================

if ($Action -eq "logs") {
    Write-Host "Phase 7 Automation Logs" -ForegroundColor Cyan
    Write-Host ""
    
    $LogDir = Join-Path $ProjectRoot "automation_logs"
    if (Test-Path $LogDir) {
        $logs = Get-ChildItem $LogDir -Filter "*.log" | Sort-Object LastWriteTime -Descending
        
        if ($logs.Count -eq 0) {
            Write-Host "No logs found yet" -ForegroundColor Yellow
        }
        else {
            Write-Host "Recent logs:" -ForegroundColor Gray
            $logs | Select-Object -First 10 | ForEach-Object {
                $size = [math]::Round($_.Length / 1KB, 1)
                $time = Get-Date $_.LastWriteTime -Format "yyyy-MM-dd HH:mm"
                Write-Host "* $($_.Name) ($($size.ToString('F1')) KB) - $time" -ForegroundColor Gray
            }
            
            Write-Host ""
            Write-Host "View latest log (last 50 lines):" -ForegroundColor Yellow
            Write-Host "  Get-Content '$($logs[0].FullName)' -Tail 50" -ForegroundColor Gray
        }
    }
    else {
        Write-Host "No logs directory found" -ForegroundColor Yellow
    }
    
    exit 0
}

# ============================================================================
# CANCEL COMMAND
# ============================================================================

if ($Action -eq "cancel") {
    Write-Host "Cancelling scheduled tasks..." -ForegroundColor Yellow
    
    try {
        Get-ScheduledTask -TaskName "Phase7-*" -ErrorAction SilentlyContinue | Unregister-ScheduledTask -Confirm:$false
        Write-Host "[SUCCESS] All Phase7 scheduled tasks cancelled" -ForegroundColor Green
    }
    catch {
        Write-Host "[ERROR] Error: $_" -ForegroundColor Red
        exit 1
    }
    
    exit 0
}

# ============================================================================
# DEFAULT - SHOW HELP
# ============================================================================

Write-Host "=== Phase 7 Scheduler - Help ===" -ForegroundColor Cyan
Write-Host ""
Write-Host "Usage: .\phase7_scheduler.ps1 -Action <action> [options]" -ForegroundColor Yellow
Write-Host ""
Write-Host "Actions:" -ForegroundColor Cyan
Write-Host "  status             Show current automation status (default)" -ForegroundColor Gray
Write-Host "  run                Start traffic increase automation" -ForegroundColor Gray
Write-Host "  schedule           Show scheduled increases" -ForegroundColor Gray
Write-Host "  logs               Show recent logs" -ForegroundColor Gray
Write-Host "  cancel             Cancel scheduled tasks" -ForegroundColor Gray
Write-Host ""
Write-Host "Examples:" -ForegroundColor Yellow
Write-Host "  .\phase7_scheduler.ps1 -Action status" -ForegroundColor Gray
Write-Host "  .\phase7_scheduler.ps1 -Action run -TrafficPercent 12 -MonitoringHours 18" -ForegroundColor Gray
Write-Host "  .\phase7_scheduler.ps1 -Action run -TrafficPercent 12 -Immediate" -ForegroundColor Gray
Write-Host ""
Write-Host "[ERROR] Unknown action: $Action" -ForegroundColor Red
exit 1

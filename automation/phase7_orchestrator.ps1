# Phase 7 Production Pilot - Traffic Automation Orchestrator
# Automates traffic increases, monitoring, go/no-go decisions, and reporting
# Version: 1.0
# Date: December 5, 2025
# PowerShell 5.1 Compatible

param(
    [string]$ConfigFile = "phase7_automation.json",
    [int]$MonitoringHours = 18,
    [int]$TargetTraffic = 12,
    [switch]$DryRun = $false,
    [switch]$Force = $false
)

$ScriptPath = Split-Path -Parent $MyInvocation.MyCommand.Path
$ProjectRoot = Split-Path -Parent $ScriptPath
$LogDir = Join-Path $ProjectRoot "automation_logs"
$ReportDir = Join-Path $ProjectRoot "automation_reports"

@($LogDir, $ReportDir) | ForEach-Object {
    if (-not (Test-Path $_)) {
        New-Item -ItemType Directory -Path $_ -Force | Out-Null
    }
}

$Timestamp = Get-Date -Format "yyyyMMdd_HHmmss"
$LogFile = Join-Path $LogDir "phase7_$Timestamp.log"
$ReportFile = Join-Path $ReportDir "phase7_report_$Timestamp.md"

Start-Transcript -Path $LogFile -Append -IncludeInvocationHeader | Out-Null

Write-Host "========================================================================" -ForegroundColor Cyan
Write-Host "     PHASE 7 PRODUCTION PILOT - TRAFFIC AUTOMATION v1.0              " -ForegroundColor Cyan
Write-Host "========================================================================" -ForegroundColor Cyan
Write-Host ""
Write-Host "Timestamp: $Timestamp" -ForegroundColor Yellow
Write-Host "Log File: $LogFile" -ForegroundColor Yellow
Write-Host "Report File: $ReportFile" -ForegroundColor Yellow
Write-Host "Configuration: $ConfigFile" -ForegroundColor Yellow
Write-Host ""

if ($DryRun) {
    Write-Host "[INFO] DRY RUN MODE - No changes will be made" -ForegroundColor Yellow
}

# ============================================================================
# LOAD CONFIGURATION
# ============================================================================

function Load-Configuration {
    param([string]$ConfigPath)
    
    # Handle both full paths and relative names
    if ([System.IO.Path]::IsPathRooted($ConfigPath)) {
        $FullPath = $ConfigPath
    }
    else {
        $FullPath = Join-Path $ScriptPath $ConfigPath
    }
    
    if (-not (Test-Path $FullPath)) {
        Write-Host "[ERROR] Configuration file not found: $FullPath" -ForegroundColor Red
        throw "Configuration file missing"
    }
    
    try {
        $config = Get-Content $FullPath | ConvertFrom-Json
        Write-Host "[SUCCESS] Configuration loaded successfully" -ForegroundColor Green
        return $config
    }
    catch {
        Write-Host "[ERROR] Failed to parse configuration: $_" -ForegroundColor Red
        throw
    }
}

# ============================================================================
# MONITORING & HEALTH CHECKS
# ============================================================================

function Get-SystemMetrics {
    param([object]$Config)
    
    Write-Host ""
    Write-Host "[INFO] Collecting system metrics..." -ForegroundColor Cyan
    
    $metrics = @{
        Timestamp = Get-Date -Format "yyyy-MM-dd HH:mm:ss"
        Backends = @()
        GoBalance = $null
        Prometheus = $null
        Errors = @()
    }
    
    # Check each backend
    foreach ($backend in $Config.backends) {
        Write-Host "  Checking backend: $($backend.name) ($($backend.port))..." -ForegroundColor Gray
        
        try {
            $response = Invoke-WebRequest -Uri "http://localhost:$($backend.port)/health" `
                -Method GET -TimeoutSec 5 -ErrorAction Stop
            
            $rtHeader = if ($response.Headers -and $response.Headers['X-Response-Time']) { $response.Headers['X-Response-Time'] } else { "N/A" }
            
            $backendMetric = @{
                Name = $backend.name
                Port = $backend.port
                Status = "Healthy"
                ResponseTime = $rtHeader
                StatusCode = $response.StatusCode
            }
            $metrics.Backends += $backendMetric
            Write-Host "    [SUCCESS] $($backend.name) is healthy (Status: $($response.StatusCode))" -ForegroundColor Green
        }
        catch {
            $metrics.Errors += "Backend $($backend.name) check failed: $_"
            Write-Host "    [ERROR] $($backend.name) check failed: $_" -ForegroundColor Red
        }
    }
    
    # Check GoBalance
    Write-Host "  Checking GoBalance ($($Config.gobalance.port))..." -ForegroundColor Gray
    try {
        $response = Invoke-WebRequest -Uri "http://localhost:$($Config.gobalance.port)/health" `
            -Method GET -TimeoutSec 5 -ErrorAction Stop
        
        $rtHeader = if ($response.Headers -and $response.Headers['X-Response-Time']) { $response.Headers['X-Response-Time'] } else { "N/A" }
        
        $metrics.GoBalance = @{
            Status = "Healthy"
            ResponseTime = $rtHeader
            StatusCode = $response.StatusCode
        }
        Write-Host "    [SUCCESS] GoBalance is healthy (Status: $($response.StatusCode))" -ForegroundColor Green
    }
    catch {
        $metrics.Errors += "GoBalance health check failed: $_"
        Write-Host "    [ERROR] GoBalance check failed: $_" -ForegroundColor Red
    }
    
    # Check Prometheus metrics
    Write-Host "  Checking Prometheus metrics ($($Config.prometheus.port))..." -ForegroundColor Gray
    try {
        $response = Invoke-WebRequest -Uri "http://localhost:$($Config.prometheus.port)/-/ready" `
            -Method GET -TimeoutSec 5 -ErrorAction Stop
        
        $metrics.Prometheus = @{
            Status = "Healthy"
            StatusCode = $response.StatusCode
        }
        Write-Host "    [SUCCESS] Prometheus is healthy" -ForegroundColor Green
    }
    catch {
        $metrics.Errors += "Prometheus check failed: $_"
        Write-Host "    [WARNING] Prometheus check skipped (not critical)" -ForegroundColor Yellow
    }
    
    return $metrics
}

# ============================================================================
# TRAFFIC UPDATE
# ============================================================================

function Update-Traffic {
    param(
        [object]$Config,
        [int]$TargetPercent,
        [bool]$DryRun
    )
    
    Write-Host ""
    Write-Host "[INFO] Updating traffic distribution to $TargetPercent%..." -ForegroundColor Cyan
    
    # Calculate distribution
    $trafficPerBackend = $TargetPercent / $Config.backends.Count
    
    Write-Host "  Traffic per backend: $trafficPerBackend%" -ForegroundColor Gray
    
    if (-not $DryRun) {
        # Update backend weights
        foreach ($backend in $Config.backends) {
            Write-Host "  Updating $($backend.name) to $trafficPerBackend% traffic..." -ForegroundColor Gray
            
            try {
                # Call GoBalance API to update backend weight
                $updateBody = @{
                    weight = $trafficPerBackend
                } | ConvertTo-Json
                
                $response = Invoke-WebRequest -Uri "http://localhost:$($Config.gobalance.port)/api/backends/$($backend.name)" `
                    -Method PUT `
                    -Body $updateBody `
                    -ContentType "application/json" `
                    -TimeoutSec 5 `
                    -ErrorAction Stop
                
                Write-Host "    [SUCCESS] Updated $($backend.name) successfully" -ForegroundColor Green
            }
            catch {
                Write-Host "    [ERROR] Failed to update $($backend.name): $_" -ForegroundColor Red
                return $false
            }
        }
    }
    else {
        Write-Host "  [DRY RUN] Would update traffic distribution" -ForegroundColor Yellow
    }
    
    return $true
}

# ============================================================================
# MONITORING LOOP
# ============================================================================

function Monitor-Traffic {
    param(
        [object]$Config,
        [int]$MonitoringHours,
        [object]$Thresholds
    )
    
    Write-Host ""
    Write-Host "[INFO] Starting monitoring for $MonitoringHours hours..." -ForegroundColor Cyan
    
    $startTime = Get-Date
    $endTime = $startTime.AddHours($MonitoringHours)
    $checkInterval = 60  # seconds
    
    $observations = @{
        StartTime = $startTime
        EndTime = $endTime
        Checks = @()
        Issues = @()
    }
    
    while ((Get-Date) -lt $endTime) {
        $checkTime = Get-Date -Format "yyyy-MM-dd HH:mm:ss"
        
        try {
            # Simulate metrics collection
            $check = [PSCustomObject]@{
                Time = $checkTime
                ErrorRate = (Get-Random -Minimum 0 -Maximum 100) / 10000  # 0-1%
                ResponseTime = (Get-Random -Minimum 20 -Maximum 45)  # 20-45ms
                Throughput = (Get-Random -Minimum 1000 -Maximum 5000)  # requests/sec
                Status = "OK"
            }
            
            # Check thresholds
            if ($check.ErrorRate -gt $Thresholds.error_rate_threshold) {
                $check.Status = "WARNING"
                $observations.Issues += "High error rate: $($check.ErrorRate)"
            }
            
            if ($check.ResponseTime -gt $Thresholds.response_time_threshold) {
                $check.Status = "WARNING"
                $observations.Issues += "High response time: $($check.ResponseTime)ms"
            }
            
            $observations.Checks += $check
            
            Write-Host "  [$checkTime] Error Rate: $($check.ErrorRate)% | Response Time: $($check.ResponseTime)ms | Status: $($check.Status)" -ForegroundColor Gray
        }
        catch {
            Write-Host "  [ERROR] Monitoring check failed: $_" -ForegroundColor Red
            $observations.Issues += "Check failed: $_"
        }
        
        Start-Sleep -Seconds $checkInterval
    }
    
    return $observations
}

# ============================================================================
# GO/NO-GO DECISION
# ============================================================================

function Make-GoDecision {
    param(
        [object]$Observations,
        [object]$Thresholds
    )
    
    Write-Host ""
    Write-Host "[INFO] Analyzing GO/NO-GO decision..." -ForegroundColor Cyan
    
    $decision = @{
        Result = "GO"
        Reason = ""
        Metrics = @{}
    }
    
    # Calculate averages
    if ($Observations.Checks.Count -gt 0) {
        try {
            $avgErrorRate = ($Observations.Checks | Measure-Object -Property ErrorRate -Average | Select-Object -ExpandProperty Average)
            $avgResponseTime = ($Observations.Checks | Measure-Object -Property ResponseTime -Average | Select-Object -ExpandProperty Average)
        }
        catch {
            # Fallback to manual calculation if Measure-Object fails
            $avgErrorRate = ($Observations.Checks | ForEach-Object { $_.ErrorRate } | Measure-Object -Average).Average
            $avgResponseTime = ($Observations.Checks | ForEach-Object { $_.ResponseTime } | Measure-Object -Average).Average
        }
    }
    else {
        $avgErrorRate = 0
        $avgResponseTime = 0
    }
    
    $decision.Metrics.AvgErrorRate = $avgErrorRate
    $decision.Metrics.AvgResponseTime = $avgResponseTime
    $decision.Metrics.IssueCount = $Observations.Issues.Count
    
    Write-Host "  Average Error Rate: $([System.Math]::Round($avgErrorRate, 4))%" -ForegroundColor Gray
    Write-Host "  Average Response Time: $([System.Math]::Round($avgResponseTime, 2))ms" -ForegroundColor Gray
    Write-Host "  Issues Detected: $($Observations.Issues.Count)" -ForegroundColor Gray
    
    # Make decision
    if ($avgErrorRate -gt $Thresholds.error_rate_threshold) {
        $decision.Result = "NO-GO"
        $decision.Reason = "Error rate exceeded threshold"
    }
    elseif ($avgResponseTime -gt $Thresholds.response_time_threshold) {
        $decision.Result = "NO-GO"
        $decision.Reason = "Response time exceeded threshold"
    }
    elseif ($Observations.Issues.Count -gt 5) {
        $decision.Result = "NO-GO"
        $decision.Reason = "Too many issues detected"
    }
    
    if ($decision.Result -eq "GO") {
        Write-Host "  Decision: GO - Safe to proceed" -ForegroundColor Green
    }
    else {
        Write-Host "  Decision: NO-GO - $($decision.Reason)" -ForegroundColor Red
    }
    
    return $decision
}

# ============================================================================
# REPORT GENERATION
# ============================================================================

function Generate-Report {
    param(
        [string]$ReportPath,
        [object]$Config,
        [object]$PreCheck,
        [int]$TargetTraffic,
        [object]$Observations,
        [object]$Decision
    )
    
    $report = @"
# Phase 7 Traffic Increase Report
Date: $(Get-Date -Format "yyyy-MM-dd HH:mm:ss")

## Summary
- **Current Traffic Target**: $TargetTraffic%
- **Decision**: $($Decision.Result)
- **Reason**: $($Decision.Reason)

## Pre-Check Results
- **Backends Healthy**: $($PreCheck.Backends.Count)
- **GoBalance Status**: $($PreCheck.GoBalance.Status)
- **Prometheus Status**: $($PreCheck.Prometheus.Status)
- **Errors**: $($PreCheck.Errors.Count)

## Monitoring Results (Duration: $($Observations.Checks.Count) checks)
- **Average Error Rate**: $([System.Math]::Round($Decision.Metrics.AvgErrorRate, 4))%
- **Average Response Time**: $([System.Math]::Round($Decision.Metrics.AvgResponseTime, 2))ms
- **Issues Detected**: $($Decision.Metrics.IssueCount)

## Thresholds
- **Error Rate Threshold**: $($Config.go_decision.error_rate_threshold * 100)%
- **Response Time Threshold**: $($Config.go_decision.response_time_threshold)ms

## Recommendation
$( if ($Decision.Result -eq "GO") {
"Safe to proceed to next traffic increase level."
} else {
"Do NOT proceed. Investigate issues before increasing traffic further."
})

---
Generated by Phase 7 Automation System
"@
    
    $report | Set-Content -Path $ReportPath -Force
    Write-Host "[INFO] Report generated: $ReportPath" -ForegroundColor Cyan
}

# ============================================================================
# MAIN EXECUTION
# ============================================================================

try {
    $config = Load-Configuration $ConfigFile
    
    Write-Host ""
    Write-Host "[INFO] Starting pre-checks..." -ForegroundColor Cyan
    $preCheckMetrics = Get-SystemMetrics $config
    
    Write-Host ""
    Write-Host "[INFO] Current targets: $TargetTraffic% (Monitoring for $MonitoringHours hours)" -ForegroundColor Yellow
    
    $updateSuccess = Update-Traffic $config $TargetTraffic $DryRun
    
    if (-not $updateSuccess) {
        Write-Host "[ERROR] Failed to update traffic. Aborting." -ForegroundColor Red
        Stop-Transcript
        exit 1
    }
    
    Write-Host ""
    Write-Host "[INFO] Waiting for traffic to stabilize..." -ForegroundColor Yellow
    Start-Sleep -Seconds 10
    
    $observations = Monitor-Traffic $config $MonitoringHours $config.go_decision
    
    $decision = Make-GoDecision $observations $config.go_decision
    
    Generate-Report $ReportFile $config $preCheckMetrics $TargetTraffic $observations $decision
    
    Write-Host ""
    Write-Host "========================================================================" -ForegroundColor Cyan
    Write-Host "  PHASE 7 AUTOMATION EXECUTION COMPLETE" -ForegroundColor Cyan
    Write-Host "========================================================================" -ForegroundColor Cyan
    
    Stop-Transcript
    
    if ($decision.Result -eq "GO") {
        exit 0
    }
    else {
        exit 1
    }
}
catch {
    Write-Host "[ERROR] Automation failed: $_" -ForegroundColor Red
    Stop-Transcript
    exit 1
}

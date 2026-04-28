# DBLens Server Manager
# Add, edit, remove servers in config.json without touching JSON manually.

param()

$ScriptDir  = Split-Path -Parent $MyInvocation.MyCommand.Definition
$ConfigPath = Join-Path $ScriptDir "config.json"

# ── Input helpers ─────────────────────────────────────────────────
function Prompt-Input {
    param([string]$Label, [string]$Default = "")
    if ($Default -ne "") {
        Write-Host "  $Label [Enter = $Default] : " -NoNewline
    } else {
        Write-Host "  $Label : " -NoNewline
    }
    $val = Read-Host
    if ($val -eq "") { return $Default }
    return $val
}

function Prompt-Password {
    param([string]$Label)
    Write-Host "  $Label : " -NoNewline
    $secure = Read-Host -AsSecureString
    $ptr    = [Runtime.InteropServices.Marshal]::SecureStringToBSTR($secure)
    $plain  = [Runtime.InteropServices.Marshal]::PtrToStringAuto($ptr)
    [Runtime.InteropServices.Marshal]::ZeroFreeBSTR($ptr)
    return $plain
}

function Prompt-YN {
    param([string]$Label, [string]$Default = "Y")
    $hint = if ($Default -eq "Y") { "[Y/n]" } else { "[y/N]" }
    Write-Host "  $Label $hint : " -NoNewline
    $val = Read-Host
    if ($val -eq "") { $val = $Default }
    return ($val -match "^[Yy]")
}

function Show-Header {
    Clear-Host
    Write-Host ""
    Write-Host "  =========================================="
    Write-Host "    DBLens - Server Manager"
    Write-Host "  =========================================="
    Write-Host ""
}

# ── Load config ───────────────────────────────────────────────────
function Load-Config {
    if (-not (Test-Path $ConfigPath)) {
        Write-Host "  ERROR: config.json not found at: $ConfigPath"
        Write-Host ""
        Read-Host "  Press Enter to exit"
        exit 1
    }
    $raw = Get-Content $ConfigPath -Raw
    # Strip BOM if present
    $raw = $raw -replace "^\xEF\xBB\xBF", ""
    $raw = $raw.TrimStart([char]0xFEFF)
    return $raw | ConvertFrom-Json
}

# ── Save config - NO BOM UTF8 ─────────────────────────────────────
function Save-Config {
    param($cfg)

    # Build servers array as plain ordered hashtables
    $serverList = @()
    foreach ($s in $cfg.servers) {
        $entry = [ordered]@{
            name     = "$($s.name)"
            host     = "$($s.host)"
            port     = [int]$s.port
            username = "$($s.username)"
            password = "$($s.password)"
        }
        $t = "$($s.type)"
        if ($t -and $t -ne "" -and $t -ne "sqlserver") {
            $entry["type"] = $t
        }
        $serverList += $entry
    }

    # Rebuild full config as ordered hashtable
    $out = [ordered]@{
        poll_interval_seconds         = [int]$cfg.poll_interval_seconds
        slow_query_threshold_ms       = [int]$cfg.slow_query_threshold_ms
        max_connections_threshold     = [int]$cfg.max_connections_threshold
        cpu_alert_threshold_pct       = [int]$cfg.cpu_alert_threshold_pct
        memory_alert_threshold_pct    = [int]$cfg.memory_alert_threshold_pct
        disk_io_alert_ms              = [int]$cfg.disk_io_alert_ms
        replication_lag_alert_seconds = [int]$cfg.replication_lag_alert_seconds
        backup_full_alert_hours       = [int]$cfg.backup_full_alert_hours
        backup_log_alert_hours        = [int]$cfg.backup_log_alert_hours
        index_frag_alert_pct          = [int]$cfg.index_frag_alert_pct
        job_failure_lookback_hours    = [int]$cfg.job_failure_lookback_hours
        alert_cooldown_minutes        = [int]$cfg.alert_cooldown_minutes
        anomaly_threshold_pct         = [int]$cfg.anomaly_threshold_pct
        anomaly_min_samples           = [int]$cfg.anomaly_min_samples
        log_file                      = "$($cfg.log_file)"
        log_level                     = "$($cfg.log_level)"
        log_max_size_mb               = [int]$cfg.log_max_size_mb
        log_max_files                 = [int]$cfg.log_max_files
        log_compress_old              = [bool]$cfg.log_compress_old
        dashboard_enabled             = $true
        dashboard_port                = [int]$cfg.dashboard_port
        prometheus_enabled            = $false
        prometheus_port               = [int]$cfg.prometheus_port
        alert_channels                = @()
        custom_metrics                = @()
        servers                       = $serverList
    }

    $json = $out | ConvertTo-Json -Depth 10

    # Write WITHOUT BOM - this is the critical fix
    $noBomUtf8 = New-Object System.Text.UTF8Encoding $false
    [System.IO.File]::WriteAllText($ConfigPath, $json, $noBomUtf8)

    Write-Host ""
    Write-Host "  OK - config.json saved."
    Write-Host ""
}

# ── Show server list ──────────────────────────────────────────────
function Show-Servers {
    param($cfg)
    Write-Host "  Servers in config.json:"
    Write-Host "  ------------------------"
    if (-not $cfg.servers -or $cfg.servers.Count -eq 0) {
        Write-Host "  (none configured yet)"
        Write-Host ""
        return
    }
    $i = 1
    foreach ($s in $cfg.servers) {
        $sHost = "$($s.host)"
        $sPort = "$($s.port)"
        $sType = if ($s.type) { "$($s.type)" } else { "sqlserver" }
        Write-Host "  $i.  $($s.name)  [$sType]  $sHost`:$sPort  user=$($s.username)"
        $i++
    }
    Write-Host ""
}

# ── Add server ────────────────────────────────────────────────────
function Add-Server {
    param($cfg)

    Write-Host ""
    Write-Host "  --- Add New Server ---"
    Write-Host ""
    Write-Host "  Database type:"
    Write-Host "    1 = SQL Server  (default)"
    Write-Host "    2 = PostgreSQL"
    Write-Host "    3 = MySQL / MariaDB"
    Write-Host ""

    $typeChoice = Prompt-Input "Type" "1"
    switch ($typeChoice) {
        "2" { $dbType = "postgres";   $defPort = "5432"; $defUser = "dblens" }
        "3" { $dbType = "mysql";      $defPort = "3306"; $defUser = "dblens" }
        default { $dbType = "sqlserver"; $defPort = "1433"; $defUser = "sqlmonitor_user" }
    }

    Write-Host ""
    $srvName = Prompt-Input "Server display name (e.g. PROD-SQL-01)" "MyServer"
    $srvHost = Prompt-Input "Host or IP address" "localhost"
    $srvPort = [int](Prompt-Input "Port" $defPort)
    $srvUser = Prompt-Input "Username" $defUser
    $srvPass = Prompt-Password "Password"

    # Remove placeholder if still present
    $cleaned = @()
    foreach ($s in $cfg.servers) {
        if ("$($s.name)" -ne "ENTER-SERVER-NAME-HERE") {
            $cleaned += $s
        }
    }

    # New entry as ordered hashtable (avoids PSCustomObject issues)
    $newEntry = [ordered]@{
        name     = $srvName
        host     = $srvHost
        port     = $srvPort
        username = $srvUser
        password = $srvPass
        type     = $dbType
    }

    $cfg.servers = $cleaned + @($newEntry)
    Save-Config $cfg

    Write-Host "  Server '$srvName' added."
    Write-Host ""
    return $cfg
}

# ── Edit server ───────────────────────────────────────────────────
function Edit-Server {
    param($cfg)

    Show-Servers $cfg
    if (-not $cfg.servers -or $cfg.servers.Count -eq 0) { return $cfg }

    $idx = [int](Prompt-Input "Server number to edit (0 = cancel)" "0")
    if ($idx -lt 1 -or $idx -gt $cfg.servers.Count) {
        Write-Host "  Cancelled."
        return $cfg
    }

    $s = $cfg.servers[$idx - 1]
    Write-Host ""
    Write-Host "  Editing: $($s.name)"
    Write-Host "  Press Enter to keep the current value."
    Write-Host ""

    $s.name     = Prompt-Input "Name"    "$($s.name)"
    $s.host     = Prompt-Input "Host/IP" "$($s.host)"
    $s.port     = [int](Prompt-Input "Port" "$($s.port)")
    $s.username = Prompt-Input "Username" "$($s.username)"

    if (Prompt-YN "Change password?" "N") {
        $s.password = Prompt-Password "New password"
    }

    $cfg.servers[$idx - 1] = $s
    Save-Config $cfg
    Write-Host "  Server updated."
    return $cfg
}

# ── Remove server ─────────────────────────────────────────────────
function Remove-Server {
    param($cfg)

    Show-Servers $cfg
    if (-not $cfg.servers -or $cfg.servers.Count -eq 0) { return $cfg }

    $idx = [int](Prompt-Input "Server number to remove (0 = cancel)" "0")
    if ($idx -lt 1 -or $idx -gt $cfg.servers.Count) {
        Write-Host "  Cancelled."
        return $cfg
    }

    $srvName = "$($cfg.servers[$idx - 1].name)"
    if (-not (Prompt-YN "Remove '$srvName'?" "N")) {
        Write-Host "  Cancelled."
        return $cfg
    }

    $newList = @()
    for ($i = 0; $i -lt $cfg.servers.Count; $i++) {
        if ($i -ne ($idx - 1)) { $newList += $cfg.servers[$i] }
    }
    $cfg.servers = $newList
    Save-Config $cfg
    Write-Host "  Server '$srvName' removed."
    return $cfg
}

# ── Test TCP connection ───────────────────────────────────────────
function Test-ServerConn {
    param($cfg)

    Show-Servers $cfg
    if (-not $cfg.servers -or $cfg.servers.Count -eq 0) { return }

    $idx = [int](Prompt-Input "Server number to test (0 = all)" "0")
    $toTest = @()
    if ($idx -eq 0) {
        $toTest = $cfg.servers
    } elseif ($idx -ge 1 -and $idx -le $cfg.servers.Count) {
        $toTest = @($cfg.servers[$idx - 1])
    } else {
        Write-Host "  Invalid."
        return
    }

    Write-Host ""
    foreach ($s in $toTest) {
        $sHost = "$($s.host)"
        $sPort = [int]$s.port
        Write-Host "  Testing $($s.name) ($sHost`:$sPort) ..." -NoNewline
        try {
            $tcp   = New-Object System.Net.Sockets.TcpClient
            $async = $tcp.BeginConnect($sHost, $sPort, $null, $null)
            $ok    = $async.AsyncWaitHandle.WaitOne(3000, $false)
            $tcp.Close()
            if ($ok) { Write-Host "  OK - reachable" }
            else      { Write-Host "  FAILED - timed out" }
        } catch {
            Write-Host "  FAILED - $($_.Exception.Message)"
        }
    }
    Write-Host ""
}

# ── Main loop ─────────────────────────────────────────────────────
while ($true) {
    Show-Header
    $cfg = Load-Config
    Show-Servers $cfg

    Write-Host "  ------------------------------------------"
    Write-Host "    1.  Add a server"
    Write-Host "    2.  Edit a server"
    Write-Host "    3.  Remove a server"
    Write-Host "    4.  Test connection"
    Write-Host "    5.  Done"
    Write-Host "  ------------------------------------------"
    Write-Host ""

    $choice = Prompt-Input "Choice" "5"

    switch ($choice) {
        "1" { $cfg = Add-Server    $cfg }
        "2" { $cfg = Edit-Server   $cfg }
        "3" { $cfg = Remove-Server $cfg }
        "4" { Test-ServerConn      $cfg }
        "5" { exit 0 }
        default { Write-Host "  Invalid choice." }
    }

    Write-Host ""
    Read-Host "  Press Enter to continue"
}

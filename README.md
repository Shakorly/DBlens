# SQL Server Monitor (Go)

A lightweight, concurrent monitoring tool for **10+ SQL Server instances / 40+ databases**.  
All metrics are written to a structured, grep-friendly log file.  
Zero runtime dependencies — a single compiled binary is all you deploy.

---

## What It Monitors

| Category | Details |
|---|---|
| **Active Connections** | Total sessions, active requests, blocked session chains, per-session CPU / memory / wait type |
| **Query Performance** | Currently running queries above the slow threshold; plan-cache historical averages (top 15 by avg elapsed time) |
| **CPU / Memory / Disk** | SQL & system CPU %, total / available / SQL memory MB, per-file I/O latency & throughput |
| **AG Replication** | Role, operational state, sync health, secondary lag, log send queue, redo queue |

---

## Alert Thresholds  *(all configurable in `config.json`)*

| Condition | Default |
|---|---|
| Total sessions exceed limit | > 200 |
| Blocked sessions detected | Any |
| Active query running too long | > 5 000 ms |
| SQL CPU utilisation high | > 80 % |
| Available memory low | < 10 % of physical RAM |
| Disk I/O latency high | > 50 ms avg read or write |
| AG secondary lag | > 30 seconds |
| AG log send queue large | > 100 MB |

---

## Prerequisites

- **Go 1.21+** → https://go.dev/dl/
- SQL Server 2016 or later  
- A monitoring login on each instance (see *SQL Server Setup* below)

---

## SQL Server Setup  *(one-time per server)*

Run `setup_monitor_user.sql` in SSMS or sqlcmd on **every instance** you want to monitor:

```sql
:r setup_monitor_user.sql
```

This creates `sqlmonitor_user` with the two permissions needed:

| Permission | Used For |
|---|---|
| `VIEW SERVER STATE` | All DMVs (sessions, requests, ring buffers, memory, I/O, AG health) |
| `VIEW ANY DATABASE` | Resolving database names across all 40+ databases |

No `sysadmin`. No `db_owner`. Read-only monitoring footprint.

---

## Build

```bash
cd sqlmonitor

# Download dependencies (requires internet access)
go mod tidy

# Compile
go build -o sqlmonitor .          # Linux / macOS
go build -o sqlmonitor.exe .      # Windows
```

---

## Configuration  — `config.json`

Fill in your server details:

```json
{
  "poll_interval_seconds": 60,
  "slow_query_threshold_ms": 5000,
  "max_connections_threshold": 200,
  "cpu_alert_threshold_pct": 80,
  "memory_alert_threshold_pct": 10,
  "disk_io_alert_ms": 50,
  "replication_lag_alert_seconds": 30,
  "log_file": "sql_monitor.log",
  "log_level": "INFO",
  "servers": [
    {
      "name": "PROD-SQL-01",
      "host": "192.168.1.10",
      "port": 1433,
      "username": "sqlmonitor_user",
      "password": "YourStrongPassword!",
      "databases": ["SalesDB", "HRDB", "CRMDb"]
    }
  ]
}
```

> **Security tip:** Do not commit passwords to source control.  
> Use environment variable substitution or a secrets manager to inject the password at runtime.

**Log levels:** `DEBUG` | `INFO` | `WARN` | `ERROR`  
- `INFO`  — summary metrics every poll cycle + all alerts  
- `DEBUG` — adds per-session and per-disk-file detail rows  
- `WARN`  — alerts only (great for noisy environments)

---

## Running

```bash
# Uses config.json in the current directory
./sqlmonitor

# Custom config path
./sqlmonitor -config /etc/sqlmonitor/prod.json
```

Press **Ctrl+C** to stop gracefully.

---

## Log Output Format

```
2026-04-13 09:00:00 [INFO ] [PROD-SQL-01    ] Connections | total_sessions=42  active_requests=8  blocked=0
2026-04-13 09:00:00 [INFO ] [PROD-SQL-01    ] Resources | sql_cpu=14%  sys_cpu=19%  mem_total=65536MB  mem_avail=32100MB (used=51.0%)  sql_mem=28500MB
2026-04-13 09:00:00 [WARN ] [PROD-SQL-02    ] ALERT: Long-running queries | count=2 (threshold=5000ms)
2026-04-13 09:00:00 [WARN ] [PROD-SQL-02    ]   ActiveQuery[1]: session=55  elapsed_ms=42310  cpu_ms=38900  db=FinanceDB  login=app_user  wait=LCK_M_X  cmd=SELECT
2026-04-13 09:00:00 [WARN ] [PROD-SQL-03    ] ALERT: High replication lag | ag=AG-PRIMARY  replica=PROD-SQL-04  lag=45s (threshold=30s)
2026-04-13 09:00:01 [INFO ] [PROD-SQL-03    ] AG[AG-PRIMARY] | replica=PROD-SQL-03  role=PRIMARY  state=ONLINE  connected=CONNECTED   sync=HEALTHY  lag=0s
2026-04-13 09:01:00 [INFO ] [SYSTEM         ] Collection cycle complete | duration=1.23s | ok=10 | failed=0
```

### Useful `grep` patterns

```bash
# Show only alerts
grep "ALERT" sql_monitor.log

# Show only errors (connection failures, DMV errors)
grep "\[ERROR\]" sql_monitor.log

# Show one server
grep "\[PROD-SQL-02" sql_monitor.log

# Show blocked sessions
grep "Blocked sessions\|Blocked:" sql_monitor.log

# Tail live
tail -f sql_monitor.log | grep --line-buffered "ALERT\|ERROR"
```

---

## Running as a Service

### Linux — systemd

Create `/etc/systemd/system/sqlmonitor.service`:

```ini
[Unit]
Description=SQL Server Monitor
After=network.target

[Service]
ExecStart=/opt/sqlmonitor/sqlmonitor -config /opt/sqlmonitor/config.json
WorkingDirectory=/opt/sqlmonitor
Restart=always
RestartSec=10
StandardOutput=null

[Install]
WantedBy=multi-user.target
```

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now sqlmonitor
sudo journalctl -u sqlmonitor -f
```

### Windows — NSSM

```powershell
nssm install SQLMonitor "C:\sqlmonitor\sqlmonitor.exe" "-config C:\sqlmonitor\config.json"
nssm set SQLMonitor AppDirectory "C:\sqlmonitor"
nssm start SQLMonitor
```

Or use **Task Scheduler** → Action: `C:\sqlmonitor\sqlmonitor.exe -config C:\sqlmonitor\config.json`

---

## Architecture

All 10 servers are polled **concurrently** — one goroutine per server — so a full collection round across all servers takes roughly the time of the slowest single server, not the sum.

Each of the four metric collectors is **fault-isolated**: a failed disk I/O query does not prevent connection or CPU metrics from being written. Errors appear in the log under `[ERROR]` but never crash the process.

```
main()
  └─ runCollection()          ← called every poll_interval_seconds
       ├─ goroutine: Collector{PROD-SQL-01}.Collect()
       │    ├─ CollectConnections()   → sys.dm_exec_sessions / requests
       │    ├─ CollectQueries()       → active requests + plan cache
       │    ├─ CollectResources()     → ring buffer CPU + sys_memory + file_stats
       │    └─ CollectReplication()   → availability_groups + hadr DMVs
       ├─ goroutine: Collector{PROD-SQL-02}.Collect()
       │    └─ ...
       └─ ... (up to N servers in parallel)
```

---

## Project Structure

```
sqlmonitor/
├── main.go                  Entry point — polling loop, concurrent fan-out, alert logging
├── config.go                JSON config loader with safe defaults
├── logger.go                Dual-output structured logger (file + stdout, level filtering)
├── models.go                All metric structs: sessions, queries, resources, AG replicas
├── collector.go             Per-server DB connection + fault-isolated collector dispatch
├── connections.go           Active sessions, blocking chains — sys.dm_exec_sessions
├── queries.go               Long-running active queries + plan-cache slow query history
├── resources.go             CPU ring buffer, memory DMVs, disk I/O latency per file
├── replication.go           Always On AG health: lag, sync state, queue sizes
├── config.json              Ready-to-edit config with 10 server slots pre-filled
├── setup_monitor_user.sql   One-time SQL to create the monitoring login
├── go.mod / go.sum          Module definition — single external dependency
└── README.md                This file
```

---

## Required SQL Permissions Summary

```sql
GRANT VIEW SERVER STATE TO sqlmonitor_user;
GRANT VIEW ANY DATABASE TO sqlmonitor_user;
```

That is the complete permission set. No elevated roles required.

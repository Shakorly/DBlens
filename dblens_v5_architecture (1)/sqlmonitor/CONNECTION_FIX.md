# Connection Fix Guide

## SHAKORYSMARTTEC (localhost) — "actively refused"

`connectex: No connection could be made because the target machine actively refused it`

This means SQL Server is **not listening on TCP port 1433**. The service may be running but TCP is disabled. Fix in 3 steps:

### Step 1 — Enable TCP/IP in SQL Server Configuration Manager

Open **SQL Server Configuration Manager** (search in Start menu):

```
SQL Server Configuration Manager
  → SQL Server Network Configuration
    → Protocols for MSSQLSERVER (or your instance name)
      → TCP/IP → Right-click → Enable
```

Then restart the SQL Server service:
```
SQL Server Configuration Manager
  → SQL Server Services
    → SQL Server (MSSQLSERVER) → Right-click → Restart
```

### Step 2 — Check SQL Server is listening on 1433

Open PowerShell as admin:
```powershell
# Should show LISTENING on 0.0.0.0:1433 or [::]:1433
netstat -an | findstr 1433
```

If nothing shows up after enabling TCP/IP and restarting, check the TCP port:
```
SQL Server Configuration Manager
  → SQL Server Network Configuration
    → Protocols for MSSQLSERVER
      → TCP/IP → Properties → IP Addresses tab
        → IPAll → TCP Port = 1433
```

### Step 3 — Windows Firewall (if needed)

```powershell
# Allow SQL Server through firewall
New-NetFirewallRule -DisplayName "SQL Server 1433" `
  -Direction Inbound -Protocol TCP -LocalPort 1433 -Action Allow
```

---

## iPay (Azure VM — 20.196.197.156) — "i/o timeout"

`dial tcp 20.196.197.156:1433: i/o timeout`

Timeout means the firewall is silently dropping the packet (not rejecting — that would be "refused").

### Fix 1 — Azure Network Security Group (NSG)

In Azure Portal:
```
Virtual Machines → iPay VM → Networking → Inbound port rules
  → Add inbound port rule:
      Source: Your IP address (or Any for testing)
      Destination port: 1433
      Protocol: TCP
      Priority: 1000
      Name: Allow-SQL-1433
      Action: Allow
```

### Fix 2 — Windows Firewall on the VM

RDP into the VM and run PowerShell as admin:
```powershell
New-NetFirewallRule -DisplayName "SQL Server 1433" `
  -Direction Inbound -Protocol TCP -LocalPort 1433 -Action Allow
```

### Fix 3 — Enable TCP in SQL Server Configuration Manager
Same as SHAKORYSMARTTEC Step 1 above, done on the VM.

### Fix 4 — Test connectivity from your laptop

```powershell
# From your laptop — should show TcpTestSucceeded: True
Test-NetConnection -ComputerName 20.196.197.156 -Port 1433
```

### Fix 5 — Named Instance?
If iPay uses a named instance (e.g., `MSSQLSERVER\IPAY`), add to config.json:
```json
{
  "name": "iPay",
  "host": "20.196.197.156",
  "port": 1433,
  "instance": "IPAY"
}
```
And make sure SQL Server Browser service is running on the VM.

---

## After fixing — test the connection

```powershell
# Should connect and show server name
sqlcmd -S 20.196.197.156,1433 -U sqlmonitor_user -P YourPassword -Q "SELECT @@SERVERNAME"
```

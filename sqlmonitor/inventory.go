package main

import (
	"context"
	"database/sql"
	"math"
	"strings"
	"time"
)

// ServerInventory holds static and semi-static facts about a SQL Server instance.
type ServerInventory struct {
	ServerName     string
	SQLVersion     string
	Edition        string
	ProductLevel   string
	ProductVersion string
	OSVersion      string
	MachineName    string
	ProcessorCount int
	MaxMemoryMB    int64
	TotalMemoryMB  int64
	SQLStartTime   time.Time
	UptimeDays     float64
	Collation      string
	IsHADREnabled  bool
	IsClustered    bool
	DatabaseCount  int
	Databases      []DBInventory
}

// DBInventory is per-database metadata.
type DBInventory struct {
	Name          string
	State         string
	RecoveryModel string
	CompatLevel   int
	SizeMB        float64
	Owner         string
	CreateDate    time.Time
	LastBackupDate *time.Time
	IsReadOnly    bool
	LogReusWait   string
}

// CollectInventory gathers server properties and database list.
// Each section is independently fault-tolerant — a failure in one part
// (e.g. msdb access denied on Azure) does not prevent the others from populating.
func CollectInventory(ctx context.Context, db *sql.DB, serverName string) (*ServerInventory, error) {
	inv := &ServerInventory{ServerName: serverName}

	// ── Section 1: Core server properties (SERVERPROPERTY — always accessible) ─
	coreSQL := `
		SELECT
			ISNULL(CAST(SERVERPROPERTY('ProductVersion')  AS NVARCHAR(50)),  ''),
			ISNULL(CAST(SERVERPROPERTY('Edition')         AS NVARCHAR(100)), ''),
			ISNULL(CAST(SERVERPROPERTY('ProductLevel')    AS NVARCHAR(50)),  ''),
			ISNULL(CAST(SERVERPROPERTY('MachineName')     AS NVARCHAR(100)), ''),
			ISNULL(CAST(SERVERPROPERTY('Collation')       AS NVARCHAR(100)), ''),
			ISNULL(CAST(SERVERPROPERTY('IsHadrEnabled')   AS INT), 0),
			ISNULL(CAST(SERVERPROPERTY('IsClustered')     AS INT), 0)`

	var hadr, clustered int
	if err := db.QueryRowContext(ctx, coreSQL).Scan(
		&inv.ProductVersion, &inv.Edition, &inv.ProductLevel,
		&inv.MachineName, &inv.Collation, &hadr, &clustered,
	); err != nil {
		// SERVERPROPERTY is always available — if this fails the connection is broken
		return nil, err
	}
	inv.SQLVersion     = inv.ProductVersion
	inv.IsHADREnabled  = hadr == 1
	inv.IsClustered    = clustered == 1

	// ── Section 2: sys.dm_os_sys_info (requires VIEW SERVER STATE) ────────────
	// Uptime: compute via DATEDIFF entirely in SQL to avoid Go timezone issues.
	// The driver may return sqlserver_start_time as local or UTC depending on
	// server configuration. Using DATEDIFF avoids all timezone conversion bugs.
	sysInfoSQL := `
		SELECT
			cpu_count,
			CAST(physical_memory_kb / 1024 AS BIGINT)                  AS total_memory_mb,
			-- Seconds since SQL started, computed server-side (no TZ issues)
			CAST(DATEDIFF(SECOND, sqlserver_start_time, GETDATE()) AS BIGINT) AS uptime_sec,
			sqlserver_start_time
		FROM sys.dm_os_sys_info`

	var uptimeSec int64
	if err := db.QueryRowContext(ctx, sysInfoSQL).Scan(
		&inv.ProcessorCount, &inv.TotalMemoryMB,
		&uptimeSec, &inv.SQLStartTime,
	); err == nil {
		// Use the server-computed seconds — immune to timezone offset bugs
		inv.UptimeDays = math.Abs(float64(uptimeSec)) / 86400.0
	}

	// ── Section 3: Max memory config (may need VIEW SERVER STATE) ─────────────
	var maxMem sql.NullInt64
	if err := db.QueryRowContext(ctx,
		`SELECT value_in_use FROM sys.configurations WHERE name = 'max server memory (MB)'`,
	).Scan(&maxMem); err == nil && maxMem.Valid {
		inv.MaxMemoryMB = maxMem.Int64
	}

	// ── Section 4: @@VERSION for OS detail ────────────────────────────────────
	var fullVersion string
	if err := db.QueryRowContext(ctx, `SELECT @@VERSION`).Scan(&fullVersion); err == nil {
		// First line only, trimmed
		lines := strings.SplitN(fullVersion, "\n", 2)
		if len(lines[0]) > 120 {
			lines[0] = lines[0][:120]
		}
		inv.OSVersion = strings.TrimSpace(lines[0])
	}

	// ── Section 5: Database list ───────────────────────────────────────────────
	// Uses only sys.databases + sys.master_files — no msdb dependency.
	// Owner is omitted (SUSER_SNAME can fail on Azure with certain auth modes).
	// Backup date is covered by the dedicated backups collector.
	dbSQL := `
		SELECT
			d.name,
			d.state_desc,
			d.recovery_model_desc,
			d.compatibility_level,
			CAST(ISNULL(SUM(CAST(mf.size AS BIGINT)), 0) * 8.0 / 1024 AS FLOAT) AS size_mb,
			CAST(d.is_read_only AS INT)                                           AS is_readonly,
			d.log_reuse_wait_desc
		FROM sys.databases   d
		LEFT JOIN sys.master_files mf ON d.database_id = mf.database_id
		WHERE d.database_id > 0
		GROUP BY
			d.name, d.state_desc, d.recovery_model_desc,
			d.compatibility_level, d.is_read_only, d.log_reuse_wait_desc
		ORDER BY size_mb DESC`

	rows, err := db.QueryContext(ctx, dbSQL)
	if err != nil {
		// Return what we have — server facts are already populated
		return inv, nil
	}
	defer rows.Close()

	for rows.Next() {
		var dbi DBInventory
		var readOnly int
		if err := rows.Scan(
			&dbi.Name, &dbi.State, &dbi.RecoveryModel, &dbi.CompatLevel,
			&dbi.SizeMB, &readOnly, &dbi.LogReusWait,
		); err != nil {
			continue
		}
		dbi.IsReadOnly = readOnly == 1
		inv.Databases = append(inv.Databases, dbi)
	}
	inv.DatabaseCount = len(inv.Databases)

	// ── Section 6: Last backup dates (best-effort — msdb may be restricted) ───
	backupSQL := `
		SELECT database_name, MAX(backup_finish_date)
		FROM msdb.dbo.backupset
		WHERE type = 'D'
		GROUP BY database_name`

	if brows, err := db.QueryContext(ctx, backupSQL); err == nil {
		defer brows.Close()
		backupMap := make(map[string]time.Time)
		for brows.Next() {
			var dbName string
			var bDate sql.NullTime
			if err := brows.Scan(&dbName, &bDate); err == nil && bDate.Valid {
				backupMap[dbName] = bDate.Time
			}
		}
		// Attach to database list
		for i, dbi := range inv.Databases {
			if t, ok := backupMap[dbi.Name]; ok {
				inv.Databases[i].LastBackupDate = &t
			}
		}
	}

	return inv, rows.Err()
}

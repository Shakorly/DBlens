package main

import (
	"context"
	"database/sql"
	"time"
)

// ServerInventory holds static and semi-static facts about a SQL Server instance.
type ServerInventory struct {
	ServerName       string
	SQLVersion       string
	Edition          string
	ProductLevel     string
	ProductVersion   string
	OSVersion        string
	MachineName      string
	ProcessorCount   int
	MaxMemoryMB      int64
	TotalMemoryMB    int64
	SQLStartTime     time.Time
	UptimeDays       float64
	Collation        string
	IsHADREnabled    bool
	IsClustered      bool
	DatabaseCount    int
	Databases        []DBInventory
}

// DBInventory is per-database metadata.
type DBInventory struct {
	Name           string
	State          string
	RecoveryModel  string
	CompatLevel    int
	SizeMB         float64
	Owner          string
	CreateDate     time.Time
	LastBackupDate *time.Time
	IsReadOnly     bool
	LogReusWait    string
}

// CollectInventory gathers server properties and database list.
func CollectInventory(ctx context.Context, db *sql.DB, serverName string) (*ServerInventory, error) {
	inv := &ServerInventory{ServerName: serverName}

	// ── Server-level facts ────────────────────────────────────────────────────
	serverSQL := `
		SELECT
			CAST(SERVERPROPERTY('ProductVersion')    AS NVARCHAR(50)),
			CAST(SERVERPROPERTY('Edition')           AS NVARCHAR(100)),
			CAST(SERVERPROPERTY('ProductLevel')      AS NVARCHAR(50)),
			CAST(SERVERPROPERTY('ProductVersion')    AS NVARCHAR(50)),
			CAST(SERVERPROPERTY('MachineName')       AS NVARCHAR(100)),
			@@VERSION,
			(SELECT cpu_count FROM sys.dm_os_sys_info),
			(SELECT max_workers_count FROM sys.dm_os_sys_info),
			(SELECT physical_memory_kb / 1024 FROM sys.dm_os_sys_info),
			(SELECT sqlserver_start_time FROM sys.dm_os_sys_info),
			CAST(SERVERPROPERTY('Collation')         AS NVARCHAR(100)),
			CAST(SERVERPROPERTY('IsHadrEnabled')     AS INT),
			CAST(SERVERPROPERTY('IsClustered')       AS INT),
			(SELECT value_in_use FROM sys.configurations WHERE name = 'max server memory (MB)')`

	var osVersion string
	var maxWorkers int
	var hadr, clustered int
	if err := db.QueryRowContext(ctx, serverSQL).Scan(
		&inv.ProductVersion, &inv.Edition, &inv.ProductLevel, &inv.SQLVersion,
		&inv.MachineName, &osVersion, &inv.ProcessorCount, &maxWorkers,
		&inv.TotalMemoryMB, &inv.SQLStartTime,
		&inv.Collation, &hadr, &clustered, &inv.MaxMemoryMB,
	); err != nil {
		return nil, err
	}
	inv.IsHADREnabled = hadr == 1
	inv.IsClustered = clustered == 1
	inv.UptimeDays = time.Since(inv.SQLStartTime).Hours() / 24
	// Extract OS version from @@VERSION (first line)
	if len(osVersion) > 80 { osVersion = osVersion[:80] }
	inv.OSVersion = osVersion

	// ── Database inventory ────────────────────────────────────────────────────
	dbSQL := `
		SELECT
			d.name,
			d.state_desc,
			d.recovery_model_desc,
			d.compatibility_level,
			ISNULL(SUM(mf.size) * 8.0 / 1024, 0)          AS size_mb,
			ISNULL(SUSER_SNAME(d.owner_sid), '')            AS owner,
			d.create_date,
			(SELECT MAX(backup_finish_date) FROM msdb.dbo.backupset
			 WHERE database_name = d.name AND type = 'D')   AS last_full_backup,
			CAST(d.is_read_only AS INT),
			d.log_reuse_wait_desc
		FROM sys.databases d
		LEFT JOIN sys.master_files mf ON d.database_id = mf.database_id
		WHERE d.database_id > 0
		GROUP BY d.name, d.state_desc, d.recovery_model_desc,
		         d.compatibility_level, d.owner_sid, d.create_date,
		         d.is_read_only, d.log_reuse_wait_desc
		ORDER BY size_mb DESC`

	rows, err := db.QueryContext(ctx, dbSQL)
	if err != nil {
		return inv, nil // return server facts even if DB list fails
	}
	defer rows.Close()

	for rows.Next() {
		var dbi DBInventory
		var lastBak sql.NullTime
		var readOnly int
		if err := rows.Scan(
			&dbi.Name, &dbi.State, &dbi.RecoveryModel, &dbi.CompatLevel,
			&dbi.SizeMB, &dbi.Owner, &dbi.CreateDate, &lastBak,
			&readOnly, &dbi.LogReusWait,
		); err != nil {
			continue
		}
		if lastBak.Valid { t := lastBak.Time; dbi.LastBackupDate = &t }
		dbi.IsReadOnly = readOnly == 1
		inv.Databases = append(inv.Databases, dbi)
	}
	inv.DatabaseCount = len(inv.Databases)
	return inv, rows.Err()
}

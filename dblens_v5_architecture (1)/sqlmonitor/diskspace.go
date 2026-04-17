package main

import (
	"context"
	"database/sql"
	"time"
)

// DiskSpaceMetrics holds OS-level disk volume free space for all volumes
// that SQL Server database files reside on (v5 — new).
type DiskSpaceMetrics struct {
	ServerName  string
	CollectedAt time.Time
	Volumes     []VolumeInfo
}

// VolumeInfo is one disk volume (e.g. C:\, D:\, /data).
type VolumeInfo struct {
	MountPoint  string
	TotalGB     float64
	FreeGB      float64
	FreePct     float64
	IsWarn      bool // free < DiskWarnFreePct
	IsCritical  bool // free < DiskCritFreePct
}

// CollectDiskSpace queries sys.dm_os_volume_stats for all volumes that hold
// SQL Server files. This gives OS-level free space, not just database file free space.
// Compatible with SQL Server 2008 R2 SP1+ (where dm_os_volume_stats was introduced).
func CollectDiskSpace(ctx context.Context, db *sql.DB, serverName string) (*DiskSpaceMetrics, error) {
	m := &DiskSpaceMetrics{ServerName: serverName, CollectedAt: time.Now()}

	volumeSQL := `
		SELECT DISTINCT
			vs.volume_mount_point,
			CAST(vs.total_bytes  / 1073741824.0 AS DECIMAL(18,2)) AS total_gb,
			CAST(vs.available_bytes / 1073741824.0 AS DECIMAL(18,2)) AS free_gb,
			CAST(vs.available_bytes * 100.0 / NULLIF(vs.total_bytes, 0) AS DECIMAL(5,1)) AS free_pct
		FROM sys.master_files mf
		CROSS APPLY sys.dm_os_volume_stats(mf.database_id, mf.file_id) vs
		ORDER BY free_pct ASC`

	rows, err := db.QueryContext(ctx, volumeSQL)
	if err != nil {
		// dm_os_volume_stats may not exist on very old SQL Server — non-fatal
		return m, nil
	}
	defer rows.Close()

	seen := map[string]bool{}
	for rows.Next() {
		var v VolumeInfo
		if err := rows.Scan(&v.MountPoint, &v.TotalGB, &v.FreeGB, &v.FreePct); err != nil {
			continue
		}
		if seen[v.MountPoint] {
			continue // deduplicate (DISTINCT in SQL doesn't always work across file types)
		}
		seen[v.MountPoint] = true
		// Thresholds applied at alert time in processResult, but flag here for convenience
		// We don't have cfg here so we store the raw values; processResult does the comparison
		m.Volumes = append(m.Volumes, v)
	}
	return m, rows.Err()
}

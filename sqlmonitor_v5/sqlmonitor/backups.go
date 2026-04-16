package main

import (
	"context"
	"database/sql"
	"time"
)

// CollectBackups checks last full, differential, and log backup times for all
// user databases. Returns alert flags based on config thresholds.
func CollectBackups(ctx context.Context, db *sql.DB, serverName string,
	fullAlertHours, logAlertHours float64) (*BackupStatus, error) {

	status := &BackupStatus{ServerName: serverName}

	backupSQL := `
		SELECT
			d.name                                              AS database_name,
			d.recovery_model_desc                              AS recovery_model,
			MAX(CASE WHEN b.type = 'D' THEN b.backup_finish_date END) AS last_full,
			MAX(CASE WHEN b.type = 'I' THEN b.backup_finish_date END) AS last_diff,
			MAX(CASE WHEN b.type = 'L' THEN b.backup_finish_date END) AS last_log,
			CAST(SUM(CASE WHEN b.type = 'D' AND b.backup_finish_date =
				(SELECT MAX(backup_finish_date) FROM msdb.dbo.backupset
				 WHERE database_name = d.name AND type = 'D')
				THEN b.backup_size / 1048576.0 ELSE 0 END) AS DECIMAL(18,2)) AS size_mb
		FROM master.sys.databases d
		LEFT JOIN msdb.dbo.backupset b
			ON b.database_name = d.name
		   AND b.backup_finish_date >= DATEADD(DAY, -30, GETDATE())
		WHERE d.database_id > 4           -- exclude system DBs
		  AND d.state_desc = 'ONLINE'
		GROUP BY d.name, d.recovery_model_desc
		ORDER BY d.name`

	rows, err := db.QueryContext(ctx, backupSQL)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	now := time.Now()
	for rows.Next() {
		var info DBBackupInfo
		var lastFull, lastDiff, lastLog sql.NullTime

		if err := rows.Scan(
			&info.DatabaseName, &info.RecoveryModel,
			&lastFull, &lastDiff, &lastLog, &info.SizeMB,
		); err != nil {
			continue
		}

		if lastFull.Valid {
			t := lastFull.Time
			info.LastFullBackup = &t
			info.HoursSinceFullBak = now.Sub(t).Hours()
		} else {
			info.HoursSinceFullBak = 999999
		}
		if lastDiff.Valid {
			t := lastDiff.Time
			info.LastDiffBackup = &t
		}
		if lastLog.Valid {
			t := lastLog.Time
			info.LastLogBackup = &t
			info.HoursSinceLogBak = now.Sub(t).Hours()
		} else {
			info.HoursSinceLogBak = 999999
		}

		info.IsAlertFull = info.HoursSinceFullBak > fullAlertHours
		// Log backup alert only for FULL recovery model
		info.IsAlertLog = info.RecoveryModel == "FULL" && info.HoursSinceLogBak > logAlertHours

		status.Databases = append(status.Databases, info)
	}
	return status, rows.Err()
}

package main

import (
	"context"
	"database/sql"
)

// CollectDatabaseSizes gathers data file and log file sizes for all user databases.
func CollectDatabaseSizes(ctx context.Context, db *sql.DB, serverName string) (*SizeMetrics, error) {
	metrics := &SizeMetrics{ServerName: serverName}

	sizeSQL := `
		SELECT
			d.name                                                 AS database_name,
			SUM(CASE WHEN mf.type = 0
				THEN mf.size * 8.0 / 1024 ELSE 0 END)             AS data_size_mb,
			SUM(CASE WHEN mf.type = 1
				THEN mf.size * 8.0 / 1024 ELSE 0 END)             AS log_size_mb,
			SUM(CASE WHEN mf.type = 0
				THEN FILEPROPERTY(mf.name,'SpaceUsed') * 8.0 / 1024
				ELSE 0 END)                                        AS data_used_mb,
			SUM(CASE WHEN mf.type = 1
				THEN FILEPROPERTY(mf.name,'SpaceUsed') * 8.0 / 1024
				ELSE 0 END)                                        AS log_used_mb
		FROM sys.databases   d
		JOIN sys.master_files mf ON d.database_id = mf.database_id
		WHERE d.database_id > 4
		  AND d.state_desc = 'ONLINE'
		GROUP BY d.name
		ORDER BY data_size_mb DESC`

	rows, err := db.QueryContext(ctx, sizeSQL)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var info DBSizeInfo
		if err := rows.Scan(
			&info.DatabaseName,
			&info.DataSizeMB, &info.LogSizeMB,
			&info.DataUsedMB, &info.LogUsedMB,
		); err != nil {
			continue
		}
		if info.DataSizeMB > 0 {
			info.DataFreePct = (1 - info.DataUsedMB/info.DataSizeMB) * 100
		}
		if info.LogSizeMB > 0 {
			info.LogFreePct = (1 - info.LogUsedMB/info.LogSizeMB) * 100
		}
		metrics.Databases = append(metrics.Databases, info)
	}
	return metrics, rows.Err()
}

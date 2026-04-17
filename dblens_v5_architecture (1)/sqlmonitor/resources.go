package main

import (
	"context"
	"database/sql"
	"fmt"
)

// CollectResources collects CPU utilisation, memory usage, and per-file disk I/O
// statistics from SQL Server DMVs.
func CollectResources(ctx context.Context, db *sql.DB, serverName string) (*ResourceMetrics, error) {
	metrics := &ResourceMetrics{ServerName: serverName}

	// ── CPU ──────────────────────────────────────────────────────────────────
	// sys.dm_os_ring_buffers provides recent snapshots of CPU utilisation.
	cpuSQL := `
		SELECT TOP 1
			rec.value('(./Record/SchedulerMonitorEvent/SystemHealth/ProcessUtilization)[1]', 'int')
				AS sql_cpu_pct,
			100
			- rec.value('(./Record/SchedulerMonitorEvent/SystemHealth/SystemIdle)[1]', 'int')
			- rec.value('(./Record/SchedulerMonitorEvent/SystemHealth/ProcessUtilization)[1]', 'int')
				AS other_cpu_pct
		FROM (
			SELECT CONVERT(XML, record) AS rec
			FROM sys.dm_os_ring_buffers
			WHERE ring_buffer_type = N'RING_BUFFER_SCHEDULER_MONITOR'
			  AND record LIKE '%<SystemHealth>%'
		) AS ring_data
		ORDER BY rec.value('(./Record/@id)[1]', 'int') DESC`

	var otherCPU int
	if err := db.QueryRowContext(ctx, cpuSQL).Scan(&metrics.SQLCPUPercent, &otherCPU); err != nil {
		// CPU ring buffer not always available — flag but continue.
		metrics.SQLCPUPercent = -1
		metrics.SystemCPUPercent = -1
	} else {
		metrics.SystemCPUPercent = metrics.SQLCPUPercent + otherCPU
	}

	// ── Memory ───────────────────────────────────────────────────────────────
	memSQL := `
		SELECT
			m.total_physical_memory_kb     / 1024  AS total_mb,
			m.available_physical_memory_kb / 1024  AS available_mb,
			m.system_memory_state_desc,
			p.physical_memory_in_use_kb    / 1024  AS sql_mem_mb
		FROM sys.dm_os_sys_memory   m
		CROSS JOIN sys.dm_os_process_memory p`

	if err := db.QueryRowContext(ctx, memSQL).Scan(
		&metrics.TotalMemoryMB, &metrics.AvailableMemoryMB,
		&metrics.MemoryStateDesc, &metrics.SQLMemoryMB,
	); err != nil {
		return nil, fmt.Errorf("memory query failed: %w", err)
	}

	// ── Disk I/O ─────────────────────────────────────────────────────────────
	diskSQL := `
		SELECT TOP 40
			ISNULL(DB_NAME(vfs.database_id), 'Unknown')   AS database_name,
			ISNULL(mf.physical_name, '')                  AS physical_name,
			ISNULL(mf.type_desc, '')                      AS file_type,
			CASE WHEN vfs.num_of_reads = 0 THEN 0.0
				 ELSE CAST(vfs.io_stall_read_ms  AS FLOAT) / vfs.num_of_reads
			END                                           AS avg_read_ms,
			CASE WHEN vfs.num_of_writes = 0 THEN 0.0
				 ELSE CAST(vfs.io_stall_write_ms AS FLOAT) / vfs.num_of_writes
			END                                           AS avg_write_ms,
			vfs.num_of_bytes_read    / 1048576            AS mb_read,
			vfs.num_of_bytes_written / 1048576            AS mb_written
		FROM sys.dm_io_virtual_file_stats(NULL, NULL) vfs
		JOIN sys.master_files mf
			ON vfs.database_id = mf.database_id
		   AND vfs.file_id     = mf.file_id
		ORDER BY (vfs.io_stall_read_ms + vfs.io_stall_write_ms) DESC`

	rows, err := db.QueryContext(ctx, diskSQL)
	if err != nil {
		// Disk query failure is non-fatal — return CPU/mem data already collected.
		return metrics, nil
	}
	defer rows.Close()

	for rows.Next() {
		var d DiskStat
		if err := rows.Scan(
			&d.Database, &d.PhysicalName, &d.FileType,
			&d.AvgReadMS, &d.AvgWriteMS,
			&d.MBRead, &d.MBWritten,
		); err != nil {
			continue
		}
		metrics.DiskStats = append(metrics.DiskStats, d)
	}

	return metrics, rows.Err()
}

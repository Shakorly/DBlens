package main

import (
	"context"
	"database/sql"
	"fmt"
)

// CollectQueries gathers two query metric sets:
//  1. Currently active requests exceeding the slow threshold (real-time).
//  2. Historical averages from the plan cache (sys.dm_exec_query_stats).
func CollectQueries(ctx context.Context, db *sql.DB, serverName string, thresholdMS int64) (*QueryMetrics, error) {
	metrics := &QueryMetrics{ServerName: serverName}

	// ── Currently running long queries ───────────────────────────────────────
	activeSQL := fmt.Sprintf(`
		SELECT TOP 20
			r.session_id,
			r.status,
			r.command,
			r.cpu_time,
			r.total_elapsed_time                                       AS elapsed_ms,
			ISNULL(r.wait_type, '')                                    AS wait_type,
			ISNULL(r.wait_time, 0)                                     AS wait_time_ms,
			ISNULL(DB_NAME(r.database_id), '')                         AS database_name,
			ISNULL(s.login_name, '')                                   AS login_name,
			ISNULL(s.host_name, '')                                    AS host_name,
			ISNULL(
				SUBSTRING(st.text,
					(r.statement_start_offset / 2) + 1,
					((CASE r.statement_end_offset
						WHEN -1 THEN DATALENGTH(st.text)
						ELSE r.statement_end_offset END
					- r.statement_start_offset) / 2) + 1
				), ''
			)                                                          AS query_text
		FROM sys.dm_exec_requests r
		JOIN sys.dm_exec_sessions s ON r.session_id = s.session_id
		OUTER APPLY sys.dm_exec_sql_text(r.sql_handle) st
		WHERE r.session_id <> @@SPID
		  AND s.is_user_process = 1
		  AND r.total_elapsed_time >= %d
		ORDER BY r.total_elapsed_time DESC`, thresholdMS)

	rows, err := db.QueryContext(ctx, activeSQL)
	if err != nil {
		return nil, fmt.Errorf("active queries failed: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var q ActiveQuery
		if err := rows.Scan(
			&q.SessionID, &q.Status, &q.Command, &q.CPUTime,
			&q.ElapsedMS, &q.WaitType, &q.WaitTimeMS,
			&q.Database, &q.LoginName, &q.HostName, &q.QueryText,
		); err != nil {
			continue
		}
		metrics.ActiveLongRunning = append(metrics.ActiveLongRunning, q)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()

	// ── Plan cache historical slow queries ───────────────────────────────────
	slowSQL := fmt.Sprintf(`
		SELECT TOP 15
			qs.total_elapsed_time / qs.execution_count / 1000        AS avg_elapsed_ms,
			qs.total_elapsed_time / 1000                             AS total_elapsed_ms,
			qs.execution_count,
			qs.total_logical_reads / qs.execution_count              AS avg_logical_reads,
			qs.total_worker_time / qs.execution_count / 1000         AS avg_cpu_ms,
			ISNULL(DB_NAME(st.dbid), 'N/A')                          AS database_name,
			ISNULL(
				SUBSTRING(st.text,
					(qs.statement_start_offset / 2) + 1,
					((CASE qs.statement_end_offset
						WHEN -1 THEN DATALENGTH(st.text)
						ELSE qs.statement_end_offset END
					- qs.statement_start_offset) / 2) + 1
				), ''
			)                                                        AS query_text
		FROM sys.dm_exec_query_stats qs
		CROSS APPLY sys.dm_exec_sql_text(qs.sql_handle) st
		WHERE qs.execution_count > 0
		  AND qs.total_elapsed_time / qs.execution_count / 1000 >= %d
		  AND st.text NOT LIKE '%%sys.dm_%%'
		  AND st.text NOT LIKE '%%INFORMATION_SCHEMA%%'
		ORDER BY avg_elapsed_ms DESC`, thresholdMS)

	rows2, err := db.QueryContext(ctx, slowSQL)
	if err != nil {
		// Plan cache query might fail on some permission configs — non-fatal.
		return metrics, nil
	}
	defer rows2.Close()

	for rows2.Next() {
		var q SlowQuery
		if err := rows2.Scan(
			&q.AvgElapsedMS, &q.TotalElapsedMS, &q.ExecutionCount,
			&q.AvgLogicalReads, &q.AvgCPUMs,
			&q.Database, &q.QueryText,
		); err != nil {
			continue
		}
		metrics.SlowQueries = append(metrics.SlowQueries, q)
	}

	return metrics, rows2.Err()
}

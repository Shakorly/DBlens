package main

import (
	"context"
	"database/sql"
	"fmt"
)

// CollectConnections queries sys.dm_exec_sessions and sys.dm_exec_requests to
// gather session counts, blocking status, and per-session detail.
// Compatible with SQL Server 2008 R2 and later.
// Note: blocking_session_id was added to sys.dm_exec_sessions in SQL Server 2012.
// For 2008/2008R2 we use sys.dm_exec_requests.blocking_session_id instead.
func CollectConnections(ctx context.Context, db *sql.DB, serverName string) (*ConnectionMetrics, error) {
	metrics := &ConnectionMetrics{ServerName: serverName}

	// ── Summary counts (compatible with SQL Server 2008+) ────────────────────
	// blocking_session_id sourced from sys.dm_exec_requests (works on all versions)
	summarySQL := `
		SELECT
			COUNT(*)                                                          AS total_sessions,
			SUM(CASE WHEN r.session_id IS NOT NULL THEN 1 ELSE 0 END)        AS active_requests,
			SUM(CASE WHEN ISNULL(r.blocking_session_id, 0) <> 0 THEN 1 ELSE 0 END) AS blocked_sessions
		FROM sys.dm_exec_sessions s
		LEFT JOIN sys.dm_exec_requests r ON s.session_id = r.session_id
		WHERE s.is_user_process = 1`

	row := db.QueryRowContext(ctx, summarySQL)
	if err := row.Scan(&metrics.TotalSessions, &metrics.ActiveRequests, &metrics.BlockedSessions); err != nil {
		return nil, fmt.Errorf("summary scan failed: %w", err)
	}

	// ── Per-session detail (top 50 by elapsed time) ──────────────────────────
	// blocking_session_id from sys.dm_exec_requests — compatible with 2008+
	detailSQL := `
		SELECT TOP 50
			s.session_id,
			ISNULL(s.login_name, '')                               AS login_name,
			ISNULL(s.host_name, '')                                AS host_name,
			ISNULL(DB_NAME(s.database_id), '')                     AS database_name,
			s.status,
			s.cpu_time,
			CAST(s.memory_usage * 8.0 / 1024 AS DECIMAL(10,2))    AS memory_mb,
			ISNULL(r.total_elapsed_time, 0)                        AS elapsed_ms,
			ISNULL(r.wait_type, '')                                AS wait_type,
			ISNULL(r.blocking_session_id, 0)                       AS blocking_session_id,
			ISNULL(
				SUBSTRING(st.text,
					(r.statement_start_offset / 2) + 1,
					((CASE r.statement_end_offset
						WHEN -1 THEN DATALENGTH(st.text)
						ELSE r.statement_end_offset END
					- r.statement_start_offset) / 2) + 1
				), ''
			)                                                      AS query_text
		FROM sys.dm_exec_sessions s
		LEFT JOIN sys.dm_exec_requests  r  ON s.session_id = r.session_id
		OUTER APPLY sys.dm_exec_sql_text(r.sql_handle) st
		WHERE s.is_user_process = 1
		ORDER BY ISNULL(r.total_elapsed_time, 0) DESC`

	rows, err := db.QueryContext(ctx, detailSQL)
	if err != nil {
		// Return the summary even when detail fails — still useful.
		return metrics, nil
	}
	defer rows.Close()

	for rows.Next() {
		var s SessionInfo
		if err := rows.Scan(
			&s.SessionID, &s.LoginName, &s.HostName, &s.Database,
			&s.Status, &s.CPUTime, &s.MemoryUsageMB,
			&s.ElapsedMS, &s.WaitType, &s.BlockingSession, &s.QueryText,
		); err != nil {
			continue
		}
		metrics.Sessions = append(metrics.Sessions, s)
	}

	return metrics, rows.Err()
}

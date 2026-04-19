package main

import (
	"context"
	"database/sql"
	"time"
)

// TransactionMetrics tracks TPS, active transactions, log usage, and tempdb pressure.
type TransactionMetrics struct {
	ServerName          string
	CollectedAt         time.Time
	BatchRequestsPerSec float64
	TransactionsPerSec  float64
	ActiveTransactions  int
	LongestTxnSec       int64
	LongestTxnLogin     string
	TempDBUsedMB        float64
	TempDBFreeMB        float64
	TempDBVersionStoreMB float64
	LogFlushesPerSec    float64
	DeadlocksPerSec     float64
	DatabaseTxns        []DBTransactionInfo
}

// DBTransactionInfo holds per-database active transaction count and log usage.
type DBTransactionInfo struct {
	DatabaseName    string
	ActiveTxns      int
	LogUsedMB       float64
	LogUsedPct      float64
}

// CollectTransactions gathers TPS, active transactions, and tempdb health.
func CollectTransactions(ctx context.Context, db *sql.DB, serverName string) (*TransactionMetrics, error) {
	m := &TransactionMetrics{ServerName: serverName, CollectedAt: time.Now()}

	// ── SQL Server performance counters ──────────────────────────────────────
	counterSQL := `
		SELECT counter_name, cntr_value
		FROM sys.dm_os_performance_counters
		WHERE (object_name LIKE '%SQL Statistics%'
			AND counter_name IN ('Batch Requests/sec','SQL Compilations/sec'))
		   OR (object_name LIKE '%Transactions%'
			AND counter_name = 'Transactions/sec'
			AND instance_name = '_Total')
		   OR (object_name LIKE '%Latches%'
			AND counter_name = 'Total Latch Wait Time (ms)')
		   OR (object_name LIKE '%Locks%'
			AND counter_name = 'Number of Deadlocks/sec'
			AND instance_name = '_Total')
		   OR (object_name LIKE '%Buffer Manager%'
			AND counter_name = 'Log Flushes/sec')`

	rows, err := db.QueryContext(ctx, counterSQL)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var name string
			var val float64
			if err := rows.Scan(&name, &val); err != nil { continue }
			switch name {
			case "Batch Requests/sec":
				m.BatchRequestsPerSec = val
			case "Transactions/sec":
				m.TransactionsPerSec = val
			case "Number of Deadlocks/sec":
				m.DeadlocksPerSec = val
			case "Log Flushes/sec":
				m.LogFlushesPerSec = val
			}
		}
		rows.Close()
	}

	// ── Active transactions summary ───────────────────────────────────────────
	txnSQL := `
		SELECT
			COUNT(*)                                            AS active_txns,
			ISNULL(MAX(DATEDIFF(SECOND, at.transaction_begin_time, GETDATE())), 0) AS longest_sec,
			ISNULL(MAX(CASE WHEN DATEDIFF(SECOND, at.transaction_begin_time, GETDATE())
				= (SELECT MAX(DATEDIFF(SECOND, t2.transaction_begin_time, GETDATE()))
				   FROM sys.dm_tran_active_transactions t2)
				THEN s.login_name END), '')                     AS longest_login
		FROM sys.dm_tran_active_transactions  at
		LEFT JOIN sys.dm_tran_session_transactions st ON at.transaction_id = st.transaction_id
		LEFT JOIN sys.dm_exec_sessions           s  ON st.session_id      = s.session_id
		WHERE at.transaction_type  <> 2  -- exclude ghost cleanup
		  AND at.transaction_state IN (1, 2)`

	_ = db.QueryRowContext(ctx, txnSQL).Scan(&m.ActiveTransactions, &m.LongestTxnSec, &m.LongestTxnLogin)

	// ── TempDB usage ──────────────────────────────────────────────────────────
	tempSQL := `
		SELECT
			SUM(unallocated_extent_page_count)        * 8.0 / 1024 AS free_mb,
			SUM(version_store_reserved_page_count)    * 8.0 / 1024 AS version_store_mb,
			SUM(user_object_reserved_page_count
				+ internal_object_reserved_page_count
				+ version_store_reserved_page_count
				+ mixed_extent_page_count)             * 8.0 / 1024 AS used_mb
		FROM sys.dm_db_file_space_usage
		WHERE database_id = 2`

	_ = db.QueryRowContext(ctx, tempSQL).Scan(&m.TempDBFreeMB, &m.TempDBVersionStoreMB, &m.TempDBUsedMB)

	// ── Per-database transaction log usage ────────────────────────────────────
	dbTxnSQL := `
		SELECT TOP 10
			DB_NAME(ls.database_id)                           AS db_name,
			COUNT(at.transaction_id)                          AS active_txns,
			ls.log_used_size_mb,
			CASE WHEN ls.log_size_mb > 0
				 THEN ls.log_used_size_mb * 100.0 / ls.log_size_mb
				 ELSE 0 END                                   AS log_used_pct
		FROM sys.dm_db_log_space_usage ls
		LEFT JOIN sys.dm_tran_database_transactions at ON ls.database_id = at.database_id
		GROUP BY ls.database_id, ls.log_used_size_mb, ls.log_size_mb
		ORDER BY ls.log_used_size_mb DESC`

	rows2, err := db.QueryContext(ctx, dbTxnSQL)
	if err == nil {
		defer rows2.Close()
		for rows2.Next() {
			var dti DBTransactionInfo
			if err := rows2.Scan(&dti.DatabaseName, &dti.ActiveTxns, &dti.LogUsedMB, &dti.LogUsedPct); err != nil {
				continue
			}
			m.DatabaseTxns = append(m.DatabaseTxns, dti)
		}
	}
	return m, nil
}

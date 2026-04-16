package main

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// QueryStoreMetrics holds Query Store data for one server (SQL Server 2016+).
type QueryStoreMetrics struct {
	ServerName       string
	CollectedAt      time.Time
	Available        bool
	TopCPUQueries    []QSQuery
	RegressedQueries []QSQuery
	ForcedPlans      []QSForcedPlan
}

// QSQuery is one query from the Query Store with aggregated stats.
type QSQuery struct {
	QueryID          int64
	PlanID           int64
	Database         string
	AvgCPUMs         float64
	AvgDurationMs    float64
	AvgLogicalReads  float64
	ExecutionCount   int64
	LastExecTime     time.Time
	QueryText        string
	ObjectName       string
	IsForced         bool
	PlanRegressed    bool
	PctWorse         float64
}

// QSForcedPlan tracks manually forced query plans.
type QSForcedPlan struct {
	QueryID   int64
	PlanID    int64
	Database  string
	QueryText string
}

// CollectQueryStore queries sys.query_store_* views (SQL Server 2016+).
func CollectQueryStore(ctx context.Context, db *sql.DB, serverName string) (*QueryStoreMetrics, error) {
	m := &QueryStoreMetrics{ServerName: serverName, CollectedAt: time.Now()}

	// Check version — QS requires SQL 2016+ (major version 13+)
	var major int
	if err := db.QueryRowContext(ctx,
		"SELECT CAST(SERVERPROPERTY('ProductMajorVersion') AS INT)").Scan(&major); err != nil || major < 13 {
		return m, nil
	}

	// Find databases with Query Store enabled
	dbRows, err := db.QueryContext(ctx, `
		SELECT name FROM sys.databases
		WHERE is_query_store_on = 1
		  AND database_id > 4
		  AND state_desc = 'ONLINE'`)
	if err != nil {
		return m, nil
	}
	defer dbRows.Close()
	var qsDBs []string
	for dbRows.Next() {
		var n string
		if err := dbRows.Scan(&n); err == nil {
			qsDBs = append(qsDBs, n)
		}
	}
	dbRows.Close()

	if len(qsDBs) == 0 {
		return m, nil
	}
	m.Available = true

	for _, dbName := range qsDBs {
		safe := dbName

		// ── Top CPU queries ───────────────────────────────────────────────────
		topSQL := fmt.Sprintf(`
			USE [%s];
			SELECT TOP 10
				q.query_id,
				p.plan_id,
				ROUND(AVG(rs.avg_cpu_time)   / 1000.0, 2) AS avg_cpu_ms,
				ROUND(AVG(rs.avg_duration)   / 1000.0, 2) AS avg_dur_ms,
				ROUND(AVG(rs.avg_logical_io_reads), 0)    AS avg_reads,
				SUM(rs.count_executions)                  AS exec_count,
				MAX(rs.last_execution_time)               AS last_exec,
				ISNULL(SUBSTRING(qt.query_sql_text,1,400),'') AS qtext,
				ISNULL(OBJECT_NAME(q.object_id),'')       AS obj_name,
				CAST(ISNULL(p.is_forced_plan,0) AS INT)   AS is_forced
			FROM sys.query_store_query          q
			JOIN sys.query_store_query_text     qt ON q.query_text_id   = qt.query_text_id
			JOIN sys.query_store_plan           p  ON q.query_id        = p.query_id
			JOIN sys.query_store_runtime_stats  rs ON p.plan_id         = rs.plan_id
			JOIN sys.query_store_runtime_stats_interval rsi
				ON rs.runtime_stats_interval_id = rsi.runtime_stats_interval_id
			WHERE rsi.start_time >= DATEADD(HOUR, -24, GETUTCDATE())
			  AND q.is_internal_query = 0
			GROUP BY q.query_id, p.plan_id, qt.query_sql_text, q.object_id, p.is_forced_plan
			ORDER BY avg_cpu_ms DESC`, safe)

		rows, err := db.QueryContext(ctx, topSQL)
		if err == nil {
			for rows.Next() {
				var q QSQuery
				var lastExec time.Time
				var forced int
				if err := rows.Scan(&q.QueryID, &q.PlanID,
					&q.AvgCPUMs, &q.AvgDurationMs, &q.AvgLogicalReads,
					&q.ExecutionCount, &lastExec, &q.QueryText, &q.ObjectName, &forced); err == nil {
					q.LastExecTime = lastExec
					q.IsForced = forced == 1
					q.Database = safe
					m.TopCPUQueries = append(m.TopCPUQueries, q)
				}
			}
			rows.Close()
		}

		// ── Regressed queries (30%+ slower than previous plan) ───────────────
		regSQL := fmt.Sprintf(`
			USE [%s];
			SELECT TOP 5
				q.query_id,
				p_new.plan_id,
				ROUND(rs_new.avg_duration / 1000.0, 2)                          AS new_ms,
				ROUND((rs_new.avg_duration - rs_old.avg_duration) * 100.0
					/ NULLIF(rs_old.avg_duration,0), 1)                          AS pct_worse,
				ISNULL(SUBSTRING(qt.query_sql_text,1,300),'')                   AS qtext
			FROM sys.query_store_query         q
			JOIN sys.query_store_query_text    qt     ON q.query_text_id      = qt.query_text_id
			JOIN sys.query_store_plan          p_new  ON q.query_id           = p_new.query_id
			JOIN sys.query_store_runtime_stats rs_new ON p_new.plan_id        = rs_new.plan_id
			JOIN sys.query_store_runtime_stats_interval rsi_new
				ON rs_new.runtime_stats_interval_id = rsi_new.runtime_stats_interval_id
			JOIN sys.query_store_runtime_stats rs_old ON q.query_id = (
				SELECT TOP 1 query_id FROM sys.query_store_plan WHERE plan_id = rs_old.plan_id)
			JOIN sys.query_store_runtime_stats_interval rsi_old
				ON rs_old.runtime_stats_interval_id = rsi_old.runtime_stats_interval_id
			WHERE rsi_new.start_time >= DATEADD(HOUR, -6, GETUTCDATE())
			  AND rsi_old.start_time <  DATEADD(HOUR, -6, GETUTCDATE())
			  AND rs_new.avg_duration > rs_old.avg_duration * 1.3
			  AND q.is_internal_query = 0
			ORDER BY pct_worse DESC`, safe)

		rrows, err := db.QueryContext(ctx, regSQL)
		if err == nil {
			for rrows.Next() {
				var q QSQuery
				if err := rrows.Scan(&q.QueryID, &q.PlanID,
					&q.AvgDurationMs, &q.PctWorse, &q.QueryText); err == nil {
					q.PlanRegressed = true
					q.Database = safe
					m.RegressedQueries = append(m.RegressedQueries, q)
				}
			}
			rrows.Close()
		}

		// ── Forced plans ──────────────────────────────────────────────────────
		fSQL := fmt.Sprintf(`
			USE [%s];
			SELECT q.query_id, p.plan_id,
				ISNULL(SUBSTRING(qt.query_sql_text,1,200),'') AS qtext
			FROM sys.query_store_plan       p
			JOIN sys.query_store_query      q  ON p.query_id      = q.query_id
			JOIN sys.query_store_query_text qt ON q.query_text_id = qt.query_text_id
			WHERE p.is_forced_plan = 1`, safe)

		frows, err := db.QueryContext(ctx, fSQL)
		if err == nil {
			for frows.Next() {
				var fp QSForcedPlan
				if err := frows.Scan(&fp.QueryID, &fp.PlanID, &fp.QueryText); err == nil {
					fp.Database = safe
					m.ForcedPlans = append(m.ForcedPlans, fp)
				}
			}
			frows.Close()
		}
	}
	return m, nil
}

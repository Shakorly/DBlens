package main

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// CollectJobs checks SQL Agent for failed jobs and long-running jobs.
func CollectJobs(ctx context.Context, db *sql.DB, serverName string, lookbackHours int) (*JobMetrics, error) {
	metrics := &JobMetrics{ServerName: serverName}

	// ── Check if SQL Agent is available ──────────────────────────────────────
	var agentRunning int
	_ = db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM msdb.dbo.sysjobs`).Scan(&agentRunning)
	if agentRunning == 0 {
		return metrics, nil
	}

	// ── Failed jobs in lookback window ───────────────────────────────────────
	failedSQL := fmt.Sprintf(`
		SELECT TOP 20
			j.name                                              AS job_name,
			CONVERT(DATETIME,
				STUFF(STUFF(CAST(h.run_date AS VARCHAR(8)),7,0,'-'),5,0,'-')
				+ ' '
				+ STUFF(STUFF(RIGHT('000000'+CAST(h.run_time AS VARCHAR(6)),6),5,0,':'),3,0,':')
			)                                                   AS last_run_time,
			STUFF(STUFF(RIGHT('000000'+CAST(h.run_duration AS VARCHAR(6)),6),5,0,':'),3,0,':')
				                                                AS run_duration,
			ISNULL(h.message,'')                                AS message,
			ISNULL(s.step_name,'')                              AS step_name
		FROM msdb.dbo.sysjobhistory  h
		JOIN msdb.dbo.sysjobs         j ON h.job_id    = j.job_id
		JOIN msdb.dbo.sysjobsteps     s ON h.job_id    = s.job_id
		                               AND h.step_id   = s.step_id
		WHERE h.run_status = 0          -- 0 = Failed
		  AND h.step_id    > 0          -- exclude job-level outcome row
		  AND CONVERT(DATETIME,
				STUFF(STUFF(CAST(h.run_date AS VARCHAR(8)),7,0,'-'),5,0,'-')
				+ ' '
				+ STUFF(STUFF(RIGHT('000000'+CAST(h.run_time AS VARCHAR(6)),6),5,0,':'),3,0,':')
			) >= DATEADD(HOUR, -%d, GETDATE())
		ORDER BY last_run_time DESC`, lookbackHours)

	rows, err := db.QueryContext(ctx, failedSQL)
	if err != nil {
		// msdb access might be restricted — non-fatal
		return metrics, nil
	}
	defer rows.Close()

	for rows.Next() {
		var fj FailedJob
		var lastRunTime time.Time
		if err := rows.Scan(&fj.JobName, &lastRunTime, &fj.RunDuration, &fj.Message, &fj.StepName); err != nil {
			continue
		}
		fj.LastRunTime = lastRunTime
		if len(fj.Message) > 300 {
			fj.Message = fj.Message[:300] + "..."
		}
		metrics.FailedJobs = append(metrics.FailedJobs, fj)
	}
	rows.Close()

	// ── Currently running jobs ────────────────────────────────────────────────
	runningSQL := `
		SELECT
			j.name                                              AS job_name,
			DATEADD(SECOND,
				-(DATEDIFF(SECOND, a.start_execution_date, GETDATE())),
				GETDATE()
			)                                                   AS start_time,
			DATEDIFF(MINUTE, a.start_execution_date, GETDATE()) AS elapsed_minutes
		FROM msdb.dbo.sysjobactivity  a
		JOIN msdb.dbo.sysjobs          j ON a.job_id = j.job_id
		WHERE a.start_execution_date IS NOT NULL
		  AND a.stop_execution_date  IS NULL
		  AND DATEDIFF(MINUTE, a.start_execution_date, GETDATE()) > 30
		ORDER BY elapsed_minutes DESC`

	rows2, err := db.QueryContext(ctx, runningSQL)
	if err != nil {
		return metrics, nil
	}
	defer rows2.Close()

	for rows2.Next() {
		var lj LongRunningJob
		var startTime time.Time
		if err := rows2.Scan(&lj.JobName, &startTime, &lj.ElapsedMinutes); err != nil {
			continue
		}
		lj.StartTime = startTime
		metrics.LongRunJobs = append(metrics.LongRunJobs, lj)
	}

	return metrics, rows2.Err()
}

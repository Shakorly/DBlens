package main

import (
	"context"
	"database/sql"
)

// wellKnownBenignWaits are SQL Server internal waits that are safe to ignore
// when analysing user-visible performance problems.
var wellKnownBenignWaits = map[string]bool{
	"SLEEP_TASK": true, "SLEEP_SYSTEMTASK": true, "SLEEP_DBSTARTUP": true,
	"SLEEP_DCOMSTARTUP": true, "SLEEP_MASTERDBREADY": true, "SLEEP_MASTERMDREADY": true,
	"SLEEP_MASTERUPGRADED": true, "SLEEP_MSDBSTARTUP": true, "SLEEP_TEMPDBSTARTUP": true,
	"SLEEP_USERTASK": true, "SLEEP_WORKER": true, "WAITFOR": true,
	"SOS_WORK_DISPATCHER":        true,
	"DISPATCHER_QUEUE_SEMAPHORE": true, "XE_DISPATCHER_WAIT": true,
	"XE_TIMER_EVENT": true, "REQUEST_FOR_DEADLOCK_SEARCH": true,
	"RESOURCE_QUEUE": true, "SERVER_IDLE_CHECK": true, "HADR_WORK_QUEUE": true,
	"BROKER_TO_FLUSH": true, "BROKER_EVENTHANDLER": true, "CHECKPOINT_QUEUE": true,
	"DBMIRROR_EVENTS_QUEUE": true, "SQLTRACE_BUFFER_FLUSH": true,
	"CLR_AUTO_EVENT": true, "CLR_MANUAL_EVENT": true, "WAIT_XTP_OFFLINE_CKPT_NEW_LOG": true,
	"ONDEMAND_TASK_QUEUE": true, "FT_IFTS_SCHEDULER_IDLE_WAIT": true,
	"DIRTY_PAGE_POLL": true, "HADR_FILESTREAM_IOMGR_IOCOMPLETION": true,
	"SP_SERVER_DIAGNOSTICS_SLEEP": true, "WAIT_XTP_CKPT_AGENT_WAKEUP": true,
}

// categoriseWait maps a SQL Server wait type to a human-readable performance category.
func categoriseWait(waitType string) string {
	switch {
	case len(waitType) >= 5 && waitType[:5] == "LCK_M":
		return "Locking"
	case len(waitType) >= 8 && waitType[:8] == "PAGEIOLATCH":
		return "Disk I/O"
	case len(waitType) >= 6 && waitType[:6] == "PAGELATCH":
		return "Buffer Contention"
	case len(waitType) >= 9 && waitType[:9] == "WRITELOG":
		return "Log I/O"
	case waitType == "ASYNC_NETWORK_IO" || waitType == "NETWORK_IO":
		return "Network / Client"
	case len(waitType) >= 4 && waitType[:4] == "HADR":
		return "Always On AG"
	case waitType == "CXPACKET" || waitType == "CXCONSUMER" || waitType == "EXCHANGE":
		return "Parallelism"
	case waitType == "SOS_SCHEDULER_YIELD":
		return "CPU Pressure"
	case len(waitType) >= 3 && waitType[:3] == "IO_":
		return "Disk I/O"
	case len(waitType) >= 6 && waitType[:6] == "MEMORY":
		return "Memory"
	case len(waitType) >= 4 && waitType[:4] == "DBCC":
		return "Maintenance"
	default:
		return "Other"
	}
}

// CollectWaitStats returns the top non-benign server-wide wait types.
func CollectWaitStats(ctx context.Context, db *sql.DB, serverName string) (*WaitMetrics, error) {
	metrics := &WaitMetrics{ServerName: serverName}

	waitSQL := `
		SELECT TOP 30
			wait_type,
			ROUND(wait_time_ms / 1000.0, 1)                  AS wait_time_sec,
			waiting_tasks_count,
			CASE WHEN waiting_tasks_count = 0 THEN 0
				 ELSE ROUND(CAST(wait_time_ms AS FLOAT) / waiting_tasks_count, 2)
			END                                               AS avg_wait_ms
		FROM sys.dm_os_wait_stats
		WHERE wait_time_ms > 0
		  AND waiting_tasks_count > 0
		ORDER BY wait_time_ms DESC`

	rows, err := db.QueryContext(ctx, waitSQL)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type rawWait struct {
		waitType    string
		waitTimeSec float64
		waitTasks   int64
		avgWaitMS   float64
	}
	var all []rawWait
	var totalSec float64

	for rows.Next() {
		var w rawWait
		if err := rows.Scan(&w.waitType, &w.waitTimeSec, &w.waitTasks, &w.avgWaitMS); err != nil {
			continue
		}
		if wellKnownBenignWaits[w.waitType] {
			continue
		}
		all = append(all, w)
		totalSec += w.waitTimeSec
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for i, w := range all {
		if i >= 10 {
			break
		}
		pct := 0.0
		if totalSec > 0 {
			pct = w.waitTimeSec / totalSec * 100
		}
		metrics.TopWaits = append(metrics.TopWaits, WaitStat{
			WaitType:     w.waitType,
			Category:     categoriseWait(w.waitType),
			WaitTimeSec:  w.waitTimeSec,
			WaitingTasks: w.waitTasks,
			AvgWaitMS:    w.avgWaitMS,
			PctOfTotal:   pct,
		})
	}
	return metrics, nil
}

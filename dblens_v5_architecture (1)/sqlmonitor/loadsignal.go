package main

import (
	"context"
	"database/sql"
	"sync"
	"time"
)

// LoadSignal is a lightweight snapshot of server stress taken at the START of
// every cycle using only two fast DMV reads. All scheduling decisions
// reference this snapshot — never re-querying mid-cycle.
type LoadSignal struct {
	mu              sync.RWMutex
	SQLCPUPercent   int
	BlockedSessions int
	ActiveRequests  int
	TotalSessions   int
	CapturedAt      time.Time
	Valid           bool
}

// StressLevel categorises current load for scheduling decisions.
type StressLevel int

const (
	StressNone     StressLevel = iota // CPU < 60%, no blocking
	StressModerate                    // CPU 60–80% or minor blocking
	StressHigh                        // CPU > 80% or blocking > 2
	StressCritical                    // CPU > 90% or blocking > 5
)

func (s StressLevel) String() string {
	return [...]string{"NONE", "MODERATE", "HIGH", "CRITICAL"}[s]
}

// Level computes the current stress level from the snapshot.
func (ls *LoadSignal) Level() StressLevel {
	ls.mu.RLock()
	defer ls.mu.RUnlock()
	if !ls.Valid {
		return StressNone // no data yet — allow all collectors
	}
	cpu := ls.SQLCPUPercent
	blocked := ls.BlockedSessions
	switch {
	case cpu >= 90 || blocked >= 5:
		return StressCritical
	case cpu >= 80 || blocked >= 2:
		return StressHigh
	case cpu >= 60 || blocked >= 1:
		return StressModerate
	default:
		return StressNone
	}
}

// ShouldSkip returns true if the given tier should be suppressed at this stress level.
//
//	Tier       NONE  MODERATE  HIGH   CRITICAL
//	FAST       run   run       run    run
//	MEDIUM     run   run       skip   skip
//	SLOW       run   skip      skip   skip
//	ONCE       run   run       run    run   (only ever fires once anyway)
func (ls *LoadSignal) ShouldSkip(tier Tier) bool {
	level := ls.Level()
	switch tier {
	case TierMedium:
		return level >= StressHigh
	case TierSlow:
		return level >= StressModerate
	default:
		return false
	}
}

// Refresh captures a fresh load snapshot from the server.
// This is intentionally minimal: two lightweight DMV reads, < 50ms total.
func (ls *LoadSignal) Refresh(ctx context.Context, db *sql.DB) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// Single query combining both metrics
	const q = `
		SELECT
			ISNULL(
				(SELECT TOP 1
					rec.value('(./Record/SchedulerMonitorEvent/SystemHealth/ProcessUtilization)[1]','int')
				FROM (SELECT CONVERT(XML,record) AS rec
					  FROM sys.dm_os_ring_buffers
					  WHERE ring_buffer_type = N'RING_BUFFER_SCHEDULER_MONITOR'
					    AND record LIKE '%<SystemHealth>%') d
				ORDER BY rec.value('(./Record/@id)[1]','int') DESC
			), -1)                                                      AS cpu_pct,
			COUNT(*)                                                    AS total_sess,
			SUM(CASE WHEN r.session_id IS NOT NULL THEN 1 ELSE 0 END)  AS active_req,
			SUM(CASE WHEN ISNULL(r.blocking_session_id,0)<>0 THEN 1 ELSE 0 END) AS blocked
		FROM sys.dm_exec_sessions s
		LEFT JOIN sys.dm_exec_requests r ON s.session_id = r.session_id
		WHERE s.is_user_process = 1`

	var cpu, total, active, blocked int
	if err := db.QueryRowContext(ctx, q).Scan(&cpu, &total, &active, &blocked); err != nil {
		// On failure keep previous snapshot — don't penalise the schedule
		return
	}

	ls.mu.Lock()
	ls.SQLCPUPercent   = cpu
	ls.TotalSessions   = total
	ls.ActiveRequests  = active
	ls.BlockedSessions = blocked
	ls.CapturedAt      = time.Now()
	ls.Valid            = true
	ls.mu.Unlock()
}

// Snapshot returns a copy of current values for safe concurrent reads.
func (ls *LoadSignal) Snapshot() (cpu, sessions, active, blocked int) {
	ls.mu.RLock()
	defer ls.mu.RUnlock()
	return ls.SQLCPUPercent, ls.TotalSessions, ls.ActiveRequests, ls.BlockedSessions
}

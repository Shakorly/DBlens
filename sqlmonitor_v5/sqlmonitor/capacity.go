package main

import (
	"context"
	"database/sql"
	"math"
	"time"
)

// CapacityMetrics holds resource capacity planning projections.
type CapacityMetrics struct {
	ServerName  string
	CollectedAt time.Time
	CPUTrend    TrendProjection
	MemoryTrend TrendProjection
	DiskTrend   []DBDiskTrend
}

// TrendProjection is a simple linear projection of current usage.
type TrendProjection struct {
	CurrentPct    float64
	Avg7DayPct    float64
	ProjectedDays int    // days until projected to exceed threshold
	Status        string // "OK", "WARNING", "CRITICAL"
}

// DBDiskTrend captures growth rate for a single database.
type DBDiskTrend struct {
	DatabaseName       string
	CurrentSizeMB      float64
	GrowthRateMBPerDay float64
	DaysUntilFull      int
	MaxSizeMB          float64
	UtilisationPct     float64
	GrowthSource       string // "measured" | "estimated"
}

// CollectCapacity estimates capacity headroom.
// v5: uses real growth rate from HistoryStore when available.
func CollectCapacity(ctx context.Context, db *sql.DB, serverName string,
	history []MetricSample, cpuThreshold, memThreshold int, hs *HistoryStore) (*CapacityMetrics, error) {

	m := &CapacityMetrics{ServerName: serverName, CollectedAt: time.Now()}

	// ── CPU capacity ──────────────────────────────────────────────────────────
	var avgCPU float64
	if len(history) > 0 {
		sum := 0
		for _, h := range history {
			sum += h.HealthScore
		}
		avgScore := float64(sum) / float64(len(history))
		avgCPU = math.Max(0, 100-avgScore)
	}

	var curCPU int
	_ = db.QueryRowContext(ctx, `
		SELECT TOP 1
			rec.value('(./Record/SchedulerMonitorEvent/SystemHealth/ProcessUtilization)[1]', 'int')
		FROM (SELECT CONVERT(XML, record) AS rec FROM sys.dm_os_ring_buffers
		      WHERE ring_buffer_type = N'RING_BUFFER_SCHEDULER_MONITOR'
		        AND record LIKE '%<SystemHealth>%') AS d
		ORDER BY rec.value('(./Record/@id)[1]', 'int') DESC`).Scan(&curCPU)

	m.CPUTrend = TrendProjection{
		CurrentPct: float64(curCPU),
		Avg7DayPct: avgCPU,
	}
	m.CPUTrend.Status, m.CPUTrend.ProjectedDays = classifyCapacity(float64(curCPU), float64(cpuThreshold))

	// ── Memory capacity ───────────────────────────────────────────────────────
	var totalMB, availMB int64
	_ = db.QueryRowContext(ctx, `
		SELECT total_physical_memory_kb/1024, available_physical_memory_kb/1024
		FROM sys.dm_os_sys_memory`).Scan(&totalMB, &availMB)

	var memUsedPct float64
	if totalMB > 0 {
		memUsedPct = float64(totalMB-availMB) / float64(totalMB) * 100
	}
	m.MemoryTrend = TrendProjection{CurrentPct: memUsedPct}
	m.MemoryTrend.Status, m.MemoryTrend.ProjectedDays = classifyCapacity(memUsedPct, float64(100-memThreshold))

	// ── Disk capacity per database ────────────────────────────────────────────
	diskSQL := `
		SELECT
			DB_NAME(mf.database_id)                          AS db_name,
			SUM(mf.size) * 8.0 / 1024                        AS current_size_mb,
			SUM(CASE WHEN mf.max_size = -1 THEN 2147483648.0
				     WHEN mf.max_size = 268435456 THEN 2048000.0
				     ELSE mf.max_size * 8.0 / 1024 END)      AS max_size_mb,
			SUM(CAST(FILEPROPERTY(mf.name,'SpaceUsed') AS BIGINT)) * 8.0 / 1024 AS used_mb
		FROM sys.master_files mf
		WHERE mf.database_id > 4
		  AND mf.type = 0
		GROUP BY mf.database_id
		ORDER BY current_size_mb DESC`

	rows, err := db.QueryContext(ctx, diskSQL)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var dt DBDiskTrend
			if err := rows.Scan(&dt.DatabaseName, &dt.CurrentSizeMB, &dt.MaxSizeMB, &dt.UtilisationPct); err != nil {
				continue
			}
			if dt.CurrentSizeMB > 0 {
				dt.UtilisationPct = dt.UtilisationPct / dt.CurrentSizeMB * 100
			}
			freeMB := dt.MaxSizeMB - dt.CurrentSizeMB

			// v5: use real growth rate from history when we have enough data
			if hs != nil {
				realGrowth := calculateActualGrowthRate(serverName, dt.DatabaseName, hs)
				if realGrowth > 0 {
					dt.GrowthRateMBPerDay = realGrowth
					dt.GrowthSource = "measured"
				}
			}
			// Fall back to 3%/week estimate only when no history exists
			if dt.GrowthRateMBPerDay == 0 {
				dt.GrowthRateMBPerDay = dt.CurrentSizeMB * 0.003 / 7 // 0.3%/day ≈ 3%/week
				dt.GrowthSource = "estimated"
			}

			if dt.GrowthRateMBPerDay > 0 && freeMB > 0 {
				dt.DaysUntilFull = int(freeMB / dt.GrowthRateMBPerDay)
				if dt.DaysUntilFull > 9999 {
					dt.DaysUntilFull = 9999
				}
			}
			m.DiskTrend = append(m.DiskTrend, dt)
		}
	}
	return m, nil
}

// calculateActualGrowthRate computes MB/day from historical size snapshots (v5).
func calculateActualGrowthRate(serverName, dbName string, hs *HistoryStore) float64 {
	snapshots := hs.GetSizeHistory(serverName, dbName, 7)
	if len(snapshots) < 2 {
		return 0 // not enough data
	}
	oldest := snapshots[0]
	newest := snapshots[len(snapshots)-1]
	days := newest.Timestamp.Sub(oldest.Timestamp).Hours() / 24
	if days <= 0 {
		return 0
	}
	growthMB := newest.SizeMB - oldest.SizeMB
	if growthMB < 0 {
		return 0 // negative growth (shrinkage) — don't project
	}
	return growthMB / days
}

func classifyCapacity(current, threshold float64) (string, int) {
	headroom := threshold - current
	if headroom <= 0 {
		return "CRITICAL", 0
	} else if headroom < 15 {
		return "WARNING", int(headroom * 2)
	}
	return "OK", 999
}

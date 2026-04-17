package main

import (
	"fmt"
	"time"
)

// ComputeHealthScore calculates an A–F server health grade from all collected
// metrics. Each problem deducts points from a starting score of 100.
func ComputeHealthScore(r *CollectionResult, cfg *Config) HealthScore {
	score := 100
	var penalties []string

	// ── Connection errors / server unreachable ────────────────────────────────
	if len(r.Errors) > 0 && r.Connections == nil {
		return HealthScore{
			ServerName: r.ServerName,
			Grade:      "F",
			Score:      0,
			Penalties:  []string{"Server unreachable or collection failed entirely"},
			Timestamp:  time.Now(),
		}
	}

	// ── Connections ───────────────────────────────────────────────────────────
	if r.Connections != nil {
		c := r.Connections
		if c.BlockedSessions >= 5 {
			score -= 20
			penalties = append(penalties, "Severe blocking (5+ blocked sessions)")
		} else if c.BlockedSessions > 0 {
			score -= 10
			penalties = append(penalties, "Blocking detected")
		}
		if c.TotalSessions > cfg.MaxConnectionsThreshold {
			score -= 15
			penalties = append(penalties, "High connection count")
		}
	}

	// ── Queries ───────────────────────────────────────────────────────────────
	if r.Queries != nil {
		longCount := len(r.Queries.ActiveLongRunning)
		if longCount >= 5 {
			score -= 20
			penalties = append(penalties, "Many long-running queries (5+)")
		} else if longCount > 0 {
			score -= 10
			penalties = append(penalties, "Long-running queries detected")
		}
	}

	// ── CPU ───────────────────────────────────────────────────────────────────
	if r.Resources != nil {
		res := r.Resources
		if res.SQLCPUPercent >= 90 {
			score -= 25
			penalties = append(penalties, "Critical CPU (90%+)")
		} else if res.SQLCPUPercent >= cfg.CPUAlertThresholdPct {
			score -= 15
			penalties = append(penalties, "High CPU utilisation")
		}

		// ── Memory ────────────────────────────────────────────────────────────
		if res.TotalMemoryMB > 0 {
			availPct := float64(res.AvailableMemoryMB) / float64(res.TotalMemoryMB) * 100
			if availPct < 5 {
				score -= 20
				penalties = append(penalties, "Critical memory pressure (<5% available)")
			} else if availPct < float64(cfg.MemoryAlertThresholdPct) {
				score -= 10
				penalties = append(penalties, "Low available memory")
			}
		}

		// ── Disk I/O ──────────────────────────────────────────────────────────
		highIOFiles := 0
		for _, d := range res.DiskStats {
			if d.AvgReadMS > cfg.DiskIOAlertMS*2 || d.AvgWriteMS > cfg.DiskIOAlertMS*2 {
				highIOFiles++
			}
		}
		if highIOFiles >= 3 {
			score -= 20
			penalties = append(penalties, "Severe disk I/O latency on multiple files")
		} else if highIOFiles > 0 {
			score -= 10
			penalties = append(penalties, "High disk I/O latency")
		}
	}

	// ── Replication ───────────────────────────────────────────────────────────
	if r.Replication != nil {
		for _, ag := range r.Replication.AGGroups {
			for _, rep := range ag.Replicas {
				if rep.SyncHealth == "NOT_HEALTHY" {
					score -= 25
					penalties = append(penalties, "AG replica NOT_HEALTHY: "+rep.ReplicaServer)
				} else if rep.SyncHealth == "PARTIALLY_HEALTHY" {
					score -= 12
					penalties = append(penalties, "AG replica PARTIALLY_HEALTHY: "+rep.ReplicaServer)
				}
				if rep.SecondaryLagSeconds > cfg.ReplicationLagAlertSeconds*2 {
					score -= 15
					penalties = append(penalties, "Severe AG replication lag: "+rep.ReplicaServer)
				} else if rep.SecondaryLagSeconds > cfg.ReplicationLagAlertSeconds {
					score -= 8
					penalties = append(penalties, "AG replication lag: "+rep.ReplicaServer)
				}
			}
		}
	}

	// ── Backups ───────────────────────────────────────────────────────────────
	if r.Backups != nil {
		alertCount := 0
		for _, db := range r.Backups.Databases {
			if db.IsAlertFull {
				alertCount++
			}
		}
		if alertCount >= 3 {
			score -= 20
			penalties = append(penalties, "Multiple databases with overdue full backups")
		} else if alertCount > 0 {
			score -= 10
			penalties = append(penalties, "Database(s) with overdue full backup")
		}
	}

	// ── Jobs ─────────────────────────────────────────────────────────────────
	if r.Jobs != nil && len(r.Jobs.FailedJobs) > 0 {
		score -= 10
		penalties = append(penalties, "SQL Agent job failures detected")
	}

	// ── Deadlocks ─────────────────────────────────────────────────────────────
	if len(r.Deadlocks) > 3 {
		score -= 15
		penalties = append(penalties, "Frequent deadlocks (3+ in this cycle)")
	} else if len(r.Deadlocks) > 0 {
		score -= 5
		penalties = append(penalties, "Deadlock(s) detected")
	}

	// ── v5: OS Disk Volume Space ──────────────────────────────────────────────
	applyDiskSpacePenalties(r, cfg, &score, &penalties)

	// ── Floor at zero ─────────────────────────────────────────────────────────
	if score < 0 {
		score = 0
	}

	grade := scoreToGrade(score)
	return HealthScore{
		ServerName: r.ServerName,
		Grade:      grade,
		Score:      score,
		Penalties:  penalties,
		Timestamp:  time.Now(),
	}
}

func scoreToGrade(score int) string {
	switch {
	case score >= 90:
		return "A"
	case score >= 75:
		return "B"
	case score >= 60:
		return "C"
	case score >= 40:
		return "D"
	default:
		return "F"
	}
}

// v5: disk volume penalties are appended by ComputeHealthScore via the
// DiskSpace field on CollectionResult. The function below is called from
// ComputeHealthScore at the end of the existing penalty block.
func applyDiskSpacePenalties(r *CollectionResult, cfg *Config, score *int, penalties *[]string) {
	if r.DiskSpace == nil {
		return
	}
	critVols := 0
	warnVols := 0
	for _, v := range r.DiskSpace.Volumes {
		if v.FreePct < cfg.DiskCritFreePct {
			critVols++
		} else if v.FreePct < cfg.DiskWarnFreePct {
			warnVols++
		}
	}
	if critVols > 0 {
		*score -= 25
		*penalties = append(*penalties, fmt.Sprintf("Critical disk space on %d volume(s) (<%.0f%% free)", critVols, cfg.DiskCritFreePct))
	} else if warnVols > 0 {
		*score -= 10
		*penalties = append(*penalties, fmt.Sprintf("Low disk space on %d volume(s) (<%.0f%% free)", warnVols, cfg.DiskWarnFreePct))
	}
}

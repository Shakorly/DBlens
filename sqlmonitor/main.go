package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"
)

func main() {
	configPath := flag.String("config", "config.json", "path to JSON configuration file")
	flag.Parse()

	cfg, err := LoadConfig(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: failed to load config: %v\n", err)
		os.Exit(1)
	}

	logger, err := NewLogger(cfg.LogFile, cfg.LogLevel)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: failed to initialise logger: %v\n", err)
		os.Exit(1)
	}
	defer logger.Close()

	logger.Info("", fmt.Sprintf(
		"SQL Server Monitor started | servers=%d | poll_interval=%ds | slow_query_threshold=%dms | log=%s",
		len(cfg.Servers), cfg.PollIntervalSeconds, cfg.SlowQueryThresholdMS, cfg.LogFile,
	))
	logger.Info("", fmt.Sprintf(
		"Alert thresholds | max_connections=%d | cpu=%d%% | mem_avail<=%d%% | disk_io>=%gms | ag_lag>=%ds",
		cfg.MaxConnectionsThreshold, cfg.CPUAlertThresholdPct,
		cfg.MemoryAlertThresholdPct, cfg.DiskIOAlertMS, cfg.ReplicationLagAlertSeconds,
	))

	// Print server list
	for _, s := range cfg.Servers {
		logger.Info("", fmt.Sprintf("  Monitoring: %-18s  %s:%d  databases=%d",
			s.Name, s.Host, s.Port, len(s.Databases)))
	}

	ticker := time.NewTicker(time.Duration(cfg.PollIntervalSeconds) * time.Second)
	defer ticker.Stop()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Execute one collection immediately at startup.
	runCollection(cfg, logger)

	for {
		select {
		case <-ticker.C:
			runCollection(cfg, logger)
		case sig := <-sigCh:
			logger.Info("", fmt.Sprintf("Signal received: %v — shutting down gracefully", sig))
			return
		}
	}
}

// runCollection fans out concurrent collection across all configured servers
// and then processes each result sequentially for log output.
func runCollection(cfg *Config, logger *Logger) {
	start := time.Now()
	logger.Info("", strings.Repeat("─", 72))
	logger.Info("", fmt.Sprintf("Collection cycle started at %s", start.Format("2006-01-02 15:04:05")))

	results := make(chan *CollectionResult, len(cfg.Servers))
	var wg sync.WaitGroup

	for _, srv := range cfg.Servers {
		wg.Add(1)
		go func(s ServerConfig) {
			defer wg.Done()
			c := NewCollector(s, cfg, logger)
			results <- c.Collect()
		}(srv)
	}

	// Close channel once all goroutines finish.
	go func() {
		wg.Wait()
		close(results)
	}()

	var successCount, errorCount int
	for r := range results {
		if len(r.Errors) > 0 && r.Connections == nil {
			errorCount++
		} else {
			successCount++
		}
		processResult(r, cfg, logger)
	}

	elapsed := time.Since(start).Round(time.Millisecond)
	logger.Info("", fmt.Sprintf(
		"Collection cycle complete | duration=%v | ok=%d | failed=%d",
		elapsed, successCount, errorCount,
	))
}

// processResult evaluates a single server's CollectionResult and emits
// structured log entries and threshold-based alert lines.
func processResult(r *CollectionResult, cfg *Config, logger *Logger) {
	srv := r.ServerName

	// ── Collection errors ────────────────────────────────────────────────────
	for _, e := range r.Errors {
		logger.Error(srv, "Collection error: "+e)
	}

	// ── Active Connections ───────────────────────────────────────────────────
	if r.Connections != nil {
		c := r.Connections
		logger.Info(srv, fmt.Sprintf(
			"Connections | total_sessions=%d  active_requests=%d  blocked=%d",
			c.TotalSessions, c.ActiveRequests, c.BlockedSessions,
		))

		if c.TotalSessions > cfg.MaxConnectionsThreshold {
			logger.Warn(srv, fmt.Sprintf(
				"ALERT: High connection count | sessions=%d (threshold=%d)",
				c.TotalSessions, cfg.MaxConnectionsThreshold,
			))
		}
		if c.BlockedSessions > 0 {
			logger.Warn(srv, fmt.Sprintf(
				"ALERT: Blocked sessions detected | blocked=%d",
				c.BlockedSessions,
			))
			// Log the blocking chains for the first 5 blocked sessions.
			logged := 0
			for _, s := range c.Sessions {
				if s.BlockingSession != 0 && logged < 5 {
					logger.Warn(srv, fmt.Sprintf(
						"  Blocked: session=%d by=%d db=%s login=%s wait=%s elapsed_ms=%d",
						s.SessionID, s.BlockingSession, s.Database,
						s.LoginName, s.WaitType, s.ElapsedMS,
					))
					logged++
				}
			}
		}

		// Debug: dump all sessions.
		for _, s := range c.Sessions {
			logger.Debug(srv, fmt.Sprintf(
				"  Session: id=%d login=%s host=%s db=%s status=%s elapsed_ms=%d mem_mb=%.1f wait=%s",
				s.SessionID, s.LoginName, s.HostName, s.Database,
				s.Status, s.ElapsedMS, s.MemoryUsageMB, s.WaitType,
			))
		}
	}

	// ── Query Performance ────────────────────────────────────────────────────
	if r.Queries != nil {
		q := r.Queries

		if len(q.ActiveLongRunning) > 0 {
			logger.Warn(srv, fmt.Sprintf(
				"ALERT: Long-running queries | count=%d (threshold=%dms)",
				len(q.ActiveLongRunning), cfg.SlowQueryThresholdMS,
			))
			for i, aq := range q.ActiveLongRunning {
				if i >= 5 {
					break
				}
				logger.Warn(srv, fmt.Sprintf(
					"  ActiveQuery[%d]: session=%d  elapsed_ms=%d  cpu_ms=%d  db=%s  login=%s  wait=%s  cmd=%s",
					i+1, aq.SessionID, aq.ElapsedMS, aq.CPUTime,
					aq.Database, aq.LoginName, aq.WaitType, aq.Command,
				))
				if len(aq.QueryText) > 300 {
					logger.Debug(srv, fmt.Sprintf("    SQL: %s...", aq.QueryText[:300]))
				} else if aq.QueryText != "" {
					logger.Debug(srv, fmt.Sprintf("    SQL: %s", aq.QueryText))
				}
			}
		} else {
			logger.Info(srv, fmt.Sprintf("Queries | no active queries exceeding %dms threshold", cfg.SlowQueryThresholdMS))
		}

		if len(q.SlowQueries) > 0 {
			logger.Info(srv, fmt.Sprintf("PlanCache | top slow queries found: %d", len(q.SlowQueries)))
			for i, sq := range q.SlowQueries {
				if i >= 3 {
					break
				}
				logger.Info(srv, fmt.Sprintf(
					"  SlowQuery[%d]: avg_ms=%d  executions=%d  avg_reads=%d  avg_cpu_ms=%d  db=%s",
					i+1, sq.AvgElapsedMS, sq.ExecutionCount,
					sq.AvgLogicalReads, sq.AvgCPUMs, sq.Database,
				))
			}
		}
	}

	// ── CPU / Memory / Disk ──────────────────────────────────────────────────
	if r.Resources != nil {
		res := r.Resources

		cpuStr := fmt.Sprintf("%d%%", res.SQLCPUPercent)
		if res.SQLCPUPercent < 0 {
			cpuStr = "n/a"
		}
		sysCPUStr := fmt.Sprintf("%d%%", res.SystemCPUPercent)
		if res.SystemCPUPercent < 0 {
			sysCPUStr = "n/a"
		}

		var memUsedPct float64
		if res.TotalMemoryMB > 0 {
			memUsedPct = float64(res.TotalMemoryMB-res.AvailableMemoryMB) / float64(res.TotalMemoryMB) * 100
		}

		logger.Info(srv, fmt.Sprintf(
			"Resources | sql_cpu=%s  sys_cpu=%s  mem_total=%dMB  mem_avail=%dMB (used=%.1f%%)  sql_mem=%dMB  mem_state=%s",
			cpuStr, sysCPUStr,
			res.TotalMemoryMB, res.AvailableMemoryMB, memUsedPct,
			res.SQLMemoryMB, res.MemoryStateDesc,
		))

		// CPU alert
		if res.SQLCPUPercent >= 0 && res.SQLCPUPercent > cfg.CPUAlertThresholdPct {
			logger.Warn(srv, fmt.Sprintf(
				"ALERT: High SQL CPU | sql_cpu=%d%% (threshold=%d%%)",
				res.SQLCPUPercent, cfg.CPUAlertThresholdPct,
			))
		}

		// Memory alert — warn when available memory drops below threshold %.
		if res.TotalMemoryMB > 0 {
			availPct := float64(res.AvailableMemoryMB) / float64(res.TotalMemoryMB) * 100
			if availPct < float64(cfg.MemoryAlertThresholdPct) {
				logger.Warn(srv, fmt.Sprintf(
					"ALERT: Low available memory | avail=%.1f%% (threshold=%d%%)",
					availPct, cfg.MemoryAlertThresholdPct,
				))
			}
		}

		// Disk I/O alerts — report files with high latency.
		for _, d := range res.DiskStats {
			if d.AvgReadMS > cfg.DiskIOAlertMS || d.AvgWriteMS > cfg.DiskIOAlertMS {
				logger.Warn(srv, fmt.Sprintf(
					"ALERT: High disk I/O | db=%s  type=%s  avg_read_ms=%.1f  avg_write_ms=%.1f  path=%s",
					d.Database, d.FileType, d.AvgReadMS, d.AvgWriteMS, d.PhysicalName,
				))
			} else {
				logger.Debug(srv, fmt.Sprintf(
					"  DiskIO: db=%s  type=%s  avg_read_ms=%.1f  avg_write_ms=%.1f  mb_read=%d  mb_written=%d",
					d.Database, d.FileType, d.AvgReadMS, d.AvgWriteMS, d.MBRead, d.MBWritten,
				))
			}
		}
	}

	// ── Always On Replication ────────────────────────────────────────────────
	if r.Replication != nil {
		if len(r.Replication.AGGroups) == 0 {
			logger.Info(srv, "Replication | No Availability Groups configured on this server")
		}
		for _, ag := range r.Replication.AGGroups {
			for _, rep := range ag.Replicas {
				logger.Info(srv, fmt.Sprintf(
					"AG[%s] | replica=%-20s  role=%-9s  state=%-12s  connected=%-13s  sync=%s  lag=%ds  log_queue=%dKB  redo_queue=%dKB",
					ag.AGName, rep.ReplicaServer, rep.Role,
					rep.OperationalState, rep.ConnectedState, rep.SyncHealth,
					rep.SecondaryLagSeconds, rep.LogSendQueueKB, rep.RedoQueueKB,
				))

				if rep.SyncHealth != "HEALTHY" && rep.SyncHealth != "" && rep.SyncHealth != "UNKNOWN" {
					logger.Warn(srv, fmt.Sprintf(
						"ALERT: AG sync issue | ag=%s  replica=%s  sync_health=%s  connected=%s",
						ag.AGName, rep.ReplicaServer, rep.SyncHealth, rep.ConnectedState,
					))
				}
				if rep.SecondaryLagSeconds > cfg.ReplicationLagAlertSeconds {
					logger.Warn(srv, fmt.Sprintf(
						"ALERT: High replication lag | ag=%s  replica=%s  lag=%ds (threshold=%ds)",
						ag.AGName, rep.ReplicaServer,
						rep.SecondaryLagSeconds, cfg.ReplicationLagAlertSeconds,
					))
				}
				if rep.LogSendQueueKB > 102400 { // > 100 MB
					logger.Warn(srv, fmt.Sprintf(
						"ALERT: Large log send queue | ag=%s  replica=%s  log_queue=%dKB",
						ag.AGName, rep.ReplicaServer, rep.LogSendQueueKB,
					))
				}
			}
		}
	}
}

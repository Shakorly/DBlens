package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"
)

// ── Alert deduplication ────────────────────────────────────────────────────────

type alertKey struct{ server, kind string }

type AlertDeduplicator struct {
	mu       sync.Mutex
	lastSeen map[alertKey]time.Time
	cooldown time.Duration
}

func NewAlertDeduplicator(cooldownMinutes int) *AlertDeduplicator {
	return &AlertDeduplicator{
		lastSeen: make(map[alertKey]time.Time),
		cooldown: time.Duration(cooldownMinutes) * time.Minute,
	}
}

func (a *AlertDeduplicator) ShouldAlert(server, kind string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	k := alertKey{server, kind}
	if last, seen := a.lastSeen[k]; seen && time.Since(last) < a.cooldown {
		return false
	}
	a.lastSeen[k] = time.Now()
	return true
}

// ── Main ──────────────────────────────────────────────────────────────────────

func main() {
	configPath := flag.String("config", "config.json", "path to config file")
	flag.Parse()

	cfg, err := LoadConfig(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		os.Exit(1)
	}

	logger, err := NewLogger(cfg.LogFile, cfg.LogLevel)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: logger: %v\n", err)
		os.Exit(1)
	}
	defer logger.Close()

	dedup    := NewAlertDeduplicator(cfg.AlertCooldownMinutes)
	notifier := NewNotifier(cfg.AlertChannels, cfg.AlertCooldownMinutes, logger)
	store    := NewDashboardStore()
	hs       := NewHistoryStore("monitor_data", 1440)
	anomaly  := NewAnomalyDetector(cfg.AnomalyThresholdPct, cfg.AnomalyMinSamples)

	logger.Info("", fmt.Sprintf(
		"DBLens Monitor v5 started | servers=%d | poll=%ds | slow_query=%dms | anomaly=%.0f%% | log=%s",
		len(cfg.Servers), cfg.PollIntervalSeconds, cfg.SlowQueryThresholdMS,
		cfg.AnomalyThresholdPct, cfg.LogFile,
	))
	logger.Info("", fmt.Sprintf(
		"Thresholds | conn=%d | cpu=%d%% | mem<=%d%% | disk_io>=%gms | ag_lag>=%ds | backup_full>=%gh | cooldown=%dm",
		cfg.MaxConnectionsThreshold, cfg.CPUAlertThresholdPct, cfg.MemoryAlertThresholdPct,
		cfg.DiskIOAlertMS, cfg.ReplicationLagAlertSeconds,
		cfg.BackupFullAlertHours, cfg.AlertCooldownMinutes,
	))
	logger.Info("", fmt.Sprintf(
		"v5: index_check=%dh | integrity_check=%dh | disk_warn=<%.0f%% | disk_crit=<%.0f%%",
		cfg.IndexCheckHours, cfg.IntegrityCheckHours, cfg.DiskWarnFreePct, cfg.DiskCritFreePct,
	))

	if len(cfg.AlertChannels) > 0 {
		for _, ch := range cfg.AlertChannels {
			if ch.Enabled {
				logger.Info("", fmt.Sprintf("Alert channel: [%s] type=%s min_severity=%s", ch.Name, ch.Type, ch.MinSeverity))
			}
		}
	}

	if cfg.DashboardEnabled {
		go StartDashboard(cfg.DashboardPort, store, hs, logger)
	}
	if cfg.PrometheusEnabled {
		pe := NewPrometheusExporter(store)
		go startPrometheus(cfg.PrometheusPort, pe, logger)
	}

	for _, s := range cfg.Servers {
		// v5: warn if password is still coming from config file rather than env
		pwSource := "env"
		if s.Password != "" {
			envKey := "DBLENS_PASS_" + strings.ToUpper(
				strings.NewReplacer("-", "_", " ", "_", ".", "_").Replace(s.Name))
			if os.Getenv(envKey) == "" && os.Getenv("DBLENS_PASS") == "" {
				pwSource = "config.json ⚠ consider using env var " + envKey
			}
		}
		logger.Info("", fmt.Sprintf("  → %-20s %s:%d  databases=%d  pw-source=%s",
			s.Name, s.Host, s.Port, len(s.Databases), pwSource))
	}

	collectors := make([]*Collector, len(cfg.Servers))
	for i, srv := range cfg.Servers {
		collectors[i] = NewCollector(srv, cfg, logger, hs, anomaly)
	}

	ticker := time.NewTicker(time.Duration(cfg.PollIntervalSeconds) * time.Second)
	defer ticker.Stop()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	runCollection(collectors, cfg, logger, dedup, notifier, store)

	for {
		select {
		case <-ticker.C:
			runCollection(collectors, cfg, logger, dedup, notifier, store)
		case sig := <-sigCh:
			logger.Info("", fmt.Sprintf("Signal: %v — shutting down", sig))
			// v5: gracefully close persistent connection pools
			for _, c := range collectors {
				c.Close()
			}
			return
		}
	}
}

func startPrometheus(port int, pe *PrometheusExporter, logger *Logger) {
	mux := http.NewServeMux()
	mux.HandleFunc("/metrics", pe.Handler)
	addr := fmt.Sprintf(":%d", port)
	logger.Info("", fmt.Sprintf("📊 Prometheus metrics → http://localhost%s/metrics", addr))
	if err := http.ListenAndServe(addr, mux); err != nil {
		logger.Error("", "Prometheus: "+err.Error())
	}
}

func runCollection(collectors []*Collector, cfg *Config, logger *Logger,
	dedup *AlertDeduplicator, notifier *Notifier, store *DashboardStore) {

	start := time.Now()
	logger.Info("", strings.Repeat("─", 72))
	logger.Info("", "Collection cycle started at "+start.Format("2006-01-02 15:04:05"))

	results := make(chan *CollectionResult, len(collectors))
	var wg sync.WaitGroup
	for _, c := range collectors {
		wg.Add(1)
		go func(col *Collector) {
			defer wg.Done()
			results <- col.Collect()
		}(c)
	}
	go func() { wg.Wait(); close(results) }()

	var ok, failed int
	for r := range results {
		store.Update(r)
		if len(r.Errors) > 0 && r.Connections == nil {
			failed++
		} else {
			ok++
		}
		// v5: fixed — processResult no longer takes store as a parameter
		processResult(r, cfg, logger, dedup, notifier)
	}

	logger.Info("", fmt.Sprintf(
		"Collection cycle complete | duration=%v | ok=%d | failed=%d",
		time.Since(start).Round(time.Millisecond), ok, failed,
	))
}

// ── Result processing & alerting ──────────────────────────────────────────────
// v5: Fixed signature (removed unused store param that caused compile error)

func processResult(r *CollectionResult, cfg *Config, logger *Logger,
	dedup *AlertDeduplicator, notifier *Notifier) {

	srv := r.ServerName

	alert := func(sev, key, title, body string) {
		if !dedup.ShouldAlert(srv, key) {
			return
		}
		switch sev {
		case "ERROR":
			logger.Error(srv, "ALERT: "+title)
		case "WARN":
			logger.Warn(srv, "ALERT: "+title)
		default:
			logger.Info(srv, title)
		}
		if body != "" {
			logger.Debug(srv, "  Detail: "+body)
		}
		notifier.Send(sev, srv, title, body)
	}

	// ── Health Score ──────────────────────────────────────────────────────────
	if r.Health != nil {
		h := r.Health
		msg := fmt.Sprintf("HealthScore | grade=%s score=%d/100", h.Grade, h.Score)
		switch h.Grade {
		case "A", "B":
			logger.Info(srv, msg)
		case "C":
			logger.Warn(srv, msg)
		default:
			logger.Error(srv, msg)
			notifier.Send("ERROR", srv, "Server health critical: grade "+h.Grade, strings.Join(h.Penalties, "; "))
		}
	}

	// ── Collection errors ─────────────────────────────────────────────────────
	for _, e := range r.Errors {
		logger.Error(srv, "Collection error: "+e)
	}

	// ── Connections ───────────────────────────────────────────────────────────
	if r.Connections != nil {
		c := r.Connections
		logger.Info(srv, fmt.Sprintf("Connections | total=%d active=%d blocked=%d",
			c.TotalSessions, c.ActiveRequests, c.BlockedSessions))
		if c.TotalSessions > cfg.MaxConnectionsThreshold {
			alert("WARN", "high-conn", fmt.Sprintf("High connection count: %d (limit %d)",
				c.TotalSessions, cfg.MaxConnectionsThreshold), "")
		}
		if c.BlockedSessions > 0 {
			alert("WARN", "blocked", fmt.Sprintf("Blocked sessions: %d", c.BlockedSessions), "")
			for _, s := range r.Connections.Sessions {
				if s.BlockingSession != 0 {
					logger.Warn(srv, fmt.Sprintf("  Blocked sid=%d by=%d db=%s wait=%s elapsed_ms=%d",
						s.SessionID, s.BlockingSession, s.Database, s.WaitType, s.ElapsedMS))
				}
			}
		}
	}

	// ── Queries ───────────────────────────────────────────────────────────────
	if r.Queries != nil {
		if len(r.Queries.ActiveLongRunning) > 0 {
			alert("WARN", "slow-q", fmt.Sprintf("Long-running queries: %d (>%dms)",
				len(r.Queries.ActiveLongRunning), cfg.SlowQueryThresholdMS), "")
			for i, q := range r.Queries.ActiveLongRunning {
				if i >= 5 {
					break
				}
				logger.Warn(srv, fmt.Sprintf("  Query[%d]: sid=%d elapsed_ms=%d db=%s wait=%s",
					i+1, q.SessionID, q.ElapsedMS, q.Database, q.WaitType))
			}
		} else {
			logger.Info(srv, fmt.Sprintf("Queries | no queries exceeding %dms", cfg.SlowQueryThresholdMS))
		}
	}

	// ── Resources ─────────────────────────────────────────────────────────────
	if r.Resources != nil {
		res := r.Resources
		cpuStr := fmt.Sprintf("%d%%", res.SQLCPUPercent)
		if res.SQLCPUPercent < 0 {
			cpuStr = "n/a"
		}
		var memPct float64
		if res.TotalMemoryMB > 0 {
			memPct = float64(res.TotalMemoryMB-res.AvailableMemoryMB) / float64(res.TotalMemoryMB) * 100
		}
		logger.Info(srv, fmt.Sprintf("Resources | cpu=%s mem_avail=%dMB (used=%.1f%%) sql_mem=%dMB",
			cpuStr, res.AvailableMemoryMB, memPct, res.SQLMemoryMB))

		if res.SQLCPUPercent >= 0 && res.SQLCPUPercent > cfg.CPUAlertThresholdPct {
			alert("WARN", "cpu", fmt.Sprintf("High CPU: %d%% (threshold %d%%)",
				res.SQLCPUPercent, cfg.CPUAlertThresholdPct), "")
		}
		if res.TotalMemoryMB > 0 {
			avail := float64(res.AvailableMemoryMB) / float64(res.TotalMemoryMB) * 100
			if avail < float64(cfg.MemoryAlertThresholdPct) {
				alert("WARN", "mem", fmt.Sprintf("Low memory: %.1f%% available", avail), "")
			}
		}
		for _, d := range res.DiskStats {
			if d.AvgReadMS > cfg.DiskIOAlertMS || d.AvgWriteMS > cfg.DiskIOAlertMS {
				alert("WARN", "disk-"+d.Database,
					fmt.Sprintf("High disk I/O: db=%s read=%.1fms write=%.1fms", d.Database, d.AvgReadMS, d.AvgWriteMS), "")
			}
		}
	}

	// ── v5: OS Disk Volume Space ──────────────────────────────────────────────
	if r.DiskSpace != nil {
		for _, v := range r.DiskSpace.Volumes {
			if v.FreePct < cfg.DiskCritFreePct {
				alert("ERROR", "vol-crit-"+v.MountPoint,
					fmt.Sprintf("CRITICAL disk space: %s — %.1f%% free (%.1fGB of %.1fGB)",
						v.MountPoint, v.FreePct, v.FreeGB, v.TotalGB),
					fmt.Sprintf("Below critical threshold %.0f%%", cfg.DiskCritFreePct))
			} else if v.FreePct < cfg.DiskWarnFreePct {
				alert("WARN", "vol-warn-"+v.MountPoint,
					fmt.Sprintf("Low disk space: %s — %.1f%% free (%.1fGB of %.1fGB)",
						v.MountPoint, v.FreePct, v.FreeGB, v.TotalGB),
					fmt.Sprintf("Below warning threshold %.0f%%", cfg.DiskWarnFreePct))
			} else {
				logger.Debug(srv, fmt.Sprintf("Volume: %s — %.1f%% free (%.1fGB)",
					v.MountPoint, v.FreePct, v.FreeGB))
			}
		}
	}

	// ── Replication ───────────────────────────────────────────────────────────
	if r.Replication != nil {
		if len(r.Replication.AGGroups) == 0 {
			logger.Info(srv, "Replication | No AG configured")
		}
		for _, ag := range r.Replication.AGGroups {
			for _, rep := range ag.Replicas {
				logger.Info(srv, fmt.Sprintf("AG[%s] | replica=%s role=%s sync=%s lag=%ds",
					ag.AGName, rep.ReplicaServer, rep.Role, rep.SyncHealth, rep.SecondaryLagSeconds))
				if rep.SyncHealth != "HEALTHY" && rep.SyncHealth != "" && rep.SyncHealth != "UNKNOWN" {
					alert("WARN", "ag-sync-"+rep.ReplicaServer,
						fmt.Sprintf("AG sync issue: %s on %s", rep.SyncHealth, rep.ReplicaServer), "")
				}
				if rep.SecondaryLagSeconds > cfg.ReplicationLagAlertSeconds {
					alert("WARN", "ag-lag-"+rep.ReplicaServer,
						fmt.Sprintf("AG lag %ds on %s (threshold %ds)",
							rep.SecondaryLagSeconds, rep.ReplicaServer, cfg.ReplicationLagAlertSeconds), "")
				}
			}
		}
	}

	// ── Backups ───────────────────────────────────────────────────────────────
	if r.Backups != nil {
		full, log := 0, 0
		for _, db := range r.Backups.Databases {
			if db.IsAlertFull {
				full++
			}
			if db.IsAlertLog {
				log++
			}
		}
		if full > 0 {
			alert("WARN", "bak-full", fmt.Sprintf("Overdue full backups: %d databases", full),
				fmt.Sprintf("Threshold: %.0fh", cfg.BackupFullAlertHours))
		}
		if log > 0 {
			alert("WARN", "bak-log", fmt.Sprintf("Overdue log backups: %d databases", log), "")
		}
		if full == 0 && log == 0 {
			logger.Info(srv, fmt.Sprintf("Backups | all %d databases within thresholds", len(r.Backups.Databases)))
		}
	}

	// ── Jobs ──────────────────────────────────────────────────────────────────
	if r.Jobs != nil {
		if len(r.Jobs.FailedJobs) > 0 {
			alert("WARN", "jobs", fmt.Sprintf("SQL Agent failures: %d jobs", len(r.Jobs.FailedJobs)), "")
			for _, j := range r.Jobs.FailedJobs {
				logger.Warn(srv, fmt.Sprintf("  FailedJob: %q step=%q at=%s", j.JobName, j.StepName, j.LastRunTime.Format("15:04:05")))
			}
		} else {
			logger.Info(srv, "Jobs | no failures in lookback window")
		}
	}

	// ── Waits ─────────────────────────────────────────────────────────────────
	if r.Waits != nil && len(r.Waits.TopWaits) > 0 {
		top := r.Waits.TopWaits[0]
		logger.Info(srv, fmt.Sprintf("Waits | top=%s (%s) %.1f%% avg=%.1fms",
			top.WaitType, top.Category, top.PctOfTotal, top.AvgWaitMS))
		if top.PctOfTotal > 30 {
			alert("WARN", "wait-"+top.WaitType,
				fmt.Sprintf("Dominant wait: %s (%s) %.0f%%", top.WaitType, top.Category, top.PctOfTotal), "")
		}
	}

	// ── Index health ──────────────────────────────────────────────────────────
	if r.Indexes != nil {
		if len(r.Indexes.MissingIndexes) > 0 {
			top := r.Indexes.MissingIndexes[0]
			alert("WARN", "missing-idx",
				fmt.Sprintf("Missing index on %s.%s (impact=%.0f)", top.Database, top.TableName, top.ImpactScore),
				fmt.Sprintf("CREATE INDEX ON %s (%s) INCLUDE (%s)", top.TableName, top.EqualityColumns, top.IncludeColumns))
		}
		if len(r.Indexes.FragmentedIndexes) > 0 {
			top := r.Indexes.FragmentedIndexes[0]
			alert("WARN", "frag-idx",
				fmt.Sprintf("Fragmented: %s on %s (%.0f%%) — %s",
					top.IndexName, top.TableName, top.FragmentationPct, top.RecommendedAction), "")
		}
	}

	// ── DB Sizes ──────────────────────────────────────────────────────────────
	if r.Sizes != nil {
		for _, db := range r.Sizes.Databases {
			logger.Debug(srv, fmt.Sprintf("DBSize: %s data=%.0fMB log=%.0fMB data_free=%.1f%% log_free=%.1f%%",
				db.DatabaseName, db.DataSizeMB, db.LogSizeMB, db.DataFreePct, db.LogFreePct))
			if db.DataFreePct < 10 {
				alert("WARN", "data-space-"+db.DatabaseName,
					fmt.Sprintf("Low data file space: %s (%.1f%% free)", db.DatabaseName, db.DataFreePct), "")
			}
		}
	}

	// ── Deadlocks ─────────────────────────────────────────────────────────────
	for _, dl := range r.Deadlocks {
		alert("ERROR", "deadlock-"+dl.OccurredAt.String(),
			fmt.Sprintf("Deadlock at %s (victim SPID %d)", dl.OccurredAt.Format("15:04:05"), dl.VictimSPID), "")
	}

	// ── Query Store ───────────────────────────────────────────────────────────
	if r.QueryStore != nil && r.QueryStore.Available {
		logger.Info(srv, fmt.Sprintf("QueryStore | top_cpu=%d regressed=%d forced_plans=%d",
			len(r.QueryStore.TopCPUQueries),
			len(r.QueryStore.RegressedQueries),
			len(r.QueryStore.ForcedPlans)))
		for i, q := range r.QueryStore.RegressedQueries {
			if i >= 3 {
				break
			}
			alert("WARN", fmt.Sprintf("qs-regress-%d", q.QueryID),
				fmt.Sprintf("Query plan regression: db=%s %.0f%% slower", q.Database, q.PctWorse), "")
		}
	}

	// ── Integrity ─────────────────────────────────────────────────────────────
	if r.Integrity != nil {
		for _, db := range r.Integrity.Databases {
			if db.IsCorrupt {
				alert("ERROR", "corrupt-"+db.DatabaseName,
					fmt.Sprintf("CORRUPTION: %s has %d suspect page(s)!", db.DatabaseName, db.SuspectPages), "")
			}
			if db.CheckDBOverdue {
				alert("WARN", "checkdb-"+db.DatabaseName,
					fmt.Sprintf("CHECKDB overdue: %s (%.0f days)", db.DatabaseName, db.DaysSinceCheckDB), "")
			}
		}
	}

	// ── Anomalies ─────────────────────────────────────────────────────────────
	for _, a := range r.Anomalies {
		alert("WARN", "anomaly-"+a.Metric, "Statistical anomaly: "+a.Message, "")
	}

	// ── Custom Metrics ────────────────────────────────────────────────────────
	for _, cm := range r.CustomMetrics {
		if cm.Error != "" {
			logger.Error(srv, fmt.Sprintf("CustomMetric[%s]: %s", cm.Name, cm.Error))
		} else {
			logger.Debug(srv, fmt.Sprintf("CustomMetric: %s = %.2f %s", cm.Name, cm.Value, cm.Unit))
			if cm.AlertHigh {
				alert("WARN", "custom-"+cm.Name, fmt.Sprintf("Custom metric %s = %.2f %s (above threshold)", cm.Name, cm.Value, cm.Unit), "")
			}
			if cm.AlertLow {
				alert("WARN", "custom-low-"+cm.Name, fmt.Sprintf("Custom metric %s = %.2f %s (below threshold)", cm.Name, cm.Value, cm.Unit), "")
			}
		}
	}
}

package main

import (
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

// PrometheusExporter exposes metrics at /metrics in Prometheus text format.
// Compatible with Prometheus scrape + Grafana dashboards out of the box.
type PrometheusExporter struct {
	store  *DashboardStore
	mu     sync.RWMutex
	scrapedAt time.Time
}

func NewPrometheusExporter(store *DashboardStore) *PrometheusExporter {
	return &PrometheusExporter{store: store}
}

// Handler serves the /metrics endpoint.
func (pe *PrometheusExporter) Handler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	pe.store.mu.RLock()
	results := pe.store.results
	pe.store.mu.RUnlock()

	var sb strings.Builder

	writeHelp := func(name, help, typ string) {
		fmt.Fprintf(&sb, "# HELP %s %s\n# TYPE %s %s\n", name, help, name, typ)
	}

	// ── Health Score ──────────────────────────────────────────────────────────
	writeHelp("dblens_health_score", "Server health score 0-100", "gauge")
	for _, r := range results {
		if r.Health != nil {
			fmt.Fprintf(&sb, `dblens_health_score{server=%q} %d`+"\n",
				r.ServerName, r.Health.Score)
		}
	}

	// ── CPU ───────────────────────────────────────────────────────────────────
	writeHelp("dblens_sql_cpu_percent", "SQL Server CPU utilisation percent", "gauge")
	for _, r := range results {
		if r.Resources != nil && r.Resources.SQLCPUPercent >= 0 {
			fmt.Fprintf(&sb, `dblens_sql_cpu_percent{server=%q} %d`+"\n",
				r.ServerName, r.Resources.SQLCPUPercent)
		}
	}

	// ── Memory ────────────────────────────────────────────────────────────────
	writeHelp("dblens_memory_total_mb", "Total physical memory MB", "gauge")
	writeHelp("dblens_memory_available_mb", "Available physical memory MB", "gauge")
	writeHelp("dblens_sql_memory_mb", "Memory used by SQL Server MB", "gauge")
	for _, r := range results {
		if r.Resources != nil {
			fmt.Fprintf(&sb, `dblens_memory_total_mb{server=%q} %d`+"\n",
				r.ServerName, r.Resources.TotalMemoryMB)
			fmt.Fprintf(&sb, `dblens_memory_available_mb{server=%q} %d`+"\n",
				r.ServerName, r.Resources.AvailableMemoryMB)
			fmt.Fprintf(&sb, `dblens_sql_memory_mb{server=%q} %d`+"\n",
				r.ServerName, r.Resources.SQLMemoryMB)
		}
	}

	// ── Sessions ──────────────────────────────────────────────────────────────
	writeHelp("dblens_sessions_total", "Total user sessions", "gauge")
	writeHelp("dblens_sessions_blocked", "Blocked sessions count", "gauge")
	writeHelp("dblens_sessions_active", "Active requests count", "gauge")
	for _, r := range results {
		if r.Connections != nil {
			fmt.Fprintf(&sb, `dblens_sessions_total{server=%q} %d`+"\n",
				r.ServerName, r.Connections.TotalSessions)
			fmt.Fprintf(&sb, `dblens_sessions_blocked{server=%q} %d`+"\n",
				r.ServerName, r.Connections.BlockedSessions)
			fmt.Fprintf(&sb, `dblens_sessions_active{server=%q} %d`+"\n",
				r.ServerName, r.Connections.ActiveRequests)
		}
	}

	// ── Query Performance ─────────────────────────────────────────────────────
	writeHelp("dblens_slow_queries", "Active queries exceeding threshold", "gauge")
	writeHelp("dblens_deadlocks_total", "Deadlocks detected this cycle", "counter")
	for _, r := range results {
		slowQ := 0
		if r.Queries != nil && r.Queries.ActiveLongRunning != nil {
			slowQ = len(r.Queries.ActiveLongRunning)
		}
		fmt.Fprintf(&sb, `dblens_slow_queries{server=%q} %d`+"\n", r.ServerName, slowQ)
		fmt.Fprintf(&sb, `dblens_deadlocks_total{server=%q} %d`+"\n",
			r.ServerName, len(r.Deadlocks))
	}

	// ── Transactions ──────────────────────────────────────────────────────────
	writeHelp("dblens_transactions_per_sec", "SQL Server transactions per second", "gauge")
	writeHelp("dblens_batch_requests_per_sec", "SQL Server batch requests per second", "gauge")
	writeHelp("dblens_active_transactions", "Active open transactions", "gauge")
	for _, r := range results {
		if r.Transactions != nil {
			fmt.Fprintf(&sb, `dblens_transactions_per_sec{server=%q} %.2f`+"\n",
				r.ServerName, r.Transactions.TransactionsPerSec)
			fmt.Fprintf(&sb, `dblens_batch_requests_per_sec{server=%q} %.2f`+"\n",
				r.ServerName, r.Transactions.BatchRequestsPerSec)
			fmt.Fprintf(&sb, `dblens_active_transactions{server=%q} %d`+"\n",
				r.ServerName, r.Transactions.ActiveTransactions)
		}
	}

	// ── Disk I/O ──────────────────────────────────────────────────────────────
	writeHelp("dblens_disk_read_latency_ms", "Average disk read latency ms per database file", "gauge")
	writeHelp("dblens_disk_write_latency_ms", "Average disk write latency ms per database file", "gauge")
	for _, r := range results {
		if r.Resources != nil {
			for _, d := range r.Resources.DiskStats {
				fmt.Fprintf(&sb, `dblens_disk_read_latency_ms{server=%q,database=%q,file_type=%q} %.2f`+"\n",
					r.ServerName, d.Database, d.FileType, d.AvgReadMS)
				fmt.Fprintf(&sb, `dblens_disk_write_latency_ms{server=%q,database=%q,file_type=%q} %.2f`+"\n",
					r.ServerName, d.Database, d.FileType, d.AvgWriteMS)
			}
		}
	}

	// ── Replication ───────────────────────────────────────────────────────────
	writeHelp("dblens_ag_replication_lag_seconds", "Always On AG secondary replication lag", "gauge")
	for _, r := range results {
		if r.Replication != nil {
			for _, ag := range r.Replication.AGGroups {
				for _, rep := range ag.Replicas {
					fmt.Fprintf(&sb, `dblens_ag_replication_lag_seconds{server=%q,ag=%q,replica=%q} %d`+"\n",
						r.ServerName, ag.AGName, rep.ReplicaServer, rep.SecondaryLagSeconds)
				}
			}
		}
	}

	// ── Backup age ────────────────────────────────────────────────────────────
	writeHelp("dblens_backup_age_hours", "Hours since last full backup per database", "gauge")
	for _, r := range results {
		if r.Backups != nil {
			for _, b := range r.Backups.Databases {
				fmt.Fprintf(&sb, `dblens_backup_age_hours{server=%q,database=%q} %.1f`+"\n",
					r.ServerName, b.DatabaseName, b.HoursSinceFullBak)
			}
		}
	}

	// ── v5: OS Disk Volume Space
	appendDiskVolumeMetrics(&sb, results)

	// ── Scrape metadata ───────────────────────────────────────────────────────
	writeHelp("dblens_scrape_timestamp", "Unix timestamp of last successful scrape", "gauge")
	fmt.Fprintf(&sb, "dblens_scrape_timestamp %.0f\n", float64(time.Now().Unix()))

	fmt.Fprint(w, sb.String())
}

// init registers v5 disk volume metrics into the existing Handler via the
// helper below. Call appendDiskVolumeMetrics from Handler before writing.
func appendDiskVolumeMetrics(sb *strings.Builder, results map[string]*CollectionResult) {
	fmt.Fprintf(sb, "# HELP dblens_volume_free_pct OS disk volume free space percent\n")
	fmt.Fprintf(sb, "# TYPE dblens_volume_free_pct gauge\n")
	fmt.Fprintf(sb, "# HELP dblens_volume_free_gb OS disk volume free space GB\n")
	fmt.Fprintf(sb, "# TYPE dblens_volume_free_gb gauge\n")
	for _, r := range results {
		if r.DiskSpace == nil {
			continue
		}
		for _, v := range r.DiskSpace.Volumes {
			fmt.Fprintf(sb, `dblens_volume_free_pct{server=%q,volume=%q} %.1f`+"\n",
				r.ServerName, v.MountPoint, v.FreePct)
			fmt.Fprintf(sb, `dblens_volume_free_gb{server=%q,volume=%q} %.2f`+"\n",
				r.ServerName, v.MountPoint, v.FreeGB)
		}
	}
}

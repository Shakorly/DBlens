package main

import (
	"fmt"
	"math"
	"strings"
	"time"
)

// ── Advisory severity levels ───────────────────────────────────────────────────

type AdvisorySeverity string

const (
	SeverityCritical AdvisorySeverity = "CRITICAL" // data loss / outage risk
	SeverityHigh     AdvisorySeverity = "HIGH"     // significant performance impact
	SeverityMedium   AdvisorySeverity = "MEDIUM"   // degraded performance
	SeverityLow      AdvisorySeverity = "LOW"      // best practice / informational
)

// ── Advisory category ─────────────────────────────────────────────────────────

type AdvisoryCategory string

const (
	CategoryBlocking    AdvisoryCategory = "Blocking & Locking"
	CategoryQuery       AdvisoryCategory = "Query Performance"
	CategoryMemory      AdvisoryCategory = "Memory"
	CategoryCPU         AdvisoryCategory = "CPU"
	CategoryDisk        AdvisoryCategory = "Disk I/O"
	CategoryIndex       AdvisoryCategory = "Indexing"
	CategoryBackup      AdvisoryCategory = "Backup & Recovery"
	CategorySecurity    AdvisoryCategory = "Security"
	CategoryReplication AdvisoryCategory = "Replication / AG"
	CategoryTempDB      AdvisoryCategory = "TempDB"
	CategoryMaintenance AdvisoryCategory = "Maintenance"
	CategoryCapacity    AdvisoryCategory = "Capacity Planning"
	CategoryJobs        AdvisoryCategory = "SQL Agent Jobs"
	CategoryIntegrity   AdvisoryCategory = "Database Integrity"
)

// ── Advisory — the core output type ───────────────────────────────────────────

// Advisory is one actionable finding produced by the advisor engine.
// It goes far beyond a simple alert: it explains what is happening, why it
// matters, how to fix it, and provides copy-paste T-SQL where applicable.
type Advisory struct {
	ID              string           // unique stable ID (e.g. "BLK-001")
	ServerName      string
	Database        string
	Severity        AdvisorySeverity
	Category        AdvisoryCategory
	Title           string           // one-line summary
	WhatIsHappening string           // plain English: what the tool detected
	WhyItMatters    string           // business impact explanation
	RootCause       string           // likely technical cause
	HowToFix        string           // step-by-step remediation guide
	FixSQL          string           // copy-paste T-SQL fix script (if applicable)
	SafeToAutoFix   bool             // true = tool can run FixSQL automatically
	EstimatedImpact string           // "Estimated improvement: X% faster queries"
	References      []string         // links to docs / best practices
	DetectedAt      time.Time
	// Context fields for UI display
	AffectedObject  string           // table, index, database name
	MetricValue     string           // e.g. "42 blocked sessions"
	MetricThreshold string           // e.g. "threshold: 0"
}

// ── Advisor engine ─────────────────────────────────────────────────────────────

// GenerateAdvisories analyses a CollectionResult and produces a prioritised
// list of Advisory items — one per finding, fully explained with fix scripts.
func GenerateAdvisories(r *CollectionResult, cfg *Config) []Advisory {
	var advisories []Advisory

	add := func(a Advisory) {
		a.ServerName = r.ServerName
		a.DetectedAt = r.Timestamp
		advisories = append(advisories, a)
	}

	// ── BLOCKING ──────────────────────────────────────────────────────────────
	if r.Connections != nil && r.Connections.BlockedSessions > 0 {
		blocked := r.Connections.BlockedSessions

		// Find blocking chains
		blockers := map[int][]int{} // blocker sessionID → list of blocked sessionIDs
		for _, s := range r.Connections.Sessions {
			if s.BlockingSession != 0 {
				blockers[s.BlockingSession] = append(blockers[s.BlockingSession], s.SessionID)
			}
		}

		var blockingSQL strings.Builder
		blockingSQL.WriteString("-- ① Identify who is blocking (run first)\n")
		blockingSQL.WriteString(`SELECT
    blocking.session_id        AS blocker_session,
    blocking.login_name        AS blocker_login,
    blocking.host_name         AS blocker_host,
    blocking.status            AS blocker_status,
    SUBSTRING(bt.text, (br.statement_start_offset/2)+1,
        ((CASE br.statement_end_offset WHEN -1 THEN DATALENGTH(bt.text)
          ELSE br.statement_end_offset END - br.statement_start_offset)/2)+1)
                               AS blocker_query,
    blocked.session_id         AS blocked_session,
    blocked.login_name         AS blocked_login,
    SUBSTRING(t.text, (r.statement_start_offset/2)+1,
        ((CASE r.statement_end_offset WHEN -1 THEN DATALENGTH(t.text)
          ELSE r.statement_end_offset END - r.statement_start_offset)/2)+1)
                               AS blocked_query,
    r.wait_type,
    r.wait_time / 1000.0      AS wait_seconds
FROM sys.dm_exec_sessions    blocking
JOIN sys.dm_exec_sessions    blocked  ON blocked.blocking_session_id = blocking.session_id
JOIN sys.dm_exec_requests    r        ON r.session_id = blocked.session_id
OUTER APPLY sys.dm_exec_sql_text(r.sql_handle) t
LEFT JOIN sys.dm_exec_requests br     ON br.session_id = blocking.session_id
OUTER APPLY sys.dm_exec_sql_text(br.sql_handle) bt
ORDER BY r.wait_time DESC;

`)
		blockingSQL.WriteString("-- ② Kill the blocking session (replace 99 with actual blocker session_id)\n")
		blockingSQL.WriteString("-- WARNING: Only kill if you are sure the transaction can be safely rolled back!\n")
		blockingSQL.WriteString("-- KILL 99;\n\n")
		blockingSQL.WriteString("-- ③ Prevent future blocking — add READ_COMMITTED_SNAPSHOT\n")
		blockingSQL.WriteString("-- ALTER DATABASE [YourDB] SET READ_COMMITTED_SNAPSHOT ON WITH ROLLBACK IMMEDIATE;\n")

		sev := SeverityHigh
		if blocked >= 5 {
			sev = SeverityCritical
		}

		add(Advisory{
			ID:       "BLK-001",
			Severity: sev,
			Category: CategoryBlocking,
			Title:    fmt.Sprintf("%d session(s) blocked — active lock contention", blocked),
			WhatIsHappening: fmt.Sprintf(
				"%d sessions are waiting for locks held by other sessions. "+
					"The longest wait is held by session %d.",
				blocked, findLongestBlocker(r.Connections.Sessions)),
			WhyItMatters: "Blocked sessions mean user requests are queued and not being processed. " +
				"In a web application this appears as slow page loads or timeouts. " +
				"If the blocking session holds a transaction, every second of delay compounds.",
			RootCause: "A transaction is holding a lock longer than expected. Common causes: " +
				"(1) Long-running transaction not committed, " +
				"(2) Missing index causing a full table scan to acquire too many row locks, " +
				"(3) Default READ COMMITTED isolation mode causing reader-writer conflicts.",
			HowToFix: "1. Run the diagnostic SQL below to identify the blocker and their query.\n" +
				"2. Determine if the blocking transaction can be killed (check with the application team).\n" +
				"3. Long-term: enable READ_COMMITTED_SNAPSHOT isolation to eliminate most reader-writer blocks.\n" +
				"4. Add missing indexes on heavily-filtered columns to reduce lock scope.",
			FixSQL:          blockingSQL.String(),
			SafeToAutoFix:   false, // killing sessions requires human judgement
			EstimatedImpact: "Resolving blocking can immediately restore normal response times for all queued sessions.",
			AffectedObject:  "Active sessions",
			MetricValue:     fmt.Sprintf("%d blocked sessions", blocked),
			MetricThreshold: "threshold: 0",
			References: []string{
				"https://learn.microsoft.com/en-us/sql/relational-databases/sql-server-transaction-locking-and-row-versioning-guide",
				"https://learn.microsoft.com/en-us/sql/t-sql/statements/alter-database-transact-sql-set-options#read_committed_snapshot",
			},
		})
	}

	// ── LONG-RUNNING QUERIES ──────────────────────────────────────────────────
	if r.Queries != nil {
		for i, q := range r.Queries.ActiveLongRunning {
			if i >= 3 { break } // top 3 only
			elapsedSec := q.ElapsedMS / 1000

			var fixSQL string
			var rootCause string
			var howToFix string

			// Diagnose by wait type
			switch {
			case strings.HasPrefix(q.WaitType, "LCK_"):
				rootCause = fmt.Sprintf(
					"This query is waiting on lock type %s — it is blocked by another session "+
						"holding a conflicting lock on a resource it needs.", q.WaitType)
				howToFix = "1. Identify who holds the lock (see BLK-001 fix SQL).\n" +
					"2. Add an appropriate index to reduce the lock scope.\n" +
					"3. Shorten the transaction that owns the lock."
				fixSQL = fmt.Sprintf(`-- Find what this session is waiting for
SELECT
    session_id, wait_type, wait_time/1000.0 AS wait_sec,
    blocking_session_id, resource_description
FROM sys.dm_exec_requests
WHERE session_id = %d;`, q.SessionID)

			case q.WaitType == "ASYNC_NETWORK_IO":
				rootCause = "The server has finished computing the result but the CLIENT is not " +
					"consuming rows fast enough. The query itself may be fast — the bottleneck is network or client processing."
				howToFix = "1. Add TOP/pagination to limit result set size.\n" +
					"2. Move data processing server-side (stored procedures, CTEs).\n" +
					"3. Check client application for slow result-set iteration.\n" +
					"4. Consider returning aggregated results instead of raw rows."
				fixSQL = fmt.Sprintf(`-- Check how many rows this session has returned
SELECT session_id, row_count, reads, logical_reads
FROM sys.dm_exec_sessions WHERE session_id = %d;`, q.SessionID)

			case q.WaitType == "CXPACKET" || q.WaitType == "CXCONSUMER":
				rootCause = "Query is using parallelism (running on multiple CPU cores). " +
					"One thread is waiting for others to finish — this can indicate skewed data distribution or suboptimal MAXDOP setting."
				howToFix = "1. Check statistics are up-to-date: UPDATE STATISTICS [table] WITH FULLSCAN.\n" +
					"2. Consider limiting parallelism: add OPTION(MAXDOP 1) to isolate the query.\n" +
					"3. Set server-level MAXDOP to half the logical CPU count (up to 8).\n" +
					"4. Set Cost Threshold for Parallelism to 50 (default 5 is too low)."
				fixSQL = fmt.Sprintf(`-- Check and fix parallelism settings
SELECT name, value_in_use FROM sys.configurations
WHERE name IN ('max degree of parallelism', 'cost threshold for parallelism');

-- Recommended: set MAXDOP to half CPU count (max 8) and threshold to 50
-- EXEC sp_configure 'max degree of parallelism', 4; RECONFIGURE;
-- EXEC sp_configure 'cost threshold for parallelism', 50; RECONFIGURE;

-- Test query with no parallelism (add to your query):
-- SELECT ... OPTION (MAXDOP 1);`)

			case strings.HasPrefix(q.WaitType, "PAGEIOLATCH"):
				rootCause = "Query is waiting for data pages to be read from disk into the buffer pool. " +
					"This means either: data is not cached (cold buffer pool), or there is insufficient memory, " +
					"or there are missing indexes causing excessive table scans."
				howToFix = "1. Add indexes to eliminate full table scans.\n" +
					"2. Increase SQL Server max memory if available RAM allows.\n" +
					"3. Check disk I/O performance — SSDs dramatically improve this.\n" +
					"4. Verify buffer pool is not being pressured by other processes."
				fixSQL = `-- Check buffer pool memory pressure
SELECT
    physical_memory_in_use_kb/1024   AS sql_mem_mb,
    page_fault_count,
    memory_utilization_percentage
FROM sys.dm_os_process_memory;

-- Check top tables by size (largest tables most likely to cause PAGEIOLATCH)
SELECT TOP 10
    OBJECT_NAME(i.object_id)      AS table_name,
    SUM(a.total_pages) * 8 / 1024 AS total_mb
FROM sys.indexes i
JOIN sys.partitions p ON i.object_id = p.object_id AND i.index_id = p.index_id
JOIN sys.allocation_units a ON p.partition_id = a.container_id
GROUP BY i.object_id ORDER BY total_mb DESC;`

			case q.WaitType == "WRITELOG":
				rootCause = "Query is waiting for the transaction log to be written to disk. " +
					"This indicates either: high write volume, slow disk for the log file, " +
					"or the log file is on the same disk as the data files."
				howToFix = "1. Move the transaction log to a separate, fast disk (SSD preferred).\n" +
					"2. Reduce unnecessary writes — check for row-by-row operations (RBAR).\n" +
					"3. Use batch operations instead of individual inserts/updates.\n" +
					"4. Check if log backup is running frequently enough."
				fixSQL = `-- Check log file location and size
SELECT name, physical_name, size*8/1024 AS size_mb, type_desc
FROM sys.master_files
WHERE type = 1; -- 1 = log files

-- Check log space usage
DBCC SQLPERF(LOGSPACE);`

			default:
				rootCause = fmt.Sprintf(
					"Query has been running for %d seconds with wait type '%s'. "+
						"This wait type indicates the query is spending time waiting for a resource.",
					elapsedSec, q.WaitType)
				howToFix = "1. Capture the execution plan: SET STATISTICS IO, TIME ON; then run the query.\n" +
					"2. Look for Table Scan operators in the plan — these need indexes.\n" +
					"3. Check for implicit type conversions which prevent index usage.\n" +
					"4. Consider rewriting the query to reduce logical reads."
				fixSQL = fmt.Sprintf(`-- Capture query plan for session %d
SELECT qp.query_plan
FROM sys.dm_exec_requests r
CROSS APPLY sys.dm_exec_query_plan(r.plan_handle) qp
WHERE r.session_id = %d;`, q.SessionID, q.SessionID)
			}

			sev := SeverityMedium
			if elapsedSec > 60 {
				sev = SeverityHigh
			}
			if elapsedSec > 300 {
				sev = SeverityCritical
			}

			add(Advisory{
				ID:       fmt.Sprintf("QRY-%03d", i+1),
				Severity: sev,
				Category: CategoryQuery,
				Database: q.Database,
				Title: fmt.Sprintf("Long-running query: %ds elapsed (session %d, db=%s)",
					elapsedSec, q.SessionID, q.Database),
				WhatIsHappening: fmt.Sprintf(
					"Session %d (login: %s, host: %s) has been executing for %d seconds. "+
						"Wait type: %s. CPU consumed: %d ms.",
					q.SessionID, q.LoginName, q.HostName, elapsedSec, q.WaitType, q.CPUTime),
				WhyItMatters: "Long-running queries consume CPU and memory, hold locks that block other sessions, " +
					"and degrade response times for all users of the database.",
				RootCause:       rootCause,
				HowToFix:        howToFix,
				FixSQL:          fixSQL,
				SafeToAutoFix:   false,
				EstimatedImpact: "Resolving the root cause can reduce query time by 50–99% depending on the fix.",
				AffectedObject:  fmt.Sprintf("Session %d in %s", q.SessionID, q.Database),
				MetricValue:     fmt.Sprintf("%d seconds elapsed", elapsedSec),
				MetricThreshold: fmt.Sprintf("threshold: %d ms", cfg.SlowQueryThresholdMS),
				References: []string{
					"https://learn.microsoft.com/en-us/sql/relational-databases/system-dynamic-management-views/sys-dm-exec-requests-transact-sql",
				},
			})
		}
	}

	// ── MEMORY PRESSURE ───────────────────────────────────────────────────────
	if r.Resources != nil && r.Resources.TotalMemoryMB > 0 {
		availPct := float64(r.Resources.AvailableMemoryMB) / float64(r.Resources.TotalMemoryMB) * 100
		if availPct < float64(cfg.MemoryAlertThresholdPct) {
			maxMemSQL := int64(r.Resources.TotalMemoryMB) * 85 / 100 // leave 15% for OS

			add(Advisory{
				ID:       "MEM-001",
				Severity: SeverityHigh,
				Category: CategoryMemory,
				Title:    fmt.Sprintf("Memory pressure: only %.1f%% RAM available", availPct),
				WhatIsHappening: fmt.Sprintf(
					"Only %d MB (%.1f%%) of %d MB total RAM is available. "+
						"SQL Server is using %d MB. Memory state: %s.",
					r.Resources.AvailableMemoryMB, availPct,
					r.Resources.TotalMemoryMB, r.Resources.SQLMemoryMB,
					r.Resources.MemoryStateDesc),
				WhyItMatters: "Insufficient free memory causes the buffer pool to evict pages, " +
					"forcing disk reads for queries that previously ran from cache. " +
					"This can slow queries by 10–100x and increase disk I/O dramatically.",
				RootCause: "Possible causes: (1) SQL Server max memory not capped — consuming too much, " +
					"(2) Memory leak in an application, " +
					"(3) Other processes competing for RAM, " +
					"(4) Insufficient RAM for the workload.",
				HowToFix: fmt.Sprintf(
					"1. Cap SQL Server max memory to leave ~15%% for the OS (~%d MB recommended).\n"+
						"2. Check for memory-intensive queries (high logical reads).\n"+
						"3. Look for memory-consuming CLR or linked server operations.\n"+
						"4. Consider adding RAM if the workload genuinely requires it.",
					maxMemSQL),
				FixSQL: fmt.Sprintf(`-- Step 1: Check current SQL Server memory settings
SELECT name, value_in_use FROM sys.configurations
WHERE name IN ('max server memory (MB)', 'min server memory (MB)');

-- Step 2: Set max server memory (leave ~15%% for OS)
-- Recommended value: %d MB based on your %d MB total RAM
EXEC sp_configure 'show advanced options', 1; RECONFIGURE;
EXEC sp_configure 'max server memory (MB)', %d; RECONFIGURE;

-- Step 3: Find top memory-consuming queries
SELECT TOP 10
    qs.total_grant_kb / 1024       AS total_grant_mb,
    qs.execution_count,
    SUBSTRING(qt.text, 1, 200)     AS query_text
FROM sys.dm_exec_query_stats qs
CROSS APPLY sys.dm_exec_sql_text(qs.sql_handle) qt
ORDER BY total_grant_mb DESC;`,
					maxMemSQL, r.Resources.TotalMemoryMB, maxMemSQL),
				SafeToAutoFix:   false,
				EstimatedImpact: "Capping max memory frees RAM for the OS and prevents paging. Queries already in buffer pool remain fast.",
				AffectedObject:  "SQL Server memory configuration",
				MetricValue:     fmt.Sprintf("%.1f%% available (%d MB)", availPct, r.Resources.AvailableMemoryMB),
				MetricThreshold: fmt.Sprintf("threshold: %d%%", cfg.MemoryAlertThresholdPct),
				References: []string{
					"https://learn.microsoft.com/en-us/sql/database-engine/configure-windows/server-memory-server-configuration-options",
				},
			})
		}
	}

	// ── HIGH CPU ──────────────────────────────────────────────────────────────
	if r.Resources != nil && r.Resources.SQLCPUPercent > cfg.CPUAlertThresholdPct {
		add(Advisory{
			ID:       "CPU-001",
			Severity: SeverityHigh,
			Category: CategoryCPU,
			Title:    fmt.Sprintf("High SQL Server CPU: %d%%", r.Resources.SQLCPUPercent),
			WhatIsHappening: fmt.Sprintf(
				"SQL Server is consuming %d%% CPU (system total: %d%%). "+
					"This is above the %d%% alert threshold.",
				r.Resources.SQLCPUPercent, r.Resources.SystemCPUPercent, cfg.CPUAlertThresholdPct),
			WhyItMatters: "High CPU prevents new queries from starting promptly, " +
				"increases latency for all users, and can cause query timeouts. " +
				"Sustained high CPU may indicate a runaway query or misconfiguration.",
			RootCause: "Common causes: (1) Missing indexes causing full table scans, " +
				"(2) Implicit type conversions invalidating index usage, " +
				"(3) Excessive recompilations, " +
				"(4) Parallelism (CXPACKET waits), " +
				"(5) A specific runaway query.",
			HowToFix: "1. Identify the top CPU queries (see fix SQL below).\n" +
				"2. Add missing indexes to eliminate table scans.\n" +
				"3. Check for parameter sniffing issues (recompilation spikes).\n" +
				"4. Verify MAXDOP and Cost Threshold for Parallelism settings.\n" +
				"5. Check for excessive ad-hoc query compilation — enable optimize for ad-hoc workloads.",
			FixSQL: `-- Top CPU-consuming queries right now
SELECT TOP 10
    r.session_id,
    r.cpu_time,
    r.total_elapsed_time / 1000    AS elapsed_sec,
    DB_NAME(r.database_id)         AS database_name,
    s.login_name,
    SUBSTRING(t.text, (r.statement_start_offset/2)+1, 300) AS query_text
FROM sys.dm_exec_requests r
JOIN sys.dm_exec_sessions s ON r.session_id = s.session_id
CROSS APPLY sys.dm_exec_sql_text(r.sql_handle) t
WHERE s.is_user_process = 1
ORDER BY r.cpu_time DESC;

-- Top CPU queries from plan cache (historical)
SELECT TOP 10
    total_worker_time/1000         AS total_cpu_ms,
    execution_count,
    total_worker_time/execution_count/1000 AS avg_cpu_ms,
    SUBSTRING(qt.text, 1, 200)     AS query_text
FROM sys.dm_exec_query_stats qs
CROSS APPLY sys.dm_exec_sql_text(qs.sql_handle) qt
ORDER BY total_worker_time DESC;

-- Enable optimize for ad-hoc workloads (reduces plan cache bloat)
-- EXEC sp_configure 'optimize for ad hoc workloads', 1; RECONFIGURE;`,
			SafeToAutoFix:   false,
			EstimatedImpact: "Adding one good index can reduce CPU by 50–80% on the affected queries.",
			AffectedObject:  "SQL Server CPU",
			MetricValue:     fmt.Sprintf("%d%%", r.Resources.SQLCPUPercent),
			MetricThreshold: fmt.Sprintf("threshold: %d%%", cfg.CPUAlertThresholdPct),
		})
	}

	// ── DISK I/O LATENCY ─────────────────────────────────────────────────────
	if r.Resources != nil {
		for _, d := range r.Resources.DiskStats {
			if d.AvgReadMS > cfg.DiskIOAlertMS*2 || d.AvgWriteMS > cfg.DiskIOAlertMS*2 {
				worstMS := math.Max(d.AvgReadMS, d.AvgWriteMS)
				sev := SeverityMedium
				if worstMS > 100 {
					sev = SeverityHigh
				}
				if worstMS > 200 {
					sev = SeverityCritical
				}
				add(Advisory{
					ID:       "DISK-001",
					Severity: sev,
					Category: CategoryDisk,
					Database: d.Database,
					Title:    fmt.Sprintf("High disk I/O latency: db=%s read=%.0fms write=%.0fms", d.Database, d.AvgReadMS, d.AvgWriteMS),
					WhatIsHappening: fmt.Sprintf(
						"Database '%s' (%s file) has avg read latency of %.1fms and write latency of %.1fms. "+
							"Acceptable latency for OLTP is <5ms read, <2ms write.",
						d.Database, d.FileType, d.AvgReadMS, d.AvgWriteMS),
					WhyItMatters: "High disk latency directly translates to slow queries. " +
						"Every PAGEIOLATCH wait is a thread blocked waiting for this disk. " +
						"At 100ms per read, a query touching 1000 pages takes 100 seconds in the worst case.",
					RootCause: "Causes in order of likelihood: " +
						"(1) HDD instead of SSD — the single biggest fix available, " +
						"(2) Data and log files sharing a disk, " +
						"(3) RAID controller cache disabled or failing, " +
						"(4) Storage array contention from other VMs (on Azure: burst limit exceeded), " +
						"(5) Missing indexes causing excessive full scans.",
					HowToFix: "1. Move database files to SSD storage (the single most impactful change).\n" +
						"2. Separate data files and log files onto different volumes.\n" +
						"3. On Azure VM: check if you're hitting IOPS limits — upgrade disk tier or enable disk caching.\n" +
						"4. Add missing indexes to reduce the number of pages read per query.\n" +
						"5. Increase SQL Server buffer pool memory to cache more pages.",
					FixSQL: fmt.Sprintf(`-- Current I/O stats for all files (cumulative since SQL restart)
SELECT
    DB_NAME(vfs.database_id)                      AS database_name,
    mf.physical_name,
    mf.type_desc,
    vfs.num_of_reads,
    vfs.num_of_writes,
    CASE WHEN vfs.num_of_reads = 0 THEN 0
         ELSE vfs.io_stall_read_ms / vfs.num_of_reads END  AS avg_read_ms,
    CASE WHEN vfs.num_of_writes = 0 THEN 0
         ELSE vfs.io_stall_write_ms / vfs.num_of_writes END AS avg_write_ms
FROM sys.dm_io_virtual_file_stats(NULL, NULL) vfs
JOIN sys.master_files mf
    ON vfs.database_id = mf.database_id AND vfs.file_id = mf.file_id
WHERE DB_NAME(vfs.database_id) = '%s'
ORDER BY (vfs.io_stall_read_ms + vfs.io_stall_write_ms) DESC;

-- Check buffer pool hit ratio (should be > 99%%)
SELECT
    cntr_value AS buffer_cache_hit_ratio
FROM sys.dm_os_performance_counters
WHERE counter_name = 'Buffer cache hit ratio'
  AND object_name LIKE '%%Buffer Manager%%';`, d.Database),
					SafeToAutoFix:   false,
					EstimatedImpact: "Moving to SSD typically reduces latency by 10–50x. Adding indexes reduces I/O volume.",
					AffectedObject:  d.PhysicalName,
					MetricValue:     fmt.Sprintf("read=%.1fms write=%.1fms", d.AvgReadMS, d.AvgWriteMS),
					MetricThreshold: fmt.Sprintf("threshold: %.0fms", cfg.DiskIOAlertMS),
				})
			}
		}
	}

	// ── MISSING INDEXES ───────────────────────────────────────────────────────
	if r.Indexes != nil {
		for i, ix := range r.Indexes.MissingIndexes {
			if i >= 3 { break }
			sev := SeverityLow
			if ix.ImpactScore > 100000 {
				sev = SeverityHigh
			} else if ix.ImpactScore > 10000 {
				sev = SeverityMedium
			}

			// Build the actual CREATE INDEX statement
			var cols []string
			if ix.EqualityColumns != "" {
				cols = append(cols, ix.EqualityColumns)
			}
			if ix.InequalityColumns != "" {
				cols = append(cols, ix.InequalityColumns)
			}
			allCols := strings.Join(cols, ", ")
			idxName := fmt.Sprintf("IX_%s_%s",
				strings.ReplaceAll(ix.TableName, ".", "_"),
				strings.ReplaceAll(strings.ReplaceAll(allCols, "[", ""), "]", ""))
			if len(idxName) > 100 {
				idxName = idxName[:100]
			}

			createStmt := fmt.Sprintf(
				"CREATE NONCLUSTERED INDEX [%s]\n    ON %s (%s)",
				idxName, ix.TableName, allCols)
			if ix.IncludeColumns != "" {
				createStmt += fmt.Sprintf("\n    INCLUDE (%s)", ix.IncludeColumns)
			}
			createStmt += "\n    WITH (ONLINE = ON, FILLFACTOR = 90);"

			add(Advisory{
				ID:       fmt.Sprintf("IDX-%03d", i+1),
				Severity: sev,
				Category: CategoryIndex,
				Database: ix.Database,
				Title: fmt.Sprintf("Missing index on %s.%s (impact score: %.0f)",
					ix.Database, ix.TableName, ix.ImpactScore),
				WhatIsHappening: fmt.Sprintf(
					"SQL Server's query optimizer has identified a missing index on table %s in database %s. "+
						"This index has been sought %d times and scanned %d times without existing.",
					ix.TableName, ix.Database, ix.UserSeeks, ix.UserScans),
				WhyItMatters: fmt.Sprintf(
					"Without this index, every query on these columns performs a full table scan. "+
						"Impact score %.0f means this index would significantly reduce query costs. "+
						"Each seek/scan without the index wastes CPU and disk I/O.",
					ix.ImpactScore),
				RootCause: "The table is being queried on columns that have no supporting index. " +
					"SQL Server has to read every row in the table to find matching records.",
				HowToFix: "1. Review the CREATE INDEX statement below.\n" +
					"2. Test in a non-production environment first.\n" +
					"3. Run during off-peak hours if ONLINE=ON is not available (non-Enterprise editions).\n" +
					"4. After creating, run UPDATE STATISTICS on the table.",
				FixSQL: fmt.Sprintf(`-- Estimated improvement: high-impact index
-- Test this in non-production first. ONLINE=ON requires Enterprise Edition.
-- Run during off-peak hours if ONLINE=ON is not available.

USE [%s];
GO

%s
GO

-- After creating index, update statistics
UPDATE STATISTICS %s WITH FULLSCAN;
GO`,
					ix.Database, createStmt, ix.TableName),
				SafeToAutoFix:   false,
				EstimatedImpact: fmt.Sprintf("Estimated %.0f impact units — queries may run 10–100x faster after this index.", ix.ImpactScore),
				AffectedObject:  ix.TableName,
				MetricValue:     fmt.Sprintf("impact=%.0f seeks=%d", ix.ImpactScore, ix.UserSeeks),
			})
		}
	}

	// ── FRAGMENTED INDEXES ────────────────────────────────────────────────────
	if r.Indexes != nil {
		for i, fi := range r.Indexes.FragmentedIndexes {
			if i >= 5 { break }
			var action, sql string
			if fi.FragmentationPct >= 30 {
				action = "REBUILD"
				sql = fmt.Sprintf(`-- Index fragmentation: %.1f%% — REBUILD recommended (>30%%)
USE [%s];
GO
ALTER INDEX [%s] ON %s REBUILD WITH (ONLINE = ON, FILLFACTOR = 90);
-- Note: ONLINE=ON requires Enterprise Edition. Remove if using Standard/Express.`,
					fi.FragmentationPct, fi.Database, fi.IndexName, fi.TableName)
			} else {
				action = "REORGANIZE"
				sql = fmt.Sprintf(`-- Index fragmentation: %.1f%% — REORGANIZE recommended (10-30%%)
USE [%s];
GO
ALTER INDEX [%s] ON %s REORGANIZE;
UPDATE STATISTICS %s WITH FULLSCAN;`,
					fi.FragmentationPct, fi.Database, fi.IndexName, fi.TableName, fi.TableName)
			}

			add(Advisory{
				ID:       fmt.Sprintf("IDX-F%02d", i+1),
				Severity: SeverityLow,
				Category: CategoryIndex,
				Database: fi.Database,
				Title:    fmt.Sprintf("Fragmented index: %s on %s (%.0f%% — %s)", fi.IndexName, fi.TableName, fi.FragmentationPct, action),
				WhatIsHappening: fmt.Sprintf(
					"Index '%s' on table '%s' in database '%s' is %.1f%% fragmented (%d pages). "+
						"Recommended action: %s.",
					fi.IndexName, fi.TableName, fi.Database, fi.FragmentationPct, fi.PageCount, action),
				WhyItMatters: "Fragmented indexes cause SQL Server to perform more I/O per query " +
					"because data pages are not in the optimal sequential order. " +
					"This wastes disk bandwidth and increases read latency.",
				RootCause: "Fragmentation builds up over time from INSERT, UPDATE, and DELETE operations " +
					"that split index pages and leave gaps. High-churn tables fragment quickly.",
				HowToFix: fmt.Sprintf(
					"Run the %s script below. Schedule this during a maintenance window.\n"+
						"Set up a weekly index maintenance job using Ola Hallengren's solution (free):\n"+
						"https://ola.hallengren.com/sql-server-index-and-statistics-maintenance.html",
					action),
				FixSQL:          sql,
				SafeToAutoFix:   true, // REORGANIZE is online and safe to auto-run
				EstimatedImpact: fmt.Sprintf("Reduces I/O by up to %.0f%% on queries using this index.", fi.FragmentationPct*0.7),
				AffectedObject:  fmt.Sprintf("%s.%s", fi.Database, fi.IndexName),
				MetricValue:     fmt.Sprintf("%.1f%% fragmented, %d pages", fi.FragmentationPct, fi.PageCount),
			})
		}
	}

	// ── BACKUP OVERDUE ────────────────────────────────────────────────────────
	if r.Backups != nil {
		for _, db := range r.Backups.Databases {
			if !db.IsAlertFull { continue }
			sev := SeverityHigh
			if db.HoursSinceFullBak > 72 {
				sev = SeverityCritical
			}
			add(Advisory{
				ID:       "BAK-001",
				Severity: sev,
				Category: CategoryBackup,
				Database: db.DatabaseName,
				Title: fmt.Sprintf("Backup overdue: %s (%.0f hours since last full backup)",
					db.DatabaseName, db.HoursSinceFullBak),
				WhatIsHappening: fmt.Sprintf(
					"Database '%s' (recovery model: %s) has not had a full backup in %.1f hours. "+
						"Your threshold is %.0f hours.",
					db.DatabaseName, db.RecoveryModel, db.HoursSinceFullBak, cfg.BackupFullAlertHours),
				WhyItMatters: "Without a recent backup, a hardware failure, accidental deletion, " +
					"or ransomware attack could result in permanent data loss. " +
					"Your recovery point objective (RPO) is being violated right now.",
				RootCause: "Possible causes: (1) Backup job failed or was disabled, " +
					"(2) Backup destination is full, " +
					"(3) Backup was recently deleted without a new one being taken.",
				HowToFix: "1. Run an immediate full backup (script below).\n" +
					"2. Check SQL Agent for failed backup jobs.\n" +
					"3. Verify the backup destination has sufficient free space.\n" +
					"4. Set up automated backup monitoring alerts.",
				FixSQL: fmt.Sprintf(`-- Immediate full backup — run NOW
BACKUP DATABASE [%s]
TO DISK = N'C:\Backups\%s_emergency_%s.bak'
WITH COMPRESSION, CHECKSUM, STATS = 10;
GO

-- Verify the backup is readable
RESTORE VERIFYONLY
FROM DISK = N'C:\Backups\%s_emergency_%s.bak';
GO`,
					db.DatabaseName, db.DatabaseName,
					time.Now().Format("20060102_150405"),
					db.DatabaseName, time.Now().Format("20060102_150405")),
				SafeToAutoFix:   false,
				EstimatedImpact: "A backup taken now limits data loss to the current moment.",
				AffectedObject:  db.DatabaseName,
				MetricValue:     fmt.Sprintf("%.0f hours since last backup", db.HoursSinceFullBak),
				MetricThreshold: fmt.Sprintf("threshold: %.0f hours", cfg.BackupFullAlertHours),
			})
		}
	}

	// ── SECURITY RISKS ────────────────────────────────────────────────────────
	if r.Security != nil {
		if r.Security.XPCmdShellOn {
			add(Advisory{
				ID:       "SEC-001",
				Severity: SeverityCritical,
				Category: CategorySecurity,
				Title:    "xp_cmdshell is ENABLED — critical security risk",
				WhatIsHappening: "The xp_cmdshell extended stored procedure is enabled. " +
					"This allows any sysadmin to execute operating system commands directly from SQL Server.",
				WhyItMatters: "xp_cmdshell is a primary attack vector for SQL injection attacks. " +
					"If an attacker gains SQL access, they can immediately gain OS-level control, " +
					"exfiltrate data, install ransomware, or create backdoors.",
				RootCause: "xp_cmdshell was explicitly enabled — it is OFF by default. " +
					"Applications or DBAs may have enabled it for automation tasks. " +
					"These tasks should be replaced with SQL Agent jobs or PowerShell.",
				HowToFix: "1. Disable xp_cmdshell immediately (script below).\n" +
					"2. Find what uses it: search stored procedures and application code.\n" +
					"3. Replace xp_cmdshell usage with SQL Agent jobs (CmdExec steps) or PowerShell.",
				FixSQL: `-- Disable xp_cmdshell immediately
EXEC sp_configure 'show advanced options', 1; RECONFIGURE;
EXEC sp_configure 'xp_cmdshell', 0; RECONFIGURE;
EXEC sp_configure 'show advanced options', 0; RECONFIGURE;

-- Verify it is disabled
SELECT name, value_in_use FROM sys.configurations WHERE name = 'xp_cmdshell';
-- value_in_use should be 0`,
				SafeToAutoFix:   false,
				EstimatedImpact: "Disabling xp_cmdshell eliminates a critical attack surface immediately.",
				AffectedObject:  "SQL Server configuration",
			})
		}

		if r.Security.SALoginEnabled {
			add(Advisory{
				ID:       "SEC-002",
				Severity: SeverityHigh,
				Category: CategorySecurity,
				Title:    "SA login is enabled — consider disabling or renaming",
				WhatIsHappening: "The 'sa' (System Administrator) login is active. " +
					"This well-known account is a primary target for brute-force attacks.",
				WhyItMatters: "Attackers specifically target the 'sa' account because its name is always the same. " +
					"A weak password on sa = full server compromise.",
				RootCause: "SA is enabled by default in mixed authentication mode. " +
					"It should be disabled unless specifically required.",
				HowToFix: "1. Rename sa to a non-obvious name (script below).\n" +
					"2. Set a very strong password.\n" +
					"3. Disable it if no application uses it directly.",
				FixSQL: `-- Option 1: Rename SA (preferred)
ALTER LOGIN [sa] WITH NAME = [sql_sa_renamed];

-- Option 2: Disable SA if not used  
ALTER LOGIN [sa] DISABLE;

-- Option 3: Set a very strong password
ALTER LOGIN [sa] WITH PASSWORD = 'Use-A-Very-Long-Random-Password-Here!';`,
				SafeToAutoFix:   false,
				EstimatedImpact: "Disabling/renaming sa eliminates the most commonly targeted attack vector.",
				AffectedObject:  "SA login",
			})
		}
	}

	// ── AG REPLICATION LAG ────────────────────────────────────────────────────
	if r.Replication != nil {
		for _, ag := range r.Replication.AGGroups {
			for _, rep := range ag.Replicas {
				if rep.SecondaryLagSeconds > cfg.ReplicationLagAlertSeconds {
					sev := SeverityMedium
					if rep.SecondaryLagSeconds > cfg.ReplicationLagAlertSeconds*3 {
						sev = SeverityHigh
					}
					add(Advisory{
						ID:       "AG-001",
						Severity: sev,
						Category: CategoryReplication,
						Title:    fmt.Sprintf("AG replication lag: %s on %s (%ds)", ag.AGName, rep.ReplicaServer, rep.SecondaryLagSeconds),
						WhatIsHappening: fmt.Sprintf(
							"Secondary replica '%s' in AG '%s' is %d seconds behind the primary. "+
								"Log send queue: %d KB, Redo queue: %d KB.",
							rep.ReplicaServer, ag.AGName, rep.SecondaryLagSeconds,
							rep.LogSendQueueKB, rep.RedoQueueKB),
						WhyItMatters: "If the primary fails, the secondary will be missing the last " +
							fmt.Sprintf("%d seconds", rep.SecondaryLagSeconds) +
							" of transactions. In synchronous mode, this means data loss. " +
							"In asynchronous mode, it violates your RPO.",
						RootCause: "Likely causes: (1) Network latency between primary and secondary, " +
							"(2) Secondary server has insufficient I/O throughput to apply redo log, " +
							"(3) Heavy write workload on primary generating too much log.",
						HowToFix: "1. Check network connectivity between primary and secondary.\n" +
							"2. Monitor secondary redo thread performance.\n" +
							"3. Reduce write workload on the primary if possible.\n" +
							"4. Upgrade network bandwidth between AG nodes.",
						FixSQL: fmt.Sprintf(`-- Detailed AG replica health check
SELECT
    ag.name                              AS ag_name,
    ar.replica_server_name,
    ars.role_desc,
    ars.operational_state_desc,
    ars.synchronization_health_desc,
    adbrs.secondary_lag_seconds,
    adbrs.log_send_queue_size            AS log_send_queue_kb,
    adbrs.redo_queue_size                AS redo_queue_kb,
    adbrs.log_send_rate                  AS log_send_rate_kb_sec,
    adbrs.redo_rate                      AS redo_rate_kb_sec
FROM sys.availability_groups ag
JOIN sys.availability_replicas ar ON ag.group_id = ar.group_id
JOIN sys.dm_hadr_availability_replica_states ars ON ar.replica_id = ars.replica_id
LEFT JOIN sys.dm_hadr_database_replica_states adbrs ON ars.replica_id = adbrs.replica_id
WHERE ag.name = '%s'
ORDER BY adbrs.secondary_lag_seconds DESC;`, ag.AGName),
						SafeToAutoFix:   false,
						EstimatedImpact: "Resolving lag brings secondary up to date and restores your RPO.",
						AffectedObject:  rep.ReplicaServer,
						MetricValue:     fmt.Sprintf("%d seconds lag", rep.SecondaryLagSeconds),
						MetricThreshold: fmt.Sprintf("threshold: %d seconds", cfg.ReplicationLagAlertSeconds),
					})
				}
			}
		}
	}

	// ── DEADLOCKS ─────────────────────────────────────────────────────────────
	for i, dl := range r.Deadlocks {
		if i >= 2 { break }
		add(Advisory{
			ID:       fmt.Sprintf("DLK-%03d", i+1),
			Severity: SeverityHigh,
			Category: CategoryBlocking,
			Title:    fmt.Sprintf("Deadlock at %s (victim SPID %d)", dl.OccurredAt.Format("15:04:05"), dl.VictimSPID),
			WhatIsHappening: fmt.Sprintf(
				"A deadlock occurred involving %d sessions. "+
					"Session %d was chosen as the victim and its transaction was rolled back.",
				len(dl.Processes), dl.VictimSPID),
			WhyItMatters: "The victim session received an error (1205) and its work was lost. " +
				"The application must retry the transaction. " +
				"Frequent deadlocks indicate a design problem that wastes resources.",
			RootCause: "Deadlocks happen when two sessions hold locks the other needs, " +
				"in opposite order. Common patterns: " +
				"(1) Application accesses tables in different order, " +
				"(2) Missing indexes causing escalated lock scope, " +
				"(3) Long transactions holding locks while doing application work.",
			HowToFix: "1. Access tables in a consistent order across all transactions.\n" +
				"2. Keep transactions short — commit as soon as possible.\n" +
				"3. Add indexes to reduce lock scope.\n" +
				"4. Use READ_COMMITTED_SNAPSHOT to eliminate reader-writer deadlocks.\n" +
				"5. Review the deadlock XML graph (available in Extended Events system_health).",
			FixSQL: `-- Enable Read Committed Snapshot to eliminate reader-writer deadlocks
-- Test in non-production first!
ALTER DATABASE [YourDatabase] SET READ_COMMITTED_SNAPSHOT ON WITH ROLLBACK IMMEDIATE;

-- Retrieve deadlock graphs from system_health XE session
SELECT
    xdr.value('@timestamp','datetime2')     AS deadlock_time,
    CAST(xdr.query('.') AS NVARCHAR(MAX))   AS deadlock_graph
FROM (
    SELECT CAST(target_data AS XML) AS target_data
    FROM sys.dm_xe_session_targets  t
    JOIN sys.dm_xe_sessions          s ON t.event_session_address = s.address
    WHERE s.name = 'system_health'
      AND t.target_name = 'ring_buffer'
) AS data
CROSS APPLY target_data.nodes('//RingBufferTarget/event[@name="xml_deadlock_report"]') AS xdt(xdr)
ORDER BY deadlock_time DESC;`,
			SafeToAutoFix:   false,
			EstimatedImpact: "READ_COMMITTED_SNAPSHOT eliminates 80–90% of common deadlocks.",
			AffectedObject:  fmt.Sprintf("Session %d", dl.VictimSPID),
		})
	}

	// ── TEMPDB PRESSURE ───────────────────────────────────────────────────────
	if r.Transactions != nil && r.Transactions.TempDBUsedMB > 0 {
		freeMB := r.Transactions.TempDBFreeMB
		usedMB := r.Transactions.TempDBUsedMB
		if freeMB > 0 && usedMB/(usedMB+freeMB) > 0.85 {
			add(Advisory{
				ID:       "TDB-001",
				Severity: SeverityHigh,
				Category: CategoryTempDB,
				Title:    fmt.Sprintf("TempDB under pressure: %.0fMB used, %.0fMB free", usedMB, freeMB),
				WhatIsHappening: fmt.Sprintf(
					"TempDB is %.0f%% full. Used: %.0fMB, Free: %.0fMB. "+
						"Version store: %.0fMB.",
					usedMB/(usedMB+freeMB)*100, usedMB, freeMB,
					r.Transactions.TempDBVersionStoreMB),
				WhyItMatters: "If TempDB runs out of space, ALL user queries that need temporary " +
					"storage will fail with error 1105. This causes a complete outage for sort, " +
					"hash join, row versioning, and temp table operations.",
				RootCause: "Common causes: (1) Queries with large sorts or hash joins, " +
					"(2) Row versioning from READ_COMMITTED_SNAPSHOT or SNAPSHOT isolation, " +
					"(3) Long-running transactions preventing version store cleanup, " +
					"(4) Excessive temp table or table variable usage.",
				HowToFix: "1. Identify the largest TempDB consumers (script below).\n" +
					"2. Kill sessions with excessive version store usage if safe.\n" +
					"3. Pre-grow TempDB data files to avoid auto-growth pauses.\n" +
					"4. Add TempDB data files (one per CPU core, max 8).",
				FixSQL: `-- Find top TempDB consumers
SELECT TOP 10
    s.session_id,
    s.login_name,
    s.host_name,
    tsu.user_objects_alloc_page_count * 8 / 1024     AS user_objects_mb,
    tsu.internal_objects_alloc_page_count * 8 / 1024 AS internal_objects_mb,
    tsu.version_store_reserved_page_count * 8 / 1024 AS version_store_mb
FROM sys.dm_db_task_space_usage tsu
JOIN sys.dm_exec_sessions s ON tsu.session_id = s.session_id
WHERE s.is_user_process = 1
ORDER BY (tsu.user_objects_alloc_page_count + tsu.internal_objects_alloc_page_count) DESC;

-- Pre-grow TempDB to avoid auto-growth
-- ALTER DATABASE tempdb MODIFY FILE (NAME='tempdev', SIZE=4096MB, FILEGROWTH=512MB);`,
				SafeToAutoFix:   false,
				EstimatedImpact: "Pre-growing TempDB eliminates auto-growth pauses that can stall all queries.",
				AffectedObject:  "TempDB",
				MetricValue:     fmt.Sprintf("%.0fMB used of %.0fMB", usedMB, usedMB+freeMB),
			})
		}
	}

	// Sort: CRITICAL first, then HIGH, MEDIUM, LOW
	sortAdvisories(advisories)
	return advisories
}

// sortAdvisories orders advisories by severity (most critical first).
func sortAdvisories(a []Advisory) {
	rank := map[AdvisorySeverity]int{
		SeverityCritical: 0, SeverityHigh: 1, SeverityMedium: 2, SeverityLow: 3,
	}
	for i := 0; i < len(a); i++ {
		for j := i + 1; j < len(a); j++ {
			if rank[a[j].Severity] < rank[a[i].Severity] {
				a[i], a[j] = a[j], a[i]
			}
		}
	}
}

// findLongestBlocker finds the session causing the most blocking.
func findLongestBlocker(sessions []SessionInfo) int {
	counts := map[int]int{}
	for _, s := range sessions {
		if s.BlockingSession != 0 {
			counts[s.BlockingSession]++
		}
	}
	maxSid, maxCount := 0, 0
	for sid, count := range counts {
		if count > maxCount {
			maxSid, maxCount = sid, count
		}
	}
	return maxSid
}

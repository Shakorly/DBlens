package main

import "time"

// ─── Connections ─────────────────────────────────────────────────────────────

// ConnectionMetrics summarises session and blocking state for one server.
type ConnectionMetrics struct {
	ServerName      string
	TotalSessions   int
	ActiveRequests  int
	BlockedSessions int
	Sessions        []SessionInfo
}

// SessionInfo represents a single user session.
type SessionInfo struct {
	SessionID       int
	LoginName       string
	HostName        string
	Database        string
	Status          string
	CPUTime         int64
	MemoryUsageMB   float64
	ElapsedMS       int64
	WaitType        string
	BlockingSession int
	QueryText       string
}

// ─── Queries ─────────────────────────────────────────────────────────────────

// QueryMetrics holds both real-time long-running queries and plan-cache slow queries.
type QueryMetrics struct {
	ServerName        string
	ActiveLongRunning []ActiveQuery
	SlowQueries       []SlowQuery
}

// ActiveQuery is a currently executing request that exceeds the slow threshold.
type ActiveQuery struct {
	SessionID   int
	Status      string
	Command     string
	CPUTime     int64
	ElapsedMS   int64
	WaitType    string
	WaitTimeMS  int64
	Database    string
	LoginName   string
	HostName    string
	QueryText   string
	PlanAdvice  string   // populated by query plan analyser
	PlanWarnings []string
}

// SlowQuery comes from sys.dm_exec_query_stats (aggregated plan-cache history).
type SlowQuery struct {
	AvgElapsedMS    int64
	TotalElapsedMS  int64
	ExecutionCount  int64
	AvgLogicalReads int64
	AvgCPUMs        int64
	Database        string
	QueryText       string
}

// ─── Resources ───────────────────────────────────────────────────────────────

// ResourceMetrics holds CPU, memory, and disk I/O data for one server.
type ResourceMetrics struct {
	ServerName        string
	SQLCPUPercent     int
	SystemCPUPercent  int
	TotalMemoryMB     int64
	AvailableMemoryMB int64
	SQLMemoryMB       int64
	MemoryStateDesc   string
	DiskStats         []DiskStat
}

// DiskStat captures per-database-file I/O latency.
type DiskStat struct {
	Database     string
	PhysicalName string
	FileType     string
	AvgReadMS    float64
	AvgWriteMS   float64
	MBRead       int64
	MBWritten    int64
}

// ─── Replication ─────────────────────────────────────────────────────────────

// ReplicationMetrics holds Always On Availability Group health for one server.
type ReplicationMetrics struct {
	ServerName string
	AGGroups   []AGGroup
}

// AGGroup represents one Availability Group and all its replicas.
type AGGroup struct {
	AGName   string
	Replicas []ReplicaInfo
}

// ReplicaInfo holds health state for a single AG replica.
type ReplicaInfo struct {
	ReplicaServer       string
	Role                string
	OperationalState    string
	ConnectedState      string
	SyncHealth          string
	SecondaryLagSeconds int64
	LogSendQueueKB      int64
	RedoQueueKB         int64
}

// ─── Collection Result ────────────────────────────────────────────────────────

// CollectionResult bundles all metric categories gathered in one poll cycle.
type CollectionResult struct {
	ServerName   string
	Timestamp    time.Time
	Connections  *ConnectionMetrics
	Queries      *QueryMetrics
	Resources    *ResourceMetrics
	Replication  *ReplicationMetrics
	Backups      *BackupStatus
	Jobs         *JobMetrics
	Indexes      *IndexMetrics
	Waits        *WaitMetrics
	Sizes        *SizeMetrics
	Deadlocks    []DeadlockEvent
	Health       *HealthScore
	Inventory    *ServerInventory
	Transactions *TransactionMetrics
	Security     *SecurityAudit
	Network      *NetworkMetrics
	Capacity     *CapacityMetrics
	QueryStore   *QueryStoreMetrics
	Integrity    *IntegrityMetrics
	CustomMetrics []CustomMetricResult
	DiskSpace    *DiskSpaceMetrics
	Anomalies    []AnomalyAlert
	Advisories   []Advisory
	QueryPlans   map[int]*QueryPlanInfo
	Errors       []string
}

// ─── Deadlocks ────────────────────────────────────────────────────────────────

// DeadlockEvent holds a parsed deadlock from system_health extended events.
type DeadlockEvent struct {
	OccurredAt  time.Time
	VictimSPID  int
	Processes   []DeadlockProcess
	RawXML      string
}

// DeadlockProcess is one participant in a deadlock graph.
type DeadlockProcess struct {
	SPID        int
	LoginName   string
	Database    string
	WaitResource string
	QueryText   string
	IsVictim    bool
}

// ─── Backups ──────────────────────────────────────────────────────────────────

// BackupStatus holds the latest backup timestamps per database.
type BackupStatus struct {
	ServerName string
	Databases  []DBBackupInfo
}

// DBBackupInfo captures backup health for one database.
type DBBackupInfo struct {
	DatabaseName      string
	RecoveryModel     string
	LastFullBackup    *time.Time
	LastDiffBackup    *time.Time
	LastLogBackup     *time.Time
	HoursSinceFullBak float64
	HoursSinceLogBak  float64
	SizeMB            float64
	IsAlertFull       bool
	IsAlertLog        bool
}

// ─── SQL Agent Jobs ───────────────────────────────────────────────────────────

// JobMetrics holds SQL Agent job health for one server.
type JobMetrics struct {
	ServerName  string
	FailedJobs  []FailedJob
	LongRunJobs []LongRunningJob
}

// FailedJob is a SQL Agent job that failed in the last check window.
type FailedJob struct {
	JobName      string
	LastRunTime  time.Time
	RunDuration  string
	Message      string
	StepName     string
}

// LongRunningJob is currently executing and exceeds the expected duration.
type LongRunningJob struct {
	JobName        string
	StartTime      time.Time
	ElapsedMinutes int
}

// ─── Index Health ─────────────────────────────────────────────────────────────

// IndexMetrics captures missing and fragmented index info.
type IndexMetrics struct {
	ServerName      string
	MissingIndexes  []MissingIndex
	FragmentedIndexes []FragmentedIndex
}

// MissingIndex comes from sys.dm_db_missing_index_details.
type MissingIndex struct {
	Database        string
	TableName       string
	EqualityColumns string
	InequalityColumns string
	IncludeColumns  string
	ImpactScore     float64
	UniqueCompiles  int64
	UserSeeks       int64
	UserScans       int64
}

// FragmentedIndex is an index whose fragmentation exceeds the alert threshold.
type FragmentedIndex struct {
	Database          string
	TableName         string
	IndexName         string
	FragmentationPct  float64
	PageCount         int64
	RecommendedAction string // "REBUILD" or "REORGANIZE"
}

// ─── Wait Statistics ──────────────────────────────────────────────────────────

// WaitMetrics holds server-wide wait category analysis.
type WaitMetrics struct {
	ServerName string
	TopWaits   []WaitStat
}

// WaitStat is one wait type from sys.dm_os_wait_stats.
type WaitStat struct {
	WaitType        string
	Category        string
	WaitTimeSec     float64
	WaitingTasks    int64
	AvgWaitMS       float64
	PctOfTotal      float64
}

// ─── Database Size ────────────────────────────────────────────────────────────

// SizeMetrics tracks data and log file sizes per database.
type SizeMetrics struct {
	ServerName string
	Databases  []DBSizeInfo
}

// DBSizeInfo holds file sizes for one database.
type DBSizeInfo struct {
	DatabaseName  string
	DataSizeMB    float64
	LogSizeMB     float64
	DataUsedMB    float64
	LogUsedMB     float64
	DataFreePct   float64
	LogFreePct    float64
}

// ─── Health Score ─────────────────────────────────────────────────────────────

// HealthScore is an A–F grade computed from all metrics for one server.
type HealthScore struct {
	ServerName  string
	Grade       string  // A B C D F
	Score       int     // 0-100
	Penalties   []string
	Timestamp   time.Time
}


// HealthSnapshot is a single data point stored in the history for trending.
// Also used by capacity planning for projections.
type HealthSnapshot struct {
	Timestamp time.Time `json:"timestamp"`
	Score     int       `json:"score"`
	Grade     string    `json:"grade"`
}

// ─── OS Disk Space (v5) ───────────────────────────────────────────────────────

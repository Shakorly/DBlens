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
	ServerName  string
	Timestamp   time.Time
	Connections *ConnectionMetrics
	Queries     *QueryMetrics
	Resources   *ResourceMetrics
	Replication *ReplicationMetrics
	Errors      []string
}

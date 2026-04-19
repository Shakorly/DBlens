package main

import "time"

// ── Tier classification ───────────────────────────────────────────────────────

// Tier controls when and under what conditions a collector runs.
type Tier int

const (
	// TierFast — every poll cycle, always. Lightweight DMV reads only.
	// Max query time budget: 2s. Must never cause blocking.
	TierFast Tier = iota

	// TierMedium — every poll cycle, but skipped when server is stressed.
	// Stress = SQLCPUPercent > skipThreshold OR BlockedSessions > 0.
	TierMedium

	// TierSlow — runs on a fixed interval (e.g. every 6h). Heavy DMVs.
	// Also skipped under stress. Never runs more than once per interval.
	TierSlow

	// TierOnce — runs once at startup only (inventory, version check).
	TierOnce
)

func (t Tier) String() string {
	return [...]string{"FAST", "MEDIUM", "SLOW", "ONCE"}[t]
}

// ── Task definition ───────────────────────────────────────────────────────────

// Task describes one unit of monitoring work — one collector for one server.
type Task struct {
	Name       string          // e.g. "connections", "indexes"
	Tier       Tier
	Interval   time.Duration   // only used for TierSlow
	QueryBudget time.Duration  // max allowed wall time for this task
	Fn         CollectorFn     // the actual work
}

// CollectorFn is the function signature all collectors implement.
// ctx is pre-cancelled if QueryBudget is exceeded.
type CollectorFn func(ctx *TaskContext) error

// TaskContext wraps everything a collector needs, without coupling to sql.DB directly.
type TaskContext struct {
	DB         interface{ QueryContext(...interface{}) (interface{}, error) } // replaced by concrete in runner
	ServerName string
	Cfg        *Config
	Result     *CollectionResult
	LoadSignal *LoadSignal // current server load snapshot
}

// ── Task registry — the single source of truth for all collectors ─────────────

// AllTasks returns the complete ordered list of monitoring tasks.
// Order within a tier affects scheduling priority (earlier = higher priority).
func AllTasks() []Task {
	return []Task{
		// ── FAST — always run, every cycle ────────────────────────────────────
		{Name: "connections",   Tier: TierFast,   QueryBudget: 3 * time.Second},
		{Name: "resources",     Tier: TierFast,   QueryBudget: 3 * time.Second},
		{Name: "transactions",  Tier: TierFast,   QueryBudget: 2 * time.Second},
		{Name: "replication",   Tier: TierFast,   QueryBudget: 3 * time.Second},
		{Name: "diskspace",     Tier: TierFast,   QueryBudget: 2 * time.Second},

		// ── MEDIUM — every cycle, skipped under stress ─────────────────────────
		{Name: "queries",       Tier: TierMedium, QueryBudget: 5 * time.Second},
		{Name: "waits",         Tier: TierMedium, QueryBudget: 3 * time.Second},
		{Name: "deadlocks",     Tier: TierMedium, QueryBudget: 4 * time.Second},
		{Name: "jobs",          Tier: TierMedium, QueryBudget: 4 * time.Second},
		{Name: "dbsize",        Tier: TierMedium, QueryBudget: 4 * time.Second},
		{Name: "network",       Tier: TierMedium, QueryBudget: 3 * time.Second},
		{Name: "querystore",    Tier: TierMedium, QueryBudget: 8 * time.Second},
		{Name: "custom",        Tier: TierMedium, QueryBudget: 5 * time.Second},

		// ── SLOW — long intervals, only when server is calm ───────────────────
		{Name: "backups",       Tier: TierSlow,   Interval: 30 * time.Minute, QueryBudget: 6 * time.Second},
		{Name: "indexes",       Tier: TierSlow,   Interval: 6 * time.Hour,    QueryBudget: 10 * time.Second},
		{Name: "integrity",     Tier: TierSlow,   Interval: 24 * time.Hour,   QueryBudget: 8 * time.Second},
		{Name: "security",      Tier: TierSlow,   Interval: 1 * time.Hour,    QueryBudget: 5 * time.Second},
		{Name: "capacity",      Tier: TierSlow,   Interval: 30 * time.Minute, QueryBudget: 6 * time.Second},

		// ── ONCE — startup only ───────────────────────────────────────────────
		{Name: "inventory",     Tier: TierOnce,   QueryBudget: 8 * time.Second},
	}
}

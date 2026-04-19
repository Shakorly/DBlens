package main

import (
	"context"
	"database/sql"
	"sync"
	"time"
)

// BuildDispatchers returns the complete map of task name → Dispatcher function.
// This is the single place that wires collector functions to the scheduler.
// Adding a new collector = add one entry here + implement the collector function.
func BuildDispatchers(
	deadlockTracker *DeadlockTracker,
	lastDeadlockAt  *time.Time,
	anomaly         *AnomalyDetector,
	historyStore    *HistoryStore,
) map[string]Dispatcher {

	return map[string]Dispatcher{

		// ── FAST tier ─────────────────────────────────────────────────────────

		"connections": func(ctx context.Context, db *sql.DB, srv string, cfg *Config, r *CollectionResult, mu *sync.Mutex) error {
			v, err := CollectConnections(ctx, db, srv)
			if err != nil { return err }
			mu.Lock(); r.Connections = v; mu.Unlock()
			return nil
		},

		"resources": func(ctx context.Context, db *sql.DB, srv string, cfg *Config, r *CollectionResult, mu *sync.Mutex) error {
			v, err := CollectResources(ctx, db, srv)
			if err != nil { return err }
			mu.Lock(); r.Resources = v; mu.Unlock()
			return nil
		},

		"transactions": func(ctx context.Context, db *sql.DB, srv string, cfg *Config, r *CollectionResult, mu *sync.Mutex) error {
			v, err := CollectTransactions(ctx, db, srv)
			if err != nil { return err }
			mu.Lock(); r.Transactions = v; mu.Unlock()
			return nil
		},

		"replication": func(ctx context.Context, db *sql.DB, srv string, cfg *Config, r *CollectionResult, mu *sync.Mutex) error {
			v, err := CollectReplication(ctx, db, srv)
			if err != nil { return err }
			mu.Lock(); r.Replication = v; mu.Unlock()
			return nil
		},

		"diskspace": func(ctx context.Context, db *sql.DB, srv string, cfg *Config, r *CollectionResult, mu *sync.Mutex) error {
			v, err := CollectDiskSpace(ctx, db, srv)
			if err != nil { return err }
			mu.Lock(); r.DiskSpace = v; mu.Unlock()
			return nil
		},

		// ── MEDIUM tier ───────────────────────────────────────────────────────

		"queries": func(ctx context.Context, db *sql.DB, srv string, cfg *Config, r *CollectionResult, mu *sync.Mutex) error {
			v, err := CollectQueries(ctx, db, srv, cfg.SlowQueryThresholdMS)
			if err != nil { return err }
			// Capture execution plans for long-running queries (top 3, best-effort)
			if v != nil && len(v.ActiveLongRunning) > 0 {
				plans := CollectActiveQueryPlans(ctx, db, v.ActiveLongRunning)
				for i, q := range v.ActiveLongRunning {
					if p, ok := plans[q.SessionID]; ok {
						v.ActiveLongRunning[i].PlanAdvice  = GeneratePlanAdvice(p)
						v.ActiveLongRunning[i].PlanWarnings = p.PlanWarnings
					}
				}
			}
			mu.Lock(); r.Queries = v; mu.Unlock()
			return nil
		},

		"waits": func(ctx context.Context, db *sql.DB, srv string, cfg *Config, r *CollectionResult, mu *sync.Mutex) error {
			v, err := CollectWaitStats(ctx, db, srv)
			if err != nil { return err }
			mu.Lock(); r.Waits = v; mu.Unlock()
			return nil
		},

		"deadlocks": func(ctx context.Context, db *sql.DB, srv string, cfg *Config, r *CollectionResult, mu *sync.Mutex) error {
			since := *lastDeadlockAt
			evts, err := CollectDeadlocks(ctx, db, srv, since)
			if err != nil { return err }
			var fresh []DeadlockEvent
			for _, e := range evts {
				if deadlockTracker.IsNew(e) { fresh = append(fresh, e) }
			}
			if len(fresh) > 0 { *lastDeadlockAt = time.Now() }
			mu.Lock(); r.Deadlocks = fresh; mu.Unlock()
			return nil
		},

		"jobs": func(ctx context.Context, db *sql.DB, srv string, cfg *Config, r *CollectionResult, mu *sync.Mutex) error {
			v, err := CollectJobs(ctx, db, srv, cfg.JobFailureLookbackHours)
			if err != nil { return err }
			mu.Lock(); r.Jobs = v; mu.Unlock()
			return nil
		},

		"dbsize": func(ctx context.Context, db *sql.DB, srv string, cfg *Config, r *CollectionResult, mu *sync.Mutex) error {
			v, err := CollectDatabaseSizes(ctx, db, srv)
			if err != nil { return err }
			mu.Lock(); r.Sizes = v; mu.Unlock()
			return nil
		},

		"network": func(ctx context.Context, db *sql.DB, srv string, cfg *Config, r *CollectionResult, mu *sync.Mutex) error {
			v, err := CollectNetwork(ctx, db, srv)
			if err != nil { return err }
			mu.Lock(); r.Network = v; mu.Unlock()
			return nil
		},

		"querystore": func(ctx context.Context, db *sql.DB, srv string, cfg *Config, r *CollectionResult, mu *sync.Mutex) error {
			v, err := CollectQueryStore(ctx, db, srv)
			if err != nil { return err }
			mu.Lock(); r.QueryStore = v; mu.Unlock()
			return nil
		},

		"custom": func(ctx context.Context, db *sql.DB, srv string, cfg *Config, r *CollectionResult, mu *sync.Mutex) error {
			if len(cfg.CustomMetrics) == 0 { return nil }
			v := CollectCustomMetrics(ctx, db, cfg.CustomMetrics)
			mu.Lock(); r.CustomMetrics = v; mu.Unlock()
			return nil
		},

		// ── SLOW tier ─────────────────────────────────────────────────────────

		"backups": func(ctx context.Context, db *sql.DB, srv string, cfg *Config, r *CollectionResult, mu *sync.Mutex) error {
			v, err := CollectBackups(ctx, db, srv, cfg.BackupFullAlertHours, cfg.BackupLogAlertHours)
			if err != nil { return err }
			mu.Lock(); r.Backups = v; mu.Unlock()
			return nil
		},

		"indexes": func(ctx context.Context, db *sql.DB, srv string, cfg *Config, r *CollectionResult, mu *sync.Mutex) error {
			v, err := CollectIndexHealth(ctx, db, srv)
			if err != nil { return err }
			mu.Lock(); r.Indexes = v; mu.Unlock()
			return nil
		},

		"integrity": func(ctx context.Context, db *sql.DB, srv string, cfg *Config, r *CollectionResult, mu *sync.Mutex) error {
			v, err := CollectIntegrity(ctx, db, srv)
			if err != nil { return err }
			mu.Lock(); r.Integrity = v; mu.Unlock()
			return nil
		},

		"security": func(ctx context.Context, db *sql.DB, srv string, cfg *Config, r *CollectionResult, mu *sync.Mutex) error {
			v, err := CollectSecurity(ctx, db, srv)
			if err != nil { return err }
			mu.Lock(); r.Security = v; mu.Unlock()
			return nil
		},

		"capacity": func(ctx context.Context, db *sql.DB, srv string, cfg *Config, r *CollectionResult, mu *sync.Mutex) error {
			hist := historyStore.Get(srv)
			v, err := CollectCapacity(ctx, db, srv, hist, cfg.CPUAlertThresholdPct, cfg.MemoryAlertThresholdPct, historyStore)
			if err != nil { return err }
			mu.Lock(); r.Capacity = v; mu.Unlock()
			return nil
		},

		// ── ONCE tier ─────────────────────────────────────────────────────────

		"inventory": func(ctx context.Context, db *sql.DB, srv string, cfg *Config, r *CollectionResult, mu *sync.Mutex) error {
			v, err := CollectInventory(ctx, db, srv)
			if err != nil { return err }
			mu.Lock(); r.Inventory = v; mu.Unlock()
			return nil
		},
	}
}

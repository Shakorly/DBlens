package main

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"sync"
	"time"

	_ "github.com/microsoft/go-mssqldb"
)

// SlowCycleState tracks the last time expensive collectors ran (v5).
type SlowCycleState struct {
	lastIndexCheck     time.Time
	lastIntegrityCheck time.Time
	lastCapacityCheck  time.Time
}

// Collector owns a persistent connection pool for one server (v5: no open/close per cycle).
type Collector struct {
	server          ServerConfig
	cfg             *Config
	logger          *Logger
	deadlockTracker *DeadlockTracker
	lastDeadlockAt  time.Time
	historyStore    *HistoryStore
	anomaly         *AnomalyDetector

	// v5: persistent pool
	mu   sync.Mutex
	db   *sql.DB

	// v5: circuit breaker
	consecutiveFails int
	backoffUntil     time.Time

	// v5: slow-cycle scheduling
	slowState SlowCycleState
}

func NewCollector(server ServerConfig, cfg *Config, logger *Logger, hs *HistoryStore, ad *AnomalyDetector) *Collector {
	c := &Collector{
		server:          server,
		cfg:             cfg,
		logger:          logger,
		deadlockTracker: NewDeadlockTracker(),
		lastDeadlockAt:  time.Now().Add(-2 * time.Hour),
		historyStore:    hs,
		anomaly:         ad,
	}
	// Open pool at startup; failure is non-fatal — Collect() will retry.
	db, err := c.openPool()
	if err != nil {
		logger.Warn(server.Name, "Initial connection failed — will retry on first poll: "+err.Error())
	}
	c.db = db
	return c
}

// openPool creates a persistent sql.DB connection pool (v5).
func (c *Collector) openPool() (*sql.DB, error) {
	dsn := fmt.Sprintf(
		"sqlserver://%s:%s@%s:%d?database=master&connection+timeout=30&dial+timeout=15",
		c.server.Username, c.server.Password, c.server.Host, c.server.Port,
	)
	db, err := sql.Open("sqlserver", dsn)
	if err != nil {
		return nil, fmt.Errorf("open failed: %w", err)
	}
	db.SetMaxOpenConns(5)
	db.SetMaxIdleConns(2)
	db.SetConnMaxLifetime(30 * time.Minute)
	db.SetConnMaxIdleTime(5 * time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping failed: %w", err)
	}
	return db, nil
}

// getDB returns the live pool, reconnecting if necessary (v5).
func (c *Collector) getDB() (*sql.DB, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.db != nil {
		// Quick liveness check without blocking
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := c.db.PingContext(ctx); err == nil {
			return c.db, nil
		}
		// Pool is broken — close and reconnect
		_ = c.db.Close()
		c.db = nil
	}
	db, err := c.openPool()
	if err != nil {
		return nil, err
	}
	c.db = db
	return db, nil
}

// Close shuts down the persistent pool (call on shutdown).
func (c *Collector) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.db != nil {
		_ = c.db.Close()
		c.db = nil
	}
}

func (c *Collector) Collect() *CollectionResult {
	result := &CollectionResult{ServerName: c.server.Name, Timestamp: time.Now()}

	// v5: circuit breaker — skip if in backoff window
	if time.Now().Before(c.backoffUntil) {
		result.Errors = append(result.Errors,
			fmt.Sprintf("skipped — circuit open until %s (%d consecutive fails)",
				c.backoffUntil.Format("15:04:05"), c.consecutiveFails))
		return result
	}

	db, err := c.getDB()
	if err != nil {
		c.consecutiveFails++
		// Exponential backoff: 1, 2, 4, 8, 15 min (cap at 15)
		backoffMin := math.Min(math.Pow(2, float64(c.consecutiveFails-1)), 15)
		c.backoffUntil = time.Now().Add(time.Duration(backoffMin) * time.Minute)
		result.Errors = append(result.Errors, fmt.Sprintf("connection failed (backoff %.0fm): %v", backoffMin, err))
		return result
	}
	// Successful connection — reset circuit breaker
	c.consecutiveFails = 0
	c.backoffUntil = time.Time{}

	ctx, cancel := context.WithTimeout(context.Background(), 55*time.Second)
	defer cancel()

	var mu sync.Mutex
	var wg sync.WaitGroup

	run := func(name string, fn func() error) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := fn(); err != nil {
				mu.Lock()
				result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", name, err))
				mu.Unlock()
			}
		}()
	}

	run("connections", func() error {
		v, err := CollectConnections(ctx, db, c.server.Name); if err != nil { return err }
		mu.Lock(); result.Connections = v; mu.Unlock(); return nil
	})
	run("queries", func() error {
		v, err := CollectQueries(ctx, db, c.server.Name, c.cfg.SlowQueryThresholdMS); if err != nil { return err }
		mu.Lock(); result.Queries = v; mu.Unlock(); return nil
	})
	run("resources", func() error {
		v, err := CollectResources(ctx, db, c.server.Name); if err != nil { return err }
		mu.Lock(); result.Resources = v; mu.Unlock(); return nil
	})
	run("replication", func() error {
		v, err := CollectReplication(ctx, db, c.server.Name); if err != nil { return err }
		mu.Lock(); result.Replication = v; mu.Unlock(); return nil
	})
	run("backups", func() error {
		v, err := CollectBackups(ctx, db, c.server.Name, c.cfg.BackupFullAlertHours, c.cfg.BackupLogAlertHours)
		if err != nil { return err }
		mu.Lock(); result.Backups = v; mu.Unlock(); return nil
	})
	run("jobs", func() error {
		v, err := CollectJobs(ctx, db, c.server.Name, c.cfg.JobFailureLookbackHours); if err != nil { return err }
		mu.Lock(); result.Jobs = v; mu.Unlock(); return nil
	})
	run("waits", func() error {
		v, err := CollectWaitStats(ctx, db, c.server.Name); if err != nil { return err }
		mu.Lock(); result.Waits = v; mu.Unlock(); return nil
	})
	run("dbsize", func() error {
		v, err := CollectDatabaseSizes(ctx, db, c.server.Name); if err != nil { return err }
		mu.Lock(); result.Sizes = v; mu.Unlock(); return nil
	})
	run("inventory", func() error {
		v, err := CollectInventory(ctx, db, c.server.Name); if err != nil { return err }
		mu.Lock(); result.Inventory = v; mu.Unlock(); return nil
	})
	run("transactions", func() error {
		v, err := CollectTransactions(ctx, db, c.server.Name); if err != nil { return err }
		mu.Lock(); result.Transactions = v; mu.Unlock(); return nil
	})
	run("security", func() error {
		v, err := CollectSecurity(ctx, db, c.server.Name); if err != nil { return err }
		mu.Lock(); result.Security = v; mu.Unlock(); return nil
	})
	run("network", func() error {
		v, err := CollectNetwork(ctx, db, c.server.Name); if err != nil { return err }
		mu.Lock(); result.Network = v; mu.Unlock(); return nil
	})
	run("querystore", func() error {
		v, err := CollectQueryStore(ctx, db, c.server.Name); if err != nil { return err }
		mu.Lock(); result.QueryStore = v; mu.Unlock(); return nil
	})
	run("deadlocks", func() error {
		evts, err := CollectDeadlocks(ctx, db, c.server.Name, c.lastDeadlockAt); if err != nil { return err }
		var fresh []DeadlockEvent
		for _, e := range evts { if c.deadlockTracker.IsNew(e) { fresh = append(fresh, e) } }
		if len(fresh) > 0 { c.lastDeadlockAt = time.Now() }
		mu.Lock(); result.Deadlocks = fresh; mu.Unlock(); return nil
	})

	// v5: OS disk volume monitoring — every cycle (fast query)
	run("diskspace", func() error {
		v, err := CollectDiskSpace(ctx, db, c.server.Name); if err != nil { return err }
		mu.Lock(); result.DiskSpace = v; mu.Unlock(); return nil
	})

	// v5: slow-cycle collectors — index check every IndexCheckHours
	indexInterval := time.Duration(c.cfg.IndexCheckHours) * time.Hour
	if time.Since(c.slowState.lastIndexCheck) >= indexInterval {
		run("indexes", func() error {
			v, err := CollectIndexHealth(ctx, db, c.server.Name); if err != nil { return err }
			mu.Lock(); result.Indexes = v; mu.Unlock()
			c.slowState.lastIndexCheck = time.Now()
			return nil
		})
	}

	// v5: integrity check every IntegrityCheckHours
	integrityInterval := time.Duration(c.cfg.IntegrityCheckHours) * time.Hour
	if time.Since(c.slowState.lastIntegrityCheck) >= integrityInterval {
		run("integrity", func() error {
			v, err := CollectIntegrity(ctx, db, c.server.Name); if err != nil { return err }
			mu.Lock(); result.Integrity = v; mu.Unlock()
			c.slowState.lastIntegrityCheck = time.Now()
			return nil
		})
	}

	if len(c.cfg.CustomMetrics) > 0 {
		run("custom", func() error {
			v := CollectCustomMetrics(ctx, db, c.cfg.CustomMetrics)
			mu.Lock(); result.CustomMetrics = v; mu.Unlock(); return nil
		})
	}

	wg.Wait()

	// Sequential post-processing
	hist := c.historyStore.Get(c.server.Name)
	if cap, err := CollectCapacity(ctx, db, c.server.Name, hist,
		c.cfg.CPUAlertThresholdPct, c.cfg.MemoryAlertThresholdPct, c.historyStore); err == nil {
		result.Capacity = cap
	}

	health := ComputeHealthScore(result, c.cfg)
	result.Health = &health

	// Anomaly detection
	result.Anomalies = c.anomaly.FeedResult(result)

	c.historyStore.Record(result)
	return result
}

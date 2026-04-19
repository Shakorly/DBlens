package main

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/microsoft/go-mssqldb"
)

// Collector owns the persistent state for one SQL Server instance across cycles.
type Collector struct {
	server          ServerConfig
	cfg             *Config
	logger          *Logger
	db              *sql.DB
	scheduler       *Scheduler
	dispatchers     map[string]Dispatcher
	deadlockTracker *DeadlockTracker
	lastDeadlockAt  time.Time
	anomaly         *AnomalyDetector
	historyStore    *HistoryStore
	cache           *ResultCache // ← preserves ONCE/SLOW data across cycles
}

// NewCollector builds a Collector and attempts an initial connection.
func NewCollector(
	server ServerConfig,
	cfg *Config,
	logger *Logger,
	hs *HistoryStore,
	ad *AnomalyDetector,
) *Collector {
	lastDL := time.Now().Add(-2 * time.Hour)

	c := &Collector{
		server:          server,
		cfg:             cfg,
		logger:          logger,
		scheduler:       NewScheduler(server, cfg, logger),
		deadlockTracker: NewDeadlockTracker(),
		lastDeadlockAt:  lastDL,
		anomaly:         ad,
		historyStore:    hs,
		cache:           &ResultCache{},
	}

	c.dispatchers = BuildDispatchers(
		c.deadlockTracker,
		&c.lastDeadlockAt,
		c.anomaly,
		c.historyStore,
	)

	// Attempt initial connection (non-fatal if unreachable at startup)
	if db, err := c.openDB(); err != nil {
		logger.Warn(server.Name, "Initial connection failed — will retry on first poll: "+err.Error())
	} else {
		c.db = db
	}

	return c
}

func (c *Collector) openDB() (*sql.DB, error) {
	dsn := fmt.Sprintf(
		"sqlserver://%s:%s@%s:%d?database=master&connection+timeout=15&dial+timeout=15&encrypt=disable",
		c.server.Username, c.server.Password, c.server.Host, c.server.Port,
	)
	db, err := sql.Open("sqlserver", dsn)
	if err != nil {
		return nil, fmt.Errorf("open failed: %w", err)
	}
	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(10 * time.Minute)
	db.SetConnMaxIdleTime(3 * time.Minute)
	return db, nil
}

func (c *Collector) ensureConnected() (*sql.DB, error) {
	if c.db != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := c.db.PingContext(ctx); err == nil {
			return c.db, nil
		}
		_ = c.db.Close()
		c.db = nil
	}
	db, err := c.openDB()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping failed: %w", err)
	}
	c.db = db
	return db, nil
}

// Collect runs one monitoring cycle and returns a fully-populated result.
// Fields not collected this cycle are filled from the result cache so the
// dashboard always shows the most recent known data for every metric.
func (c *Collector) Collect() *CollectionResult {
	result := &CollectionResult{
		ServerName: c.server.Name,
		Timestamp:  time.Now(),
	}

	// ── Circuit breaker ───────────────────────────────────────────────────────
	allowed, reason := c.scheduler.circuit.Allow()
	if !allowed {
		result.Errors = append(result.Errors, reason)
		// Still merge cached data so the dashboard keeps showing last known state
		c.cache.Merge(result)
		return result
	}

	// ── Ensure connected ──────────────────────────────────────────────────────
	db, err := c.ensureConnected()
	if err != nil {
		c.scheduler.circuit.RecordFailure()
		failures := c.scheduler.circuit.Failures()
		backoff := time.Minute * (1 << min(failures-1, 5))
		result.Errors = append(result.Errors,
			fmt.Sprintf("connection failed (backoff %v): %v", backoff, err))
		// Merge cache — dashboard still shows old data rather than going blank
		c.cache.Merge(result)
		return result
	}

	// ── Run scheduled tasks ───────────────────────────────────────────────────
	result = c.scheduler.RunCycle(db, c.dispatchers)
	result.ServerName = c.server.Name

	if len(result.Errors) == 0 || result.Connections != nil {
		c.scheduler.circuit.RecordSuccess()
	} else {
		c.scheduler.circuit.RecordFailure()
	}

	// ── Step A: Save freshly collected fields to cache ────────────────────────
	c.cache.Absorb(result)

	// ── Step B: Fill any nil fields from cache ────────────────────────────────
	// This is the core fix: cycles that skipped TierOnce/TierSlow collectors
	// still expose the last known data instead of returning nil.
	c.cache.Merge(result)

	// ── Post-cycle: health score + anomaly detection ──────────────────────────
	health := ComputeHealthScore(result, c.cfg)
	result.Health = &health
	result.Anomalies = c.anomaly.FeedResult(result)
	// Generate intelligent advisories
	result.Advisories = GenerateAdvisories(result, c.cfg)

	// ── Persist to history ────────────────────────────────────────────────────
	c.historyStore.Record(result)

	return result
}

// Close releases the underlying DB connection pool.
func (c *Collector) Close() {
	if c.db != nil {
		_ = c.db.Close()
		c.db = nil
	}
}


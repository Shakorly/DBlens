package main

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/microsoft/go-mssqldb"
)

// Collector owns the persistent state for one SQL Server instance across cycles:
// the DB connection pool, circuit breaker, scheduler, and stateful sub-systems
// (deadlock tracker, anomaly detector, history store).
type Collector struct {
	server          ServerConfig
	cfg             *Config
	logger          *Logger
	db              *sql.DB           // persistent connection pool (reused across cycles)
	scheduler       *Scheduler
	dispatchers     map[string]Dispatcher
	deadlockTracker *DeadlockTracker
	lastDeadlockAt  time.Time
	anomaly         *AnomalyDetector
	historyStore    *HistoryStore
}

// NewCollector builds a Collector. It attempts an initial connection so startup
// logs show which servers are reachable before the first poll cycle.
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
	}

	// Build dispatcher table (stateful dispatchers capture pointers into Collector)
	c.dispatchers = BuildDispatchers(
		c.deadlockTracker,
		&c.lastDeadlockAt,
		c.anomaly,
		c.historyStore,
	)

	// Attempt initial connection (non-fatal if unreachable)
	if db, err := c.openDB(); err != nil {
		logger.Warn(server.Name, "Initial connection failed — will retry on first poll: "+err.Error())
	} else {
		c.db = db
	}

	return c
}

// openDB opens a new *sql.DB with appropriate pool settings.
// It does NOT ping — just configures the pool.
func (c *Collector) openDB() (*sql.DB, error) {
	dsn := fmt.Sprintf(
		"sqlserver://%s:%s@%s:%d?database=master&connection+timeout=15&dial+timeout=15&encrypt=disable",
		c.server.Username, c.server.Password, c.server.Host, c.server.Port,
	)
	db, err := sql.Open("sqlserver", dsn)
	if err != nil {
		return nil, fmt.Errorf("open failed: %w", err)
	}
	// Conservative pool — monitoring must not consume significant connection slots
	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(10 * time.Minute)
	db.SetConnMaxIdleTime(3 * time.Minute)
	return db, nil
}

// ensureConnected returns the live DB, reconnecting if necessary.
func (c *Collector) ensureConnected() (*sql.DB, error) {
	if c.db != nil {
		// Cheap liveness check
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := c.db.PingContext(ctx); err == nil {
			return c.db, nil
		}
		// Ping failed — close stale pool and reconnect
		_ = c.db.Close()
		c.db = nil
	}

	db, err := c.openDB()
	if err != nil {
		return nil, err
	}
	// Ping new connection
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping failed: %w", err)
	}
	c.db = db
	return db, nil
}

// Collect runs one monitoring cycle via the Scheduler.
// The Scheduler decides what runs based on tier, load, and timing.
func (c *Collector) Collect() *CollectionResult {
	result := &CollectionResult{
		ServerName: c.server.Name,
		Timestamp:  time.Now(),
	}

	// ── Circuit breaker ───────────────────────────────────────────────────────
	allowed, reason := c.scheduler.circuit.Allow()
	if !allowed {
		result.Errors = append(result.Errors, reason)
		return result
	}

	// ── Ensure DB connection ──────────────────────────────────────────────────
	db, err := c.ensureConnected()
	if err != nil {
		c.scheduler.circuit.RecordFailure()
		failures := c.scheduler.circuit.Failures()
		// Compute next backoff for the log message
		backoff := time.Minute * (1 << min(failures-1, 5))
		result.Errors = append(result.Errors, fmt.Sprintf(
			"connection failed (backoff %v): %v", backoff, err))
		return result
	}

	// ── Run the scheduled tasks ───────────────────────────────────────────────
	result = c.scheduler.RunCycle(db, c.dispatchers)
	result.ServerName = c.server.Name // ensure set even if scheduler forgets

	if len(result.Errors) == 0 || result.Connections != nil {
		c.scheduler.circuit.RecordSuccess()
	} else {
		c.scheduler.circuit.RecordFailure()
	}

	// ── Post-cycle: health score + anomaly detection ──────────────────────────
	health := ComputeHealthScore(result, c.cfg)
	result.Health = &health
	result.Anomalies = c.anomaly.FeedResult(result)

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

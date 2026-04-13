package main

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/microsoft/go-mssqldb"
)

// Collector manages the connection lifecycle and orchestrates metric collection
// for a single SQL Server instance.
type Collector struct {
	server ServerConfig
	cfg    *Config
	logger *Logger
}

// NewCollector constructs a Collector for the given server.
func NewCollector(server ServerConfig, cfg *Config, logger *Logger) *Collector {
	return &Collector{server: server, cfg: cfg, logger: logger}
}

// connect opens and validates a connection to the SQL Server instance.
func (c *Collector) connect() (*sql.DB, error) {
	dsn := fmt.Sprintf(
		"sqlserver://%s:%s@%s:%d?database=master&connection+timeout=30&dial+timeout=15",
		c.server.Username, c.server.Password, c.server.Host, c.server.Port,
	)

	db, err := sql.Open("sqlserver", dsn)
	if err != nil {
		return nil, fmt.Errorf("open failed: %w", err)
	}

	// Keep the monitoring footprint small — we only need a handful of connections.
	db.SetMaxOpenConns(3)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(5 * time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping failed: %w", err)
	}
	return db, nil
}

// Collect runs all four metric collectors and returns an aggregated result.
// Each collector is fault-isolated: failure in one does not block the others.
func (c *Collector) Collect() *CollectionResult {
	result := &CollectionResult{
		ServerName: c.server.Name,
		Timestamp:  time.Now(),
	}

	db, err := c.connect()
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("connection failed: %v", err))
		return result
	}
	defer db.Close()

	// Use a parent context with a generous timeout for the full collection round.
	ctx, cancel := context.WithTimeout(context.Background(), 55*time.Second)
	defer cancel()

	// ── Active Connections ───────────────────────────────────────────────────
	conns, err := CollectConnections(ctx, db, c.server.Name)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("connections: %v", err))
	} else {
		result.Connections = conns
	}

	// ── Query Performance ────────────────────────────────────────────────────
	queries, err := CollectQueries(ctx, db, c.server.Name, c.cfg.SlowQueryThresholdMS)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("queries: %v", err))
	} else {
		result.Queries = queries
	}

	// ── CPU / Memory / Disk ──────────────────────────────────────────────────
	resources, err := CollectResources(ctx, db, c.server.Name)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("resources: %v", err))
	} else {
		result.Resources = resources
	}

	// ── Always On Replication ────────────────────────────────────────────────
	replication, err := CollectReplication(ctx, db, c.server.Name)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("replication: %v", err))
	} else {
		result.Replication = replication
	}

	return result
}

package main

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// CustomMetric is a user-defined SQL query that returns a single numeric value.
type CustomMetric struct {
	Name        string  `json:"name"`
	Description string  `json:"description"`
	SQL         string  `json:"sql"`
	Database    string  `json:"database"`    // empty = master
	AlertAbove  float64 `json:"alert_above"` // 0 = no alert
	AlertBelow  float64 `json:"alert_below"` // 0 = no alert
	Unit        string  `json:"unit"`
}

// CustomMetricResult is one sampled custom metric value.
type CustomMetricResult struct {
	Name      string
	Value     float64
	Unit      string
	AlertHigh bool
	AlertLow  bool
	Error     string
	Timestamp time.Time
}

// CollectCustomMetrics executes all user-defined metric queries.
// Each metric is fault-isolated: failure in one does not affect others.
func CollectCustomMetrics(ctx context.Context, db *sql.DB, metrics []CustomMetric) []CustomMetricResult {
	if len(metrics) == 0 {
		return nil
	}

	results := make([]CustomMetricResult, 0, len(metrics))
	for _, m := range metrics {
		r := CustomMetricResult{Name: m.Name, Unit: m.Unit, Timestamp: time.Now()}

		// Validate database exists before running query
		if m.Database != "" {
			var exists int
			checkCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
			err := db.QueryRowContext(checkCtx,
				"SELECT COUNT(*) FROM sys.databases WHERE name = @name AND state_desc = 'ONLINE'",
				sql.Named("name", m.Database)).Scan(&exists)
			cancel()
			if err != nil || exists == 0 {
				r.Error = fmt.Sprintf("database %q does not exist or is not ONLINE on this server", m.Database)
				results = append(results, r)
				continue
			}
		}

		// Build safe query — use current DB or switch context
		query := m.SQL
		if m.Database != "" {
			// Sanitize database name (allow only alphanumeric, underscore, dash)
			safe := sanitizeIdentifier(m.Database)
			if safe != m.Database {
				r.Error = fmt.Sprintf("database name %q contains unsafe characters", m.Database)
				results = append(results, r)
				continue
			}
			query = fmt.Sprintf("USE [%s]; %s", safe, m.SQL)
		}

		metricCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		var val float64
		if err := db.QueryRowContext(metricCtx, query).Scan(&val); err != nil {
			r.Error = err.Error()
		} else {
			r.Value = val
			r.AlertHigh = m.AlertAbove > 0 && val > m.AlertAbove
			r.AlertLow  = m.AlertBelow > 0 && val < m.AlertBelow
		}
		cancel()
		results = append(results, r)
	}
	return results
}

// sanitizeIdentifier ensures a SQL identifier contains only safe characters.
func sanitizeIdentifier(s string) string {
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' || r == ' ' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

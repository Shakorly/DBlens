package main

import (
	"context"
	"database/sql"
	"fmt"
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
	Unit        string  `json:"unit"`        // e.g. "MB", "%", "rows"
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
func CollectCustomMetrics(ctx context.Context, db *sql.DB, metrics []CustomMetric) []CustomMetricResult {
	results := make([]CustomMetricResult, 0, len(metrics))
	for _, m := range metrics {
		r := CustomMetricResult{Name: m.Name, Unit: m.Unit, Timestamp: time.Now()}

		query := m.SQL
		if m.Database != "" {
			query = fmt.Sprintf("USE [%s]; %s", m.Database, m.SQL)
		}

		var val float64
		if err := db.QueryRowContext(ctx, query).Scan(&val); err != nil {
			r.Error = err.Error()
		} else {
			r.Value = val
			r.AlertHigh = m.AlertAbove > 0 && val > m.AlertAbove
			r.AlertLow  = m.AlertBelow > 0 && val < m.AlertBelow
		}
		results = append(results, r)
	}
	return results
}

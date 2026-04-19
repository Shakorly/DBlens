package main

import (
	"fmt"
	"math"
	"sync"
	"time"
)

// AnomalyDetector maintains rolling baselines per server/metric and fires
// alerts when the current value deviates beyond a configurable threshold.
type AnomalyDetector struct {
	mu        sync.Mutex
	baselines map[string]*baseline
	threshold float64 // e.g. 0.30 = 30% deviation triggers alert
	minSamples int    // minimum samples before anomaly detection kicks in
}

type baseline struct {
	samples []float64
	sum     float64
	sumSq   float64
}

// NewAnomalyDetector creates a detector with the given deviation threshold.
func NewAnomalyDetector(thresholdPct float64, minSamples int) *AnomalyDetector {
	if minSamples <= 0 {
		minSamples = 10
	}
	return &AnomalyDetector{
		baselines:  make(map[string]*baseline),
		threshold:  thresholdPct / 100.0,
		minSamples: minSamples,
	}
}

// Record adds a new data point to the rolling window (last 100 samples ≈ ~1.7h).
func (a *AnomalyDetector) Record(server, metric string, value float64) {
	a.mu.Lock()
	defer a.mu.Unlock()
	key := server + ":" + metric
	b := a.baselines[key]
	if b == nil {
		b = &baseline{}
		a.baselines[key] = b
	}
	b.samples = append(b.samples, value)
	b.sum += value
	b.sumSq += value * value
	// Keep last 100 samples
	if len(b.samples) > 100 {
		old := b.samples[0]
		b.samples = b.samples[1:]
		b.sum -= old
		b.sumSq -= old * old
	}
}

// CheckAnomaly returns an alert string if the value is anomalous, or "".
// Uses z-score: alert if |z| > 2 AND deviation > threshold.
func (a *AnomalyDetector) CheckAnomaly(server, metric string, current float64) string {
	a.mu.Lock()
	defer a.mu.Unlock()
	key := server + ":" + metric
	b := a.baselines[key]
	if b == nil || len(b.samples) < a.minSamples {
		return ""
	}
	n := float64(len(b.samples))
	mean := b.sum / n
	variance := b.sumSq/n - mean*mean
	if variance <= 0 {
		return ""
	}
	stddev := math.Sqrt(variance)
	zScore := math.Abs(current-mean) / stddev

	if mean == 0 {
		return ""
	}
	deviationPct := math.Abs(current-mean) / mean

	if zScore >= 2.0 && deviationPct >= a.threshold {
		direction := "above"
		if current < mean {
			direction = "below"
		}
		return fmt.Sprintf(
			"ANOMALY: %s — current=%.1f baseline=%.1f (%.0f%% %s normal, z=%.1f)",
			metric, current, mean, deviationPct*100, direction, zScore,
		)
	}
	return ""
}

// FeedResult extracts key metrics from a CollectionResult and feeds them
// into the detector, then returns any anomaly alerts found.
func (a *AnomalyDetector) FeedResult(r *CollectionResult) []AnomalyAlert {
	var alerts []AnomalyAlert
	srv := r.ServerName

	check := func(metric string, value float64) {
		a.Record(srv, metric, value)
		if msg := a.CheckAnomaly(srv, metric, value); msg != "" {
			alerts = append(alerts, AnomalyAlert{
				ServerName: srv,
				Metric:     metric,
				Message:    msg,
				Timestamp:  time.Now(),
				Current:    value,
			})
		}
	}

	if r.Resources != nil && r.Resources.SQLCPUPercent >= 0 {
		check("CPU%", float64(r.Resources.SQLCPUPercent))
		if r.Resources.TotalMemoryMB > 0 {
			memUsed := float64(r.Resources.TotalMemoryMB-r.Resources.AvailableMemoryMB) /
				float64(r.Resources.TotalMemoryMB) * 100
			check("MemUsed%", memUsed)
		}
	}
	if r.Connections != nil {
		check("Sessions", float64(r.Connections.TotalSessions))
		check("Blocked", float64(r.Connections.BlockedSessions))
	}
	if r.Queries != nil {
		check("SlowQueries", float64(len(r.Queries.ActiveLongRunning)))
	}
	if r.Transactions != nil {
		check("TPS", r.Transactions.TransactionsPerSec)
		check("BatchReq/s", r.Transactions.BatchRequestsPerSec)
	}
	return alerts
}

// AnomalyAlert is one detected statistical anomaly.
type AnomalyAlert struct {
	ServerName string
	Metric     string
	Message    string
	Timestamp  time.Time
	Current    float64
}

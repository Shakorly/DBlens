package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"
)

// MetricSample is one data point stored in the historical ring buffer.
type MetricSample struct {
	Timestamp   time.Time `json:"ts"`
	ServerName  string    `json:"server"`
	SQLCPUPct   int       `json:"cpu"`
	MemUsedPct  float64   `json:"mem"`
	Sessions    int       `json:"sess"`
	Blocked     int       `json:"blocked"`
	SlowQueries int       `json:"slow_q"`
	TPS         float64   `json:"tps"`
	BatchReqSec float64   `json:"batch_req"`
	HealthScore int       `json:"health"`
	HealthGrade string    `json:"grade"`
	ActiveTxns  int       `json:"txns"`
	DeadlockCnt int       `json:"deadlocks"`
}

// SizeSnapshot is a single database size measurement used for growth rate calculation (v5).
type SizeSnapshot struct {
	Timestamp time.Time `json:"ts"`
	SizeMB    float64   `json:"size_mb"`
}

// HistoryStore keeps an in-memory ring buffer and persists to disk.
// v5: also tracks per-database size snapshots for real growth rate calculation.
type HistoryStore struct {
	mu      sync.RWMutex
	samples map[string][]MetricSample  // keyed by serverName
	sizes   map[string][]SizeSnapshot  // keyed by "serverName:dbName"
	maxPts  int
	maxSize int // max size snapshots to keep per db
	dataDir string
}

// NewHistoryStore creates a new store that persists data to dataDir.
func NewHistoryStore(dataDir string, maxPts int) *HistoryStore {
	if maxPts <= 0 {
		maxPts = 1440
	}
	hs := &HistoryStore{
		samples: make(map[string][]MetricSample),
		sizes:   make(map[string][]SizeSnapshot),
		maxPts:  maxPts,
		maxSize: 288, // 7 days at 30-min intervals
		dataDir: dataDir,
	}
	hs.loadFromDisk()
	return hs
}

// Record appends a new data point for a server.
func (hs *HistoryStore) Record(r *CollectionResult) {
	sample := MetricSample{
		Timestamp:   r.Timestamp,
		ServerName:  r.ServerName,
		HealthScore: 0,
		HealthGrade: "?",
	}
	if r.Health != nil {
		sample.HealthScore = r.Health.Score
		sample.HealthGrade = r.Health.Grade
	}
	if r.Connections != nil {
		sample.Sessions = r.Connections.TotalSessions
		sample.Blocked = r.Connections.BlockedSessions
	}
	if r.Resources != nil {
		sample.SQLCPUPct = r.Resources.SQLCPUPercent
		if r.Resources.TotalMemoryMB > 0 {
			sample.MemUsedPct = float64(r.Resources.TotalMemoryMB-r.Resources.AvailableMemoryMB) /
				float64(r.Resources.TotalMemoryMB) * 100
		}
	}
	if r.Queries != nil {
		sample.SlowQueries = len(r.Queries.ActiveLongRunning)
	}
	if r.Transactions != nil {
		sample.TPS = r.Transactions.TransactionsPerSec
		sample.BatchReqSec = r.Transactions.BatchRequestsPerSec
		sample.ActiveTxns = r.Transactions.ActiveTransactions
	}
	sample.DeadlockCnt = len(r.Deadlocks)

	hs.mu.Lock()
	pts := hs.samples[r.ServerName]
	pts = append(pts, sample)
	if len(pts) > hs.maxPts {
		pts = pts[len(pts)-hs.maxPts:]
	}
	hs.samples[r.ServerName] = pts

	// v5: record per-database size snapshots for growth rate calculation
	if r.Sizes != nil {
		for _, db := range r.Sizes.Databases {
			key := r.ServerName + ":" + db.DatabaseName
			snap := SizeSnapshot{Timestamp: r.Timestamp, SizeMB: db.DataSizeMB}
			snaps := hs.sizes[key]
			snaps = append(snaps, snap)
			if len(snaps) > hs.maxSize {
				snaps = snaps[len(snaps)-hs.maxSize:]
			}
			hs.sizes[key] = snaps
		}
	}
	hs.mu.Unlock()

	hs.saveToDisk(r.ServerName)
}

// Get returns a copy of stored samples for a server.
func (hs *HistoryStore) Get(serverName string) []MetricSample {
	hs.mu.RLock()
	defer hs.mu.RUnlock()
	src := hs.samples[serverName]
	out := make([]MetricSample, len(src))
	copy(out, src)
	return out
}

// GetAll returns samples for all servers.
func (hs *HistoryStore) GetAll() map[string][]MetricSample {
	hs.mu.RLock()
	defer hs.mu.RUnlock()
	out := make(map[string][]MetricSample, len(hs.samples))
	for k, v := range hs.samples {
		cp := make([]MetricSample, len(v))
		copy(cp, v)
		out[k] = cp
	}
	return out
}

// GetSizeHistory returns size snapshots for a specific database over the last N days (v5).
func (hs *HistoryStore) GetSizeHistory(serverName, dbName string, days int) []SizeSnapshot {
	hs.mu.RLock()
	defer hs.mu.RUnlock()
	key := serverName + ":" + dbName
	src := hs.sizes[key]
	if len(src) == 0 {
		return nil
	}
	cutoff := time.Now().AddDate(0, 0, -days)
	var out []SizeSnapshot
	for _, s := range src {
		if s.Timestamp.After(cutoff) {
			out = append(out, s)
		}
	}
	return out
}

// ── Persistence ───────────────────────────────────────────────────────────────

func (hs *HistoryStore) filePath(server string) string {
	safe := ""
	for _, c := range server {
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' {
			safe += string(c)
		} else {
			safe += "_"
		}
	}
	return fmt.Sprintf("%s/history_%s.json", hs.dataDir, safe)
}

func (hs *HistoryStore) sizeFilePath(server string) string {
	safe := ""
	for _, c := range server {
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' {
			safe += string(c)
		} else {
			safe += "_"
		}
	}
	return fmt.Sprintf("%s/sizes_%s.json", hs.dataDir, safe)
}

func (hs *HistoryStore) saveToDisk(server string) {
	if hs.dataDir == "" {
		return
	}
	_ = os.MkdirAll(hs.dataDir, 0755)

	hs.mu.RLock()
	pts := hs.samples[server]
	// collect size snapshots for this server
	serverSizes := map[string][]SizeSnapshot{}
	for k, v := range hs.sizes {
		// key is "serverName:dbName" — only save this server's entries
		if len(k) > len(server)+1 && k[:len(server)+1] == server+":" {
			serverSizes[k] = v
		}
	}
	hs.mu.RUnlock()

	if data, err := json.Marshal(pts); err == nil {
		_ = os.WriteFile(hs.filePath(server), data, 0644)
	}
	if data, err := json.Marshal(serverSizes); err == nil {
		_ = os.WriteFile(hs.sizeFilePath(server), data, 0644)
	}
}

func (hs *HistoryStore) loadFromDisk() {
	if hs.dataDir == "" {
		return
	}
	entries, err := os.ReadDir(hs.dataDir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		fullPath := hs.dataDir + "/" + name

		if len(name) >= 8 && name[:8] == "history_" {
			data, err := os.ReadFile(fullPath)
			if err != nil {
				continue
			}
			var pts []MetricSample
			if err := json.Unmarshal(data, &pts); err != nil {
				continue
			}
			if len(pts) > 0 {
				hs.samples[pts[0].ServerName] = pts
			}
		}

		if len(name) >= 6 && name[:6] == "sizes_" {
			data, err := os.ReadFile(fullPath)
			if err != nil {
				continue
			}
			var serverSizes map[string][]SizeSnapshot
			if err := json.Unmarshal(data, &serverSizes); err != nil {
				continue
			}
			for k, v := range serverSizes {
				hs.sizes[k] = v
			}
		}
	}
}

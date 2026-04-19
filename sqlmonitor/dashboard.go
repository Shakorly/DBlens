package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
)

type DashboardStore struct {
	mu      sync.RWMutex
	results map[string]*CollectionResult
}

func NewDashboardStore() *DashboardStore {
	return &DashboardStore{results: make(map[string]*CollectionResult)}
}

func (ds *DashboardStore) Update(r *CollectionResult) {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	ds.results[r.ServerName] = r
}

func StartDashboard(port int, store *DashboardStore, hs *HistoryStore, logger *Logger) {
	mux := http.NewServeMux()

	// Main UI
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, dashHTML)
	})

	// Current results — safe JSON marshal
	mux.HandleFunc("/api/results", func(w http.ResponseWriter, r *http.Request) {
		store.mu.RLock()
		defer store.mu.RUnlock()
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		if err := json.NewEncoder(w).Encode(store.results); err != nil {
			http.Error(w, `{"error":"marshal failed"}`, 500)
		}
	})

	// History
	mux.HandleFunc("/api/history", func(w http.ResponseWriter, r *http.Request) {
		data := hs.GetAll()
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		if err := json.NewEncoder(w).Encode(data); err != nil {
			w.Write([]byte("{}"))
		}
	})

	// Advisories — separate from results to keep result payload small
	mux.HandleFunc("/api/advisories", func(w http.ResponseWriter, r *http.Request) {
		store.mu.RLock()
		defer store.mu.RUnlock()
		type advDTO struct {
			ID        string `json:"id"`
			Server    string `json:"server"`
			Database  string `json:"database"`
			Severity  string `json:"severity"`
			Category  string `json:"category"`
			Title     string `json:"title"`
			What      string `json:"what"`
			Why       string `json:"why"`
			Cause     string `json:"cause"`
			Fix       string `json:"fix"`
			SQL       string `json:"sql"`
			Impact    string `json:"impact"`
			Object    string `json:"object"`
			Metric    string `json:"metric"`
			Threshold string `json:"threshold"`
			Safe      bool   `json:"safe"`
		}
		var all []advDTO
		for _, result := range store.results {
			for _, adv := range result.Advisories {
				all = append(all, advDTO{
					ID: adv.ID, Server: adv.ServerName, Database: adv.Database,
					Severity: string(adv.Severity), Category: string(adv.Category),
					Title: adv.Title, What: adv.WhatIsHappening, Why: adv.WhyItMatters,
					Cause: adv.RootCause, Fix: adv.HowToFix, SQL: adv.FixSQL,
					Impact: adv.EstimatedImpact, Object: adv.AffectedObject,
					Metric: adv.MetricValue, Threshold: adv.MetricThreshold,
					Safe: adv.SafeToAutoFix,
				})
			}
		}
		if all == nil {
			all = []advDTO{}
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		json.NewEncoder(w).Encode(all)
	})


	// Apply Fix — only safe, read-only-equivalent operations allowed
	// Currently supports: REORGANIZE INDEX (online, never blocks)
	mux.HandleFunc("/api/apply-fix", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, `{"error":"POST only"}`, 405)
			return
		}
		var req struct {
			ServerName string `json:"server"`
			FixType    string `json:"fix_type"`
			Database   string `json:"database"`
			Object     string `json:"object"`    // e.g. "dbo.Orders"
			IndexName  string `json:"index_name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid JSON"}`, 400)
			return
		}
		// Only allow safe operations — never REBUILD (locks table), never KILL
		allowed := map[string]bool{
			"reorganize_index": true,
			"update_statistics": true,
		}
		if !allowed[req.FixType] {
			http.Error(w, `{"error":"operation not permitted via API"}`, 403)
			return
		}

		store.mu.RLock()
		result, ok := store.results[req.ServerName]
		store.mu.RUnlock()
		if !ok {
			http.Error(w, `{"error":"server not found"}`, 404)
			return
		}
		_ = result

		w.Header().Set("Content-Type", "application/json")
		// Return the script — actual execution requires DBA approval
		var script string
		switch req.FixType {
		case "reorganize_index":
			script = fmt.Sprintf("USE [%s];\nALTER INDEX [%s] ON %s REORGANIZE;\nUPDATE STATISTICS %s WITH ROWCOUNT, PAGECOUNT;",
				req.Database, req.IndexName, req.Object, req.Object)
		case "update_statistics":
			script = fmt.Sprintf("USE [%s];\nUPDATE STATISTICS %s WITH FULLSCAN;", req.Database, req.Object)
		}
		json.NewEncoder(w).Encode(map[string]string{
			"status": "script_ready",
			"script": script,
			"note":   "Review and run this script in SSMS. DBLens does not execute fix scripts automatically.",
		})
	})

	addr := fmt.Sprintf(":%d", port)
	logger.Info("", fmt.Sprintf("🌐 Dashboard → http://localhost%s", addr))
	if err := http.ListenAndServe(addr, mux); err != nil {
		logger.Error("", "Dashboard: "+err.Error())
	}
}


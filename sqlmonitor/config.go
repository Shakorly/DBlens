package main

import (
	"encoding/json"
	"fmt"
	"os"
)

// ServerConfig holds connection details for a single SQL Server instance.
type ServerConfig struct {
	Name      string   `json:"name"`
	Host      string   `json:"host"`
	Port      int      `json:"port"`
	Username  string   `json:"username"`
	Password  string   `json:"password"`
	Databases []string `json:"databases"`
}

// Config is the root configuration structure loaded from config.json.
type Config struct {
	PollIntervalSeconds        int            `json:"poll_interval_seconds"`
	SlowQueryThresholdMS       int64          `json:"slow_query_threshold_ms"`
	MaxConnectionsThreshold    int            `json:"max_connections_threshold"`
	CPUAlertThresholdPct       int            `json:"cpu_alert_threshold_pct"`
	MemoryAlertThresholdPct    int            `json:"memory_alert_threshold_pct"`
	DiskIOAlertMS              float64        `json:"disk_io_alert_ms"`
	ReplicationLagAlertSeconds int64          `json:"replication_lag_alert_seconds"`
	LogFile                    string         `json:"log_file"`
	LogLevel                   string         `json:"log_level"`
	Servers                    []ServerConfig `json:"servers"`
}

// LoadConfig reads and parses the JSON config file, applying safe defaults.
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("cannot read config file %q: %w", path, err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("cannot parse config file: %w", err)
	}

	if cfg.PollIntervalSeconds <= 0 {
		cfg.PollIntervalSeconds = 60
	}
	if cfg.SlowQueryThresholdMS <= 0 {
		cfg.SlowQueryThresholdMS = 5000
	}
	if cfg.MaxConnectionsThreshold <= 0 {
		cfg.MaxConnectionsThreshold = 200
	}
	if cfg.CPUAlertThresholdPct <= 0 {
		cfg.CPUAlertThresholdPct = 80
	}
	if cfg.MemoryAlertThresholdPct <= 0 {
		cfg.MemoryAlertThresholdPct = 10
	}
	if cfg.DiskIOAlertMS <= 0 {
		cfg.DiskIOAlertMS = 50
	}
	if cfg.ReplicationLagAlertSeconds <= 0 {
		cfg.ReplicationLagAlertSeconds = 30
	}
	if cfg.LogFile == "" {
		cfg.LogFile = "sql_monitor.log"
	}
	if cfg.LogLevel == "" {
		cfg.LogLevel = "INFO"
	}
	if len(cfg.Servers) == 0 {
		return nil, fmt.Errorf("no servers defined in config")
	}
	for i, s := range cfg.Servers {
		if s.Port == 0 {
			cfg.Servers[i].Port = 1433
		}
	}
	return &cfg, nil
}

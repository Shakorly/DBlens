package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

type ServerConfig struct {
	Name      string   `json:"name"`
	Host      string   `json:"host"`
	Port      int      `json:"port"`
	Username  string   `json:"username"`
	Password  string   `json:"password"`
	Databases []string `json:"databases"`
}

type Config struct {
	PollIntervalSeconds     int   `json:"poll_interval_seconds"`
	SlowQueryThresholdMS    int64 `json:"slow_query_threshold_ms"`
	MaxConnectionsThreshold int   `json:"max_connections_threshold"`

	CPUAlertThresholdPct       int     `json:"cpu_alert_threshold_pct"`
	MemoryAlertThresholdPct    int     `json:"memory_alert_threshold_pct"`
	DiskIOAlertMS              float64 `json:"disk_io_alert_ms"`
	ReplicationLagAlertSeconds int64   `json:"replication_lag_alert_seconds"`

	// v5: OS-level disk volume thresholds
	DiskWarnFreePct float64 `json:"disk_warn_free_pct"`
	DiskCritFreePct float64 `json:"disk_crit_free_pct"`

	BackupFullAlertHours float64 `json:"backup_full_alert_hours"`
	BackupLogAlertHours  float64 `json:"backup_log_alert_hours"`

	IndexFragAlertPct float64 `json:"index_frag_alert_pct"`
	// v5: slow-cycle intervals
	IndexCheckHours     int `json:"index_check_hours"`
	IntegrityCheckHours int `json:"integrity_check_hours"`

	JobFailureLookbackHours int `json:"job_failure_lookback_hours"`
	AlertCooldownMinutes    int `json:"alert_cooldown_minutes"`

	AnomalyThresholdPct float64 `json:"anomaly_threshold_pct"`
	AnomalyMinSamples   int     `json:"anomaly_min_samples"`

	LogMaxSizeMB   int  `json:"log_max_size_mb"`
	LogMaxFiles    int  `json:"log_max_files"`
	LogCompressOld bool `json:"log_compress_old"`

	DashboardEnabled bool `json:"dashboard_enabled"`
	DashboardPort    int  `json:"dashboard_port"`

	PrometheusEnabled bool `json:"prometheus_enabled"`
	PrometheusPort    int  `json:"prometheus_port"`

	AlertChannels []AlertChannel `json:"alert_channels"`
	CustomMetrics []CustomMetric `json:"custom_metrics"`

	LogFile  string `json:"log_file"`
	LogLevel string `json:"log_level"`

	Servers []ServerConfig `json:"servers"`
}

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
	if cfg.BackupFullAlertHours <= 0 {
		cfg.BackupFullAlertHours = 25
	}
	if cfg.BackupLogAlertHours <= 0 {
		cfg.BackupLogAlertHours = 4
	}
	if cfg.IndexFragAlertPct <= 0 {
		cfg.IndexFragAlertPct = 30
	}
	if cfg.IndexCheckHours <= 0 {
		cfg.IndexCheckHours = 6
	}
	if cfg.IntegrityCheckHours <= 0 {
		cfg.IntegrityCheckHours = 24
	}
	if cfg.JobFailureLookbackHours <= 0 {
		cfg.JobFailureLookbackHours = 2
	}
	if cfg.AlertCooldownMinutes <= 0 {
		cfg.AlertCooldownMinutes = 10
	}
	if cfg.AnomalyThresholdPct <= 0 {
		cfg.AnomalyThresholdPct = 30
	}
	if cfg.AnomalyMinSamples <= 0 {
		cfg.AnomalyMinSamples = 10
	}
	if cfg.LogMaxSizeMB <= 0 {
		cfg.LogMaxSizeMB = 50
	}
	if cfg.LogMaxFiles <= 0 {
		cfg.LogMaxFiles = 30
	}
	if cfg.DashboardPort <= 0 {
		cfg.DashboardPort = 8080
	}
	if cfg.PrometheusPort <= 0 {
		cfg.PrometheusPort = 9090
	}
	if cfg.LogFile == "" {
		cfg.LogFile = "sql_monitor.log"
	}
	if cfg.LogLevel == "" {
		cfg.LogLevel = "INFO"
	}
	if cfg.DiskWarnFreePct <= 0 {
		cfg.DiskWarnFreePct = 20
	}
	if cfg.DiskCritFreePct <= 0 {
		cfg.DiskCritFreePct = 10
	}

	if len(cfg.Servers) == 0 {
		return nil, fmt.Errorf("no servers defined in config")
	}

	for i, s := range cfg.Servers {
		if s.Port == 0 {
			cfg.Servers[i].Port = 1433
		}
		// v5: Inject password from environment variable.
		// DBLENS_PASS_<SERVER_NAME> (hyphens/spaces → underscores, uppercased)
		// e.g. server "PROD-SQL-01" → DBLENS_PASS_PROD_SQL_01
		envKey := "DBLENS_PASS_" + strings.ToUpper(
			strings.NewReplacer("-", "_", " ", "_", ".", "_").Replace(s.Name))
		if pw := os.Getenv(envKey); pw != "" {
			cfg.Servers[i].Password = pw
		}
		// Global fallback: DBLENS_PASS (all servers share one monitoring account)
		if cfg.Servers[i].Password == "" {
			if pw := os.Getenv("DBLENS_PASS"); pw != "" {
				cfg.Servers[i].Password = pw
			}
		}
	}

	return &cfg, nil
}

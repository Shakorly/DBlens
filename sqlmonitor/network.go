package main

import (
	"context"
	"database/sql"
	"time"
)

// NetworkMetrics captures SQL Server network and connection throughput.
type NetworkMetrics struct {
	ServerName        string
	CollectedAt       time.Time
	BytesSentPerSec   float64
	BytesRecvPerSec   float64
	PacketErrorsPerSec float64
	ActiveConnections int
	ConnectionsPerSec float64
	Endpoints         []EndpointInfo
}

// EndpointInfo describes a SQL Server network endpoint.
type EndpointInfo struct {
	Name          string
	Protocol      string
	Port          int
	State         string
	ActiveSessions int
}

// CollectNetwork gathers SQL Server network statistics.
func CollectNetwork(ctx context.Context, db *sql.DB, serverName string) (*NetworkMetrics, error) {
	m := &NetworkMetrics{ServerName: serverName, CollectedAt: time.Now()}

	// ── SQL Server network perf counters ─────────────────────────────────────
	counterSQL := `
		SELECT counter_name, cntr_value
		FROM sys.dm_os_performance_counters
		WHERE object_name LIKE '%Broker/DBM Transport%'
		   OR (object_name LIKE '%SQL Statistics%'
			   AND counter_name = 'SQL Re-Compilations/sec')
		   OR (object_name LIKE '%General Statistics%'
			   AND counter_name IN ('User Connections','Connection Resets/sec',
			                        'Logins/sec','Logouts/sec'))`

	rows, err := db.QueryContext(ctx, counterSQL)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var name string
			var val float64
			if err := rows.Scan(&name, &val); err != nil { continue }
			switch name {
			case "User Connections":
				m.ActiveConnections = int(val)
			case "Logins/sec":
				m.ConnectionsPerSec = val
			}
		}
		rows.Close()
	}

	// ── Connection throughput via dm_exec_connections ─────────────────────────
	networkSQL := `
		SELECT
			SUM(CAST(c.num_reads  AS BIGINT))    AS total_reads,
			SUM(CAST(c.num_writes AS BIGINT))    AS total_writes,
			COUNT(*)                             AS total_connections
		FROM sys.dm_exec_connections c`

	var reads, writes int64
	if err := db.QueryRowContext(ctx, networkSQL).Scan(&reads, &writes, &m.ActiveConnections); err == nil {
		m.BytesRecvPerSec = float64(reads)
		m.BytesSentPerSec = float64(writes)
	}

	// ── SQL Server endpoints ──────────────────────────────────────────────────
	endpointSQL := `
		SELECT
			e.name,
			e.protocol_desc,
			ISNULL(te.port, 0)                             AS port,
			e.state_desc,
			(SELECT COUNT(*) FROM sys.dm_exec_sessions s
			 WHERE s.endpoint_id = e.endpoint_id
			   AND s.is_user_process = 1)                  AS active_sessions
		FROM sys.endpoints            e
		LEFT JOIN sys.tcp_endpoints   te ON e.endpoint_id = te.endpoint_id
		WHERE e.type IN (2, 3, 4)      -- TSQL, SERVICE_BROKER, DATABASE_MIRRORING
		ORDER BY active_sessions DESC`

	rows2, err := db.QueryContext(ctx, endpointSQL)
	if err == nil {
		defer rows2.Close()
		for rows2.Next() {
			var ep EndpointInfo
			if err := rows2.Scan(&ep.Name, &ep.Protocol, &ep.Port, &ep.State, &ep.ActiveSessions); err != nil {
				continue
			}
			m.Endpoints = append(m.Endpoints, ep)
		}
	}
	return m, nil
}

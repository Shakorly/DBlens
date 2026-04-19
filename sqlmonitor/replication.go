package main

import (
	"context"
	"database/sql"
)

// CollectReplication probes Always On Availability Group health.
// If the server has no AG configured the function returns an empty but valid result.
func CollectReplication(ctx context.Context, db *sql.DB, serverName string) (*ReplicationMetrics, error) {
	metrics := &ReplicationMetrics{ServerName: serverName}

	// Quick check — if no AGs exist, skip the rest silently.
	var agCount int
	if err := db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM sys.availability_groups").Scan(&agCount); err != nil || agCount == 0 {
		return metrics, nil
	}

	// Aggregate per replica so that databases sharing a replica don't create
	// duplicate rows — we take the maximum lag and sum of queue sizes.
	agSQL := `
		SELECT
			ag.name                                              AS ag_name,
			ar.replica_server_name,
			ISNULL(ars.role_desc, 'UNKNOWN')                    AS role,
			ISNULL(ars.operational_state_desc, 'UNKNOWN')       AS operational_state,
			ISNULL(ars.connected_state_desc, 'UNKNOWN')         AS connected_state,
			ISNULL(ars.synchronization_health_desc, 'UNKNOWN')  AS sync_health,
			ISNULL(MAX(adbrs.secondary_lag_seconds), 0)         AS max_lag_seconds,
			ISNULL(SUM(adbrs.log_send_queue_size), 0)           AS total_log_send_kb,
			ISNULL(SUM(adbrs.redo_queue_size), 0)               AS total_redo_kb
		FROM sys.availability_groups ag
		JOIN sys.availability_replicas ar
			ON ag.group_id = ar.group_id
		LEFT JOIN sys.dm_hadr_availability_replica_states ars
			ON ar.replica_id = ars.replica_id
		LEFT JOIN sys.dm_hadr_database_replica_states adbrs
			ON ars.replica_id = adbrs.replica_id
		GROUP BY
			ag.name,
			ar.replica_server_name,
			ars.role_desc,
			ars.operational_state_desc,
			ars.connected_state_desc,
			ars.synchronization_health_desc
		ORDER BY ag.name, ar.replica_server_name`

	rows, err := db.QueryContext(ctx, agSQL)
	if err != nil {
		// AG DMVs might not be accessible on every edition — treat as non-fatal.
		return metrics, nil
	}
	defer rows.Close()

	// Accumulate replicas grouped by AG name.
	agMap := make(map[string]*AGGroup)

	for rows.Next() {
		var (
			agName, replicaServer                       string
			role, opState, connState, syncHealth        string
			lagSecs, logSendQueueKB, redoQueueKB        int64
		)
		if err := rows.Scan(
			&agName, &replicaServer,
			&role, &opState, &connState, &syncHealth,
			&lagSecs, &logSendQueueKB, &redoQueueKB,
		); err != nil {
			continue
		}

		if _, ok := agMap[agName]; !ok {
			agMap[agName] = &AGGroup{AGName: agName}
		}
		agMap[agName].Replicas = append(agMap[agName].Replicas, ReplicaInfo{
			ReplicaServer:       replicaServer,
			Role:                role,
			OperationalState:    opState,
			ConnectedState:      connState,
			SyncHealth:          syncHealth,
			SecondaryLagSeconds: lagSecs,
			LogSendQueueKB:      logSendQueueKB,
			RedoQueueKB:         redoQueueKB,
		})
	}

	for _, ag := range agMap {
		metrics.AGGroups = append(metrics.AGGroups, *ag)
	}

	return metrics, rows.Err()
}

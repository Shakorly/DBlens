package main

import (
	"context"
	"database/sql"
)

// CollectIndexHealth gathers missing index recommendations and fragmented indexes.
// v5: uses SAMPLED (not DETAILED) to avoid long-running I/O on large databases.
//     This is called on a slow schedule (IndexCheckHours) from collector.go.
func CollectIndexHealth(ctx context.Context, db *sql.DB, serverName string) (*IndexMetrics, error) {
	metrics := &IndexMetrics{ServerName: serverName}

	// ── Missing indexes (from plan cache analysis) ────────────────────────────
	missingSQL := `
		SELECT TOP 15
			DB_NAME(mid.database_id)                          AS database_name,
			OBJECT_NAME(mid.object_id, mid.database_id)      AS table_name,
			ISNULL(mid.equality_columns, '')                  AS equality_cols,
			ISNULL(mid.inequality_columns, '')                AS inequality_cols,
			ISNULL(mid.included_columns, '')                  AS include_cols,
			ROUND(migs.avg_total_user_cost
				* migs.avg_user_impact
				* (migs.user_seeks + migs.user_scans), 2)    AS impact_score,
			migs.unique_compiles,
			migs.user_seeks,
			migs.user_scans
		FROM sys.dm_db_missing_index_details    mid
		JOIN sys.dm_db_missing_index_groups      mig  ON mid.index_handle    = mig.index_handle
		JOIN sys.dm_db_missing_index_group_stats migs ON mig.index_group_handle = migs.group_handle
		WHERE migs.avg_total_user_cost * migs.avg_user_impact * (migs.user_seeks + migs.user_scans) > 1000
		ORDER BY impact_score DESC`

	rows, err := db.QueryContext(ctx, missingSQL)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var mi MissingIndex
			if err := rows.Scan(
				&mi.Database, &mi.TableName,
				&mi.EqualityColumns, &mi.InequalityColumns, &mi.IncludeColumns,
				&mi.ImpactScore, &mi.UniqueCompiles, &mi.UserSeeks, &mi.UserScans,
			); err != nil {
				continue
			}
			metrics.MissingIndexes = append(metrics.MissingIndexes, mi)
		}
		rows.Close()
	}

	// ── Fragmented indexes ────────────────────────────────────────────────────
	// v5: Changed DETAILED → SAMPLED. SAMPLED is ~100x faster on large databases
	// because it reads a statistical sample of pages rather than every page.
	// We also require page_count >= 1000 (not 500) to skip tiny indexes that
	// fragment quickly but have negligible real-world impact.
	fragSQL := `
		SELECT TOP 20
			DB_NAME()                                         AS database_name,
			OBJECT_NAME(ips.object_id)                        AS table_name,
			ISNULL(i.name, 'HEAP')                            AS index_name,
			ROUND(ips.avg_fragmentation_in_percent, 1)        AS frag_pct,
			ips.page_count,
			CASE
				WHEN ips.avg_fragmentation_in_percent >= 30 THEN 'REBUILD'
				ELSE 'REORGANIZE'
			END                                               AS recommended_action
		FROM sys.dm_db_index_physical_stats(DB_ID(), NULL, NULL, NULL, 'SAMPLED') ips
		JOIN sys.indexes i
			ON ips.object_id = i.object_id
		   AND ips.index_id  = i.index_id
		WHERE ips.avg_fragmentation_in_percent >= 10
		  AND ips.page_count >= 1000
		  AND ips.index_type_desc <> 'HEAP'
		ORDER BY ips.avg_fragmentation_in_percent DESC`

	rows2, err := db.QueryContext(ctx, fragSQL)
	if err == nil {
		defer rows2.Close()
		for rows2.Next() {
			var fi FragmentedIndex
			if err := rows2.Scan(
				&fi.Database, &fi.TableName, &fi.IndexName,
				&fi.FragmentationPct, &fi.PageCount, &fi.RecommendedAction,
			); err != nil {
				continue
			}
			metrics.FragmentedIndexes = append(metrics.FragmentedIndexes, fi)
		}
	}

	return metrics, nil
}

package main

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// IntegrityMetrics holds DBCC CHECKDB results and corruption indicators.
type IntegrityMetrics struct {
	ServerName  string
	CollectedAt time.Time
	Databases   []DBIntegrity
}

// DBIntegrity is the integrity status for one database.
type DBIntegrity struct {
	DatabaseName      string
	LastCheckDBDate   *time.Time
	DaysSinceCheckDB  float64
	ErrorCount        int
	WarningCount      int
	IsCorrupt         bool
	CheckDBOverdue    bool   // > 7 days since last CHECKDB
	SuspectPages      int
	LastCheckDBResult string
}

// CollectIntegrity reads DBCC history from msdb and suspect_pages.
// It does NOT run CHECKDB itself (that would be very expensive).
func CollectIntegrity(ctx context.Context, db *sql.DB, serverName string) (*IntegrityMetrics, error) {
	m := &IntegrityMetrics{ServerName: serverName, CollectedAt: time.Now()}

	// ── Last successful DBCC CHECKDB date from msdb ───────────────────────────
	// SQL Server records CHECKDB completion in msdb.dbo.suspect_pages and
	// DBCC DBINFO stores the last known clean check date.
	checkSQL := `
		SELECT
			d.name                                                  AS db_name,
			ISNULL(
				(SELECT MAX(event_time)
				 FROM msdb.dbo.restorehistory rh
				 WHERE rh.destination_database_name = d.name), NULL) AS last_restore,
			(SELECT COUNT(*) FROM msdb.dbo.suspect_pages sp
			 WHERE sp.database_id = d.database_id
			   AND sp.event_type IN (1,2,3,4))                      AS suspect_page_count
		FROM sys.databases d
		WHERE d.database_id > 4
		  AND d.state_desc = 'ONLINE'
		ORDER BY d.name`

	rows, err := db.QueryContext(ctx, checkSQL)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	now := time.Now()
	for rows.Next() {
		var dbi DBIntegrity
		var lastRestore sql.NullTime
		if err := rows.Scan(&dbi.DatabaseName, &lastRestore, &dbi.SuspectPages); err != nil {
			continue
		}

		// Get CHECKDB date from DBCC DBINFO (most reliable source)
		var checkDate sql.NullTime
		dbInfoRows, err := db.QueryContext(ctx,
			fmt.Sprintf("DBCC DBINFO([%s]) WITH TABLERESULTS, NO_INFOMSGS", dbi.DatabaseName))
		if err == nil {
			for dbInfoRows.Next() {
				var parentObj, obj, field, val string
				var idx int
				if err := dbInfoRows.Scan(&parentObj, &obj, &field, &idx, &val); err != nil {
					continue
				}
				if field == "dbi_dbccLastKnownGood" && val != "1900-01-01 00:00:00.000" {
					t, err := time.Parse("2006-01-02 15:04:05.000", val)
					if err == nil {
						checkDate = sql.NullTime{Time: t, Valid: true}
					}
				}
			}
			dbInfoRows.Close()
		}

		if checkDate.Valid {
			t := checkDate.Time
			dbi.LastCheckDBDate = &t
			dbi.DaysSinceCheckDB = now.Sub(t).Hours() / 24
		} else {
			dbi.DaysSinceCheckDB = 999
		}

		dbi.IsCorrupt = dbi.SuspectPages > 0
		dbi.CheckDBOverdue = dbi.DaysSinceCheckDB > 7

		if dbi.SuspectPages > 0 {
			dbi.LastCheckDBResult = fmt.Sprintf("⚠ %d suspect page(s) found", dbi.SuspectPages)
		} else if dbi.CheckDBOverdue {
			dbi.LastCheckDBResult = fmt.Sprintf("⏰ Overdue — %.0f days since last CHECKDB", dbi.DaysSinceCheckDB)
		} else {
			dbi.LastCheckDBResult = "✓ OK"
		}

		m.Databases = append(m.Databases, dbi)
	}
	return m, rows.Err()
}

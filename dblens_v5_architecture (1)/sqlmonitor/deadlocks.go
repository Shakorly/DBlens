package main

import (
	"context"
	"database/sql"
	"encoding/xml"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// CollectDeadlocks reads deadlock graphs from the system_health extended event
// session, which is enabled by default on SQL Server 2012+.
func CollectDeadlocks(ctx context.Context, db *sql.DB, serverName string, since time.Time) ([]DeadlockEvent, error) {
	// system_health ring buffer stores deadlock XML — no agent or XE session setup needed.
	deadlockSQL := `
		SELECT
			xdr.value('@timestamp','datetime2')          AS event_time,
			CAST(xdr.query('.') AS NVARCHAR(MAX))        AS deadlock_xml
		FROM (
			SELECT CAST(target_data AS XML) AS target_data
			FROM sys.dm_xe_session_targets  t
			JOIN sys.dm_xe_sessions          s ON t.event_session_address = s.address
			WHERE s.name = 'system_health'
			  AND t.target_name = 'ring_buffer'
		) AS data
		CROSS APPLY target_data.nodes('//RingBufferTarget/event[@name="xml_deadlock_report"]') AS xdt(xdr)
		WHERE xdr.value('@timestamp','datetime2') > @since
		ORDER BY event_time DESC`

	rows, err := db.QueryContext(ctx, deadlockSQL,
		sql.Named("since", since.Format("2006-01-02T15:04:05")))
	if err != nil {
		// XE not available on older SQL Server — treat as non-fatal
		return nil, nil
	}
	defer rows.Close()

	var events []DeadlockEvent
	for rows.Next() {
		var evtTime time.Time
		var rawXML string
		if err := rows.Scan(&evtTime, &rawXML); err != nil {
			continue
		}
		evt := parseDeadlockXML(rawXML, evtTime)
		events = append(events, evt)
	}
	return events, rows.Err()
}

// ── Minimal XML structs for deadlock graph parsing ────────────────────────────

type deadlockXML struct {
	XMLName    xml.Name       `xml:"event"`
	Data       []deadlockData `xml:"data"`
}

type deadlockData struct {
	Name  string         `xml:"name,attr"`
	Value deadlockValue  `xml:"value"`
}

type deadlockValue struct {
	Deadlock *deadlockGraph `xml:"deadlock"`
}

type deadlockGraph struct {
	VictimList []deadlockVictim  `xml:"victim-list>victimProcess"`
	ProcessList []deadlockProcess `xml:"process-list>process"`
}

type deadlockVictim struct {
	ID string `xml:"id,attr"`
}

type deadlockProcess struct {
	ID           string `xml:"id,attr"`
	SPID         string `xml:"spid,attr"`
	LoginName    string `xml:"loginname,attr"`
	DatabaseName string `xml:"currentdb,attr"`
	WaitResource string `xml:"waitresource,attr"`
	InputBuf     string `xml:"inputbuf"`
}

func parseDeadlockXML(raw string, ts time.Time) DeadlockEvent {
	evt := DeadlockEvent{OccurredAt: ts, RawXML: raw}

	// Find the inner <deadlock> graph element
	start := strings.Index(raw, "<deadlock>")
	if start == -1 {
		start = strings.Index(raw, "<deadlock ")
	}
	if start == -1 {
		return evt
	}
	end := strings.LastIndex(raw, "</deadlock>")
	if end == -1 {
		return evt
	}
	graphXML := raw[start : end+len("</deadlock>")]

	type graph struct {
		XMLName     xml.Name `xml:"deadlock"`
		VictimList  []struct {
			ID string `xml:"id,attr"`
		} `xml:"victim-list>victimProcess"`
		ProcessList []struct {
			ID           string `xml:"id,attr"`
			SPID         string `xml:"spid,attr"`
			LoginName    string `xml:"loginname,attr"`
			DBName       string `xml:"currentdb,attr"`
			WaitResource string `xml:"waitresource,attr"`
			InputBuf     string `xml:"inputbuf"`
		} `xml:"process-list>process"`
	}

	var g graph
	if err := xml.Unmarshal([]byte(graphXML), &g); err != nil {
		return evt
	}

	victimIDs := map[string]bool{}
	for _, v := range g.VictimList {
		victimIDs[v.ID] = true
		if spid, err := strconv.Atoi(v.ID); err == nil {
			evt.VictimSPID = spid
		}
	}

	for _, p := range g.ProcessList {
		spid, _ := strconv.Atoi(p.SPID)
		qtext := strings.TrimSpace(p.InputBuf)
		if len(qtext) > 400 {
			qtext = qtext[:400] + "..."
		}
		proc := DeadlockProcess{
			SPID:         spid,
			LoginName:    p.LoginName,
			Database:     p.DBName,
			WaitResource: p.WaitResource,
			QueryText:    qtext,
			IsVictim:     victimIDs[p.ID],
		}
		evt.Processes = append(evt.Processes, proc)
	}

	return evt
}

// ── Alert deduplication ───────────────────────────────────────────────────────

// DeadlockTracker keeps seen deadlock timestamps to avoid re-alerting.
type DeadlockTracker struct {
	seen map[string]bool
}

func NewDeadlockTracker() *DeadlockTracker {
	return &DeadlockTracker{seen: make(map[string]bool)}
}

func (dt *DeadlockTracker) IsNew(evt DeadlockEvent) bool {
	key := fmt.Sprintf("%s|%d", evt.OccurredAt.Format(time.RFC3339), evt.VictimSPID)
	if dt.seen[key] {
		return false
	}
	dt.seen[key] = true
	// Prune old entries to avoid unbounded growth
	if len(dt.seen) > 1000 {
		dt.seen = make(map[string]bool)
		dt.seen[key] = true
	}
	return true
}

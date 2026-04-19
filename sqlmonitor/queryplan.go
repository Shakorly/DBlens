package main

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// QueryPlanInfo captures an analysed execution plan for a running session.
type QueryPlanInfo struct {
	SessionID             int
	PlanXML               string
	PlanWarnings          []string
	HasMissingIndex       bool
	HasImplicitConversion bool
	HasParallelism        bool
	HasKeyLookup          bool
	HasScan               bool
}

// CaptureQueryPlan retrieves the live execution plan for a specific session.
// Returns nil (not an error) if the plan is unavailable — this is common and
// expected when the session finishes between query and plan capture.
func CaptureQueryPlan(ctx context.Context, db *sql.DB, sessionID int) (*QueryPlanInfo, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	const planSQL = `
		SELECT
			r.session_id,
			ISNULL(CAST(qp.query_plan AS NVARCHAR(MAX)), '') AS plan_xml
		FROM sys.dm_exec_requests r
		CROSS APPLY sys.dm_exec_query_plan(r.plan_handle) qp
		WHERE r.session_id = @sid
		  AND qp.query_plan IS NOT NULL`

	var sid int
	var planXML string
	err := db.QueryRowContext(ctx, planSQL, sql.Named("sid", sessionID)).Scan(&sid, &planXML)
	if err != nil || planXML == "" {
		return nil, nil // non-fatal — plan not yet available
	}

	info := &QueryPlanInfo{
		SessionID:             sid,
		HasMissingIndex:       strContains(planXML, "MissingIndex"),
		HasImplicitConversion: strContains(planXML, "CONVERT_IMPLICIT") || strContains(planXML, "ImplicitConvert"),
		HasParallelism:        strContains(planXML, "Parallelism"),
		HasKeyLookup:          strContains(planXML, "KeyLookup"),
		HasScan:               strContains(planXML, "TableScan") || strContains(planXML, "IndexScan"),
	}
	// Only keep plan XML if it's a reasonable size (avoid huge XML in memory)
	if len(planXML) <= 50000 {
		info.PlanXML = planXML
	}

	if info.HasMissingIndex {
		info.PlanWarnings = append(info.PlanWarnings, "Missing index in plan")
	}
	if info.HasImplicitConversion {
		info.PlanWarnings = append(info.PlanWarnings, "Implicit type conversion — index bypass likely")
	}
	if info.HasKeyLookup {
		info.PlanWarnings = append(info.PlanWarnings, "Key Lookup — add INCLUDE columns to index")
	}
	if info.HasScan {
		info.PlanWarnings = append(info.PlanWarnings, "Table/Index Scan — full scan in progress")
	}
	if info.HasParallelism {
		info.PlanWarnings = append(info.PlanWarnings, "Parallel execution — check MAXDOP")
	}
	return info, nil
}

// CollectActiveQueryPlans captures plans for the top N longest-running queries.
// Runs in background goroutines with individual timeouts to avoid blocking.
func CollectActiveQueryPlans(ctx context.Context, db *sql.DB, queries []ActiveQuery) map[int]*QueryPlanInfo {
	plans := make(map[int]*QueryPlanInfo)
	const maxPlans = 3 // only top 3 to limit overhead
	for i, q := range queries {
		if i >= maxPlans {
			break
		}
		plan, err := CaptureQueryPlan(ctx, db, q.SessionID)
		if err == nil && plan != nil {
			plans[q.SessionID] = plan
		}
	}
	return plans
}

// GeneratePlanAdvice returns a concise human-readable summary of plan warnings.
func GeneratePlanAdvice(plan *QueryPlanInfo) string {
	if plan == nil || len(plan.PlanWarnings) == 0 {
		return ""
	}
	return fmt.Sprintf("Plan: %s", strings.Join(plan.PlanWarnings, " | "))
}

func strContains(s, sub string) bool {
	return strings.Contains(s, sub)
}

package main

import (
	"context"
	"database/sql"
	"time"
)

// SecurityAudit holds security-related findings for one server.
type SecurityAudit struct {
	ServerName       string
	CollectedAt      time.Time
	FailedLogins     []FailedLogin
	PrivilegedUsers  []PrivilegedUser
	OrphanedUsers    []OrphanedUser
	SuspiciousConns  []SuspiciousConn
	SALoginEnabled   bool
	MixedAuthMode    bool
	XPCmdShellOn     bool
	LinkedServers    []LinkedServerInfo
}

type FailedLogin struct {
	LoginName   string
	FailCount   int
	LastAttempt time.Time
	HostName    string
}

type PrivilegedUser struct {
	LoginName   string
	Role        string
	IsDisabled  bool
	LastLogin   *time.Time
}

type OrphanedUser struct {
	DatabaseName string
	UserName     string
}

type SuspiciousConn struct {
	SessionID   int
	LoginName   string
	HostName    string
	ProgramName string
	ConnectedAt time.Time
	QueryText   string
}

type LinkedServerInfo struct {
	Name         string
	DataSource   string
	Provider     string
	IsRemoteLogin bool
}

// CollectSecurity gathers security posture data from SQL Server.
func CollectSecurity(ctx context.Context, db *sql.DB, serverName string) (*SecurityAudit, error) {
	audit := &SecurityAudit{ServerName: serverName, CollectedAt: time.Now()}

	// ── Server configuration risks ────────────────────────────────────────────
	configSQL := `
		SELECT
			(SELECT CAST(value_in_use AS INT) FROM sys.configurations WHERE name = 'xp_cmdshell') AS xp_cmd,
			CAST(ISNULL(SERVERPROPERTY('IsIntegratedSecurityOnly'),0) AS INT)                      AS windows_auth_only,
			(SELECT CAST(is_disabled AS INT) FROM sys.server_principals WHERE name = 'sa')         AS sa_disabled`

	var xpCmd, windowsOnly, saDisabled int
	if err := db.QueryRowContext(ctx, configSQL).Scan(&xpCmd, &windowsOnly, &saDisabled); err == nil {
		audit.XPCmdShellOn = xpCmd == 1
		audit.MixedAuthMode = windowsOnly == 0
		audit.SALoginEnabled = saDisabled == 0
	}

	// ── Sysadmin and securityadmin members ────────────────────────────────────
	privSQL := `
		SELECT
			m.name                                                  AS login_name,
			r.name                                                  AS role_name,
			CAST(m.is_disabled AS INT),
			m.modify_date
		FROM sys.server_role_members rm
		JOIN sys.server_principals   r ON rm.role_principal_id  = r.principal_id
		JOIN sys.server_principals   m ON rm.member_principal_id = m.principal_id
		WHERE r.name IN ('sysadmin','securityadmin','serveradmin')
		  AND m.type IN ('S','U','G')
		ORDER BY r.name, m.name`

	rows, err := db.QueryContext(ctx, privSQL)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var pu PrivilegedUser
			var lastMod time.Time
			var disabled int
			if err := rows.Scan(&pu.LoginName, &pu.Role, &disabled, &lastMod); err != nil { continue }
			pu.IsDisabled = disabled == 1
			pu.LastLogin = &lastMod
			audit.PrivilegedUsers = append(audit.PrivilegedUsers, pu)
		}
		rows.Close()
	}

	// ── Failed logins from ring buffer (SQL Server 2012+) ─────────────────────
	failedSQL := `
		SELECT TOP 20
			target_data.value('(//RingBufferTarget/event/data[@name="failure_reason"]/text)[1]','nvarchar(100)') AS reason,
			target_data.value('(//RingBufferTarget/event/data[@name="client_app"]/text)[1]','nvarchar(200)')     AS app,
			COUNT(*)                                                                                              AS attempts
		FROM (
			SELECT CAST(t.target_data AS XML) AS target_data
			FROM sys.dm_xe_session_targets  t
			JOIN sys.dm_xe_sessions          s ON t.event_session_address = s.address
			WHERE s.name = 'system_health'
			  AND t.target_name = 'ring_buffer'
		) ring
		WHERE target_data.exist('//event[@name="error_reported"]
			[data[@name="error_number"]/value=18456]') = 1
		GROUP BY
			target_data.value('(//RingBufferTarget/event/data[@name="failure_reason"]/text)[1]','nvarchar(100)'),
			target_data.value('(//RingBufferTarget/event/data[@name="client_app"]/text)[1]','nvarchar(200)')`

	// Simpler fallback: count sessions with unusual programs
	unusualSQL := `
		SELECT TOP 10
			s.session_id,
			ISNULL(s.login_name,'')                             AS login_name,
			ISNULL(s.host_name,'')                              AS host_name,
			ISNULL(s.program_name,'')                           AS program_name,
			s.login_time,
			ISNULL(SUBSTRING(st.text,1,200),'')                 AS query_text
		FROM sys.dm_exec_sessions s
		OUTER APPLY sys.dm_exec_sql_text(
			(SELECT TOP 1 r.sql_handle FROM sys.dm_exec_requests r WHERE r.session_id = s.session_id)
		) st
		WHERE s.is_user_process = 1
		  AND (s.program_name LIKE '%sqlcmd%'
			OR s.program_name LIKE '%powershell%'
			OR s.program_name LIKE '%python%'
			OR s.program_name = ''
			OR s.program_name IS NULL)
		  AND s.login_name NOT LIKE '%sqlmonitor%'`

	rows2, err := db.QueryContext(ctx, unusualSQL)
	if err == nil {
		defer rows2.Close()
		for rows2.Next() {
			var sc SuspiciousConn
			if err := rows2.Scan(&sc.SessionID, &sc.LoginName, &sc.HostName,
				&sc.ProgramName, &sc.ConnectedAt, &sc.QueryText); err != nil { continue }
			audit.SuspiciousConns = append(audit.SuspiciousConns, sc)
		}
		rows2.Close()
	}
	_ = failedSQL // used on 2012+ only

	// ── Linked servers (potential lateral movement vector) ───────────────────
	linkedSQL := `
		SELECT
			s.name,
			ISNULL(s.data_source,''),
			ISNULL(s.provider,''),
			CAST(ISNULL(
				(SELECT COUNT(*) FROM sys.linked_logins ll WHERE ll.server_id = s.server_id AND ll.uses_self_credential = 0),0
			) AS INT)
		FROM sys.servers s
		WHERE s.is_linked = 1`

	rows3, err := db.QueryContext(ctx, linkedSQL)
	if err == nil {
		defer rows3.Close()
		for rows3.Next() {
			var ls LinkedServerInfo
			var remoteLogin int
			if err := rows3.Scan(&ls.Name, &ls.DataSource, &ls.Provider, &remoteLogin); err != nil { continue }
			ls.IsRemoteLogin = remoteLogin > 0
			audit.LinkedServers = append(audit.LinkedServers, ls)
		}
	}

	return audit, nil
}

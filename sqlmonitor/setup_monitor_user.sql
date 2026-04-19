-- ============================================================
-- SQL Server Monitor – Minimum Privilege Setup Script
-- Run this on EACH SQL Server instance you want to monitor.
-- Replace 'YourStrongPassword!' with a secure password.
-- ============================================================

USE master;
GO

-- 1. Create a dedicated login for the monitoring tool
IF NOT EXISTS (SELECT 1 FROM sys.server_principals WHERE name = 'sqlmonitor_user')
BEGIN
    CREATE LOGIN sqlmonitor_user
    WITH PASSWORD    = 'YourStrongPassword!',
         CHECK_POLICY = ON,
         CHECK_EXPIRATION = OFF;
    PRINT 'Login sqlmonitor_user created.';
END
GO

-- 2. Grant server-level permissions required by the DMVs
--    VIEW SERVER STATE  → sys.dm_exec_sessions, sys.dm_exec_requests,
--                         sys.dm_exec_query_stats, sys.dm_os_ring_buffers,
--                         sys.dm_os_sys_memory, sys.dm_os_process_memory,
--                         sys.dm_io_virtual_file_stats, AG health DMVs
GRANT VIEW SERVER STATE TO sqlmonitor_user;
GO

-- 3. Grant VIEW ANY DATABASE so the tool can see all database names
GRANT VIEW ANY DATABASE TO sqlmonitor_user;
GO

-- 4. Create a user in master (needed for login to work and to read master_files)
USE master;
GO
IF NOT EXISTS (SELECT 1 FROM sys.database_principals WHERE name = 'sqlmonitor_user')
BEGIN
    CREATE USER sqlmonitor_user FOR LOGIN sqlmonitor_user;
    PRINT 'User sqlmonitor_user created in master.';
END
GO

-- 5. For each user database you are monitoring, run the block below.
--    Replace 'YourDatabaseName' with the actual database name.
--    (Repeat for every database listed in your config.yaml)

/*
USE YourDatabaseName;
GO
IF NOT EXISTS (SELECT 1 FROM sys.database_principals WHERE name = 'sqlmonitor_user')
BEGIN
    CREATE USER sqlmonitor_user FOR LOGIN sqlmonitor_user;
END
-- db_datareader is NOT required just for DMV-based monitoring.
-- Add it only if you need query-level detail from user tables.
-- EXEC sp_addrolemember 'db_datareader', 'sqlmonitor_user';
GO
*/

-- 6. Verify permissions
SELECT
    sp.name            AS principal_name,
    sp.type_desc       AS principal_type,
    perm.permission_name,
    perm.state_desc
FROM sys.server_permissions perm
JOIN sys.server_principals  sp ON perm.grantee_principal_id = sp.principal_id
WHERE sp.name = 'sqlmonitor_user';
GO

PRINT '=== Setup complete. Test with: EXECUTE AS LOGIN = ''sqlmonitor_user''; SELECT @@SERVERNAME; REVERT; ===';
GO

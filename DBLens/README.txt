═══════════════════════════════════════════════════════════════
  DBLens Monitor — Quick Start
═══════════════════════════════════════════════════════════════

WHAT YOU NEED IN THIS FOLDER
──────────────────────────────
  sqlmonitor.exe    
  config.json        
  DBLens.bat         ← the ONE file you double-click for everything
  Manage-Servers.ps1


HOW TO START (3 steps)
──────────────────────

  1. Double-click  DBLens.bat
     Choose option 2 — "Add / Edit servers"
     Enter your SQL Server name, host, port, username, password.
     Press Enter after each field.
     Type 5 when done to go back.

  2. Choose option 1 — "Start DBLens"
     The dashboard opens automatically at http://localhost:8080
     Keep the window open. Close it to stop.


ADD MORE SERVERS LATER
──────────────────────
  Double-click DBLens.bat → option 2 → option 1 (Add a server)
  Add as many servers as you like. Each gets its own tab
  in the dashboard.


UPDATE sqlmonitor.exe 
──────────────────────────────────────────────────
  Double-click DBLens.bat → option 4
  Follow the on-screen instructions (copy new exe, press Enter).
  Done.


DASHBOARD NOT SHOWING?
──────────────────────
  • Make sure DBLens is running (window must be open)
  • Go to  http://localhost:8080  in your browser manually
  • Wait 60 seconds after first start — first data takes one poll cycle
  • Check  dblens.log  in this folder for error details
    (DBLens.bat option 5 shows the last 40 lines)


SQL SERVER PERMISSION SETUP (one-time per server)
──────────────────────────────────────────────────
  Run this SQL on each server you want to monitor:

    CREATE LOGIN sqlmonitor_user
      WITH PASSWORD = 'YourPassword!';
    GRANT VIEW SERVER STATE TO sqlmonitor_user;
    GRANT VIEW ANY DATABASE  TO sqlmonitor_user;

  Use the same username/password when DBLens asks.


═══════════════════════════════════════════════════════════════

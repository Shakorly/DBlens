# ================================================
# DBLens Monitor v5 - Secure Startup Script
# Run this instead of sqlmonitor.exe directly
# ================================================

# === SET PASSWORDS HERE (only change the password value) ===
$env:DBLENS_PASS_ServerName = "YourSqlmonitorPassword"


# Optional: Global fallback password (uncomment if needed)
# $env:DBLENS_PASS = "YourStrongPassword"

Write-Host "✅ DBLens passwords loaded successfully" -ForegroundColor Green


# Optional: Change dashboard port if you want (example: 8085)
# $env:DBLENS_DASHBOARD_PORT = "8085"

Write-Host "`n🚀 Starting DBLens Monitor..." -ForegroundColor Yellow

# Start the monitor
.\sqlmonitor.exe

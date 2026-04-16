package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
)

type DashboardStore struct {
	mu      sync.RWMutex
	results map[string]*CollectionResult
}

func NewDashboardStore() *DashboardStore {
	return &DashboardStore{results: make(map[string]*CollectionResult)}
}

func (ds *DashboardStore) Update(r *CollectionResult) {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	ds.results[r.ServerName] = r
}

func StartDashboard(port int, store *DashboardStore, hs *HistoryStore, logger *Logger) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, dashboardHTML)
	})
	mux.HandleFunc("/api/results", func(w http.ResponseWriter, r *http.Request) {
		store.mu.RLock(); defer store.mu.RUnlock()
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		json.NewEncoder(w).Encode(store.results)
	})
	mux.HandleFunc("/api/history", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		json.NewEncoder(w).Encode(hs.GetAll())
	})
	addr := fmt.Sprintf(":%d", port)
	logger.Info("", fmt.Sprintf("🌐 Dashboard → http://localhost%s", addr))
	if err := http.ListenAndServe(addr, mux); err != nil {
		logger.Error("", "Dashboard: "+err.Error())
	}
}

const dashboardHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>DBLens — SQL Server Monitor</title>
<script src="https://cdn.jsdelivr.net/npm/chart.js@4.4.0/dist/chart.umd.min.js"></script>
<style>
:root{
  --bg:#07090f;--surface:#0e1117;--card:#131720;--border:#1e2535;
  --accent:#3b82f6;--accent2:#8b5cf6;--green:#10b981;--yellow:#f59e0b;
  --red:#ef4444;--orange:#f97316;--cyan:#06b6d4;--pink:#ec4899;
  --text:#e2e8f0;--sub:#64748b;--sub2:#94a3b8;
  --grad1:linear-gradient(135deg,#3b82f6,#8b5cf6);
  --grad2:linear-gradient(135deg,#10b981,#06b6d4);
  --grad3:linear-gradient(135deg,#f59e0b,#ef4444);
}
*{box-sizing:border-box;margin:0;padding:0;}
html{scroll-behavior:smooth;}
body{background:var(--bg);color:var(--text);font-family:'Segoe UI',system-ui,sans-serif;overflow-x:hidden;}

/* ── Scrollbar ── */
::-webkit-scrollbar{width:6px;height:6px}
::-webkit-scrollbar-track{background:var(--bg)}
::-webkit-scrollbar-thumb{background:var(--border);border-radius:3px}

/* ── Topbar ── */
#topbar{position:sticky;top:0;z-index:100;background:rgba(7,9,15,.92);
  backdrop-filter:blur(12px);border-bottom:1px solid var(--border);
  display:flex;align-items:center;padding:0 24px;height:56px;gap:16px;}
#topbar .logo{display:flex;align-items:center;gap:10px;font-weight:800;font-size:1.1rem;
  background:var(--grad1);-webkit-background-clip:text;-webkit-text-fill-color:transparent;}
#topbar .logo svg{flex-shrink:0}
.top-stats{display:flex;gap:6px;margin-left:auto;}
.top-stat{background:var(--card);border:1px solid var(--border);border-radius:8px;
  padding:6px 14px;font-size:.78rem;display:flex;flex-direction:column;align-items:center;min-width:70px;}
.top-stat .tv{font-size:1.1rem;font-weight:800;line-height:1;}
.top-stat .tl{color:var(--sub);font-size:.65rem;margin-top:2px;text-transform:uppercase;letter-spacing:.05em;}
#live-dot{width:8px;height:8px;border-radius:50%;background:var(--green);
  animation:pulse 2s infinite;box-shadow:0 0 6px var(--green);}
@keyframes pulse{0%,100%{opacity:1}50%{opacity:.4}}
#last-update{font-size:.72rem;color:var(--sub);white-space:nowrap;}

/* ── Nav tabs ── */
#nav{background:var(--surface);border-bottom:1px solid var(--border);
  display:flex;gap:0;padding:0 24px;overflow-x:auto;}
.nav-tab{padding:12px 18px;font-size:.82rem;cursor:pointer;color:var(--sub);
  border-bottom:2px solid transparent;transition:.2s;white-space:nowrap;font-weight:500;}
.nav-tab:hover{color:var(--text);}
.nav-tab.active{color:var(--accent);border-bottom-color:var(--accent);}

/* ── Page sections ── */
.page{display:none;padding:24px;max-width:1800px;margin:0 auto;animation:fadeIn .3s;}
.page.active{display:block;}
@keyframes fadeIn{from{opacity:0;transform:translateY(8px)}to{opacity:1;transform:none}}

/* ── KPI bar ── */
.kpi-bar{display:grid;grid-template-columns:repeat(auto-fill,minmax(160px,1fr));gap:12px;margin-bottom:24px;}
.kpi{background:var(--card);border:1px solid var(--border);border-radius:12px;
  padding:16px;position:relative;overflow:hidden;}
.kpi::before{content:'';position:absolute;top:0;left:0;right:0;height:3px;}
.kpi.blue::before{background:var(--grad1);}
.kpi.green::before{background:var(--grad2);}
.kpi.amber::before{background:linear-gradient(90deg,var(--yellow),var(--orange));}
.kpi.red::before{background:linear-gradient(90deg,var(--orange),var(--red));}
.kpi.purple::before{background:linear-gradient(90deg,var(--accent2),var(--pink));}
.kpi.cyan::before{background:linear-gradient(90deg,var(--cyan),var(--green));}
.kpi-val{font-size:2rem;font-weight:900;line-height:1.1;margin:6px 0 4px;}
.kpi-label{font-size:.72rem;color:var(--sub);text-transform:uppercase;letter-spacing:.06em;font-weight:600;}
.kpi-sub{font-size:.72rem;color:var(--sub2);margin-top:2px;}

/* ── Grid layouts ── */
.grid-2{display:grid;grid-template-columns:repeat(2,1fr);gap:16px;}
.grid-3{display:grid;grid-template-columns:repeat(3,1fr);gap:16px;}
.grid-4{display:grid;grid-template-columns:repeat(4,1fr);gap:16px;}
@media(max-width:1200px){.grid-4{grid-template-columns:repeat(2,1fr)}}
@media(max-width:900px){.grid-3,.grid-2{grid-template-columns:1fr}.grid-4{grid-template-columns:1fr}}

/* ── Cards ── */
.card{background:var(--card);border:1px solid var(--border);border-radius:12px;overflow:hidden;}
.card-header{padding:14px 18px;border-bottom:1px solid var(--border);
  display:flex;align-items:center;gap:10px;}
.card-title{font-weight:700;font-size:.9rem;}
.card-body{padding:16px 18px;}
.card-icon{width:32px;height:32px;border-radius:8px;display:flex;align-items:center;
  justify-content:center;font-size:1rem;flex-shrink:0;}
.ci-blue{background:#3b82f620;}
.ci-green{background:#10b98120;}
.ci-amber{background:#f59e0b20;}
.ci-red{background:#ef444420;}
.ci-purple{background:#8b5cf620;}
.ci-cyan{background:#06b6d420;}

/* ── Server health card ── */
.srv-grid{display:grid;grid-template-columns:repeat(auto-fill,minmax(520px,1fr));gap:16px;}
.srv-card{background:var(--card);border:1px solid var(--border);border-radius:14px;overflow:hidden;}
.srv-card-head{padding:16px 20px;display:flex;align-items:center;gap:14px;
  border-bottom:1px solid var(--border);}
.grade-ring{width:52px;height:52px;border-radius:50%;display:flex;align-items:center;
  justify-content:center;font-size:1.5rem;font-weight:900;border:3px solid;flex-shrink:0;}
.srv-meta{flex:1;}
.srv-name{font-weight:800;font-size:1rem;}
.srv-detail{font-size:.75rem;color:var(--sub);margin-top:2px;}
.score-wrap{text-align:right;}
.score-big{font-size:1.8rem;font-weight:900;}
.score-track{width:90px;height:6px;border-radius:3px;background:var(--border);margin-top:5px;overflow:hidden;margin-left:auto;}
.score-fill{height:100%;border-radius:3px;transition:width .6s;}
.mini-stats{display:grid;grid-template-columns:repeat(6,1fr);padding:12px 20px;
  border-bottom:1px solid var(--border);gap:4px;}
.ms{text-align:center;padding:6px 4px;border-radius:8px;background:#0f111a;}
.ms-v{font-size:1rem;font-weight:800;}
.ms-l{font-size:.62rem;color:var(--sub);margin-top:2px;}
.srv-tabs{display:flex;gap:0;border-bottom:1px solid var(--border);overflow-x:auto;}
.stab{padding:8px 14px;font-size:.75rem;cursor:pointer;color:var(--sub);
  border-bottom:2px solid transparent;transition:.15s;white-space:nowrap;}
.stab.active{color:var(--accent);border-bottom-color:var(--accent);}
.stab-content{display:none;padding:14px 20px;min-height:120px;}
.stab-content.active{display:block;}

/* ── Tables ── */
.dt{width:100%;border-collapse:collapse;font-size:.76rem;}
.dt th{color:var(--sub);text-align:left;padding:6px 10px;border-bottom:1px solid var(--border);
  font-size:.68rem;text-transform:uppercase;letter-spacing:.05em;font-weight:700;}
.dt td{padding:7px 10px;border-bottom:1px solid #131720;}
.dt tr:last-child td{border-bottom:none;}
.dt tr:hover td{background:#0f1117;}

/* ── Badges ── */
.badge{display:inline-flex;align-items:center;border-radius:5px;padding:2px 8px;font-size:.68rem;font-weight:700;}
.b-ok{background:#10b98120;color:#10b981;}
.b-warn{background:#f59e0b20;color:#f59e0b;}
.b-crit{background:#ef444420;color:#ef4444;}
.b-blue{background:#3b82f620;color:#3b82f6;}
.b-purple{background:#8b5cf620;color:#8b5cf6;}

/* ── Penalty list ── */
.pen-list{display:flex;flex-direction:column;gap:6px;}
.pen-item{display:flex;align-items:center;gap:8px;font-size:.78rem;padding:7px 10px;
  background:#0f111a;border-radius:8px;border-left:3px solid var(--yellow);}
.pen-item.ok{border-left-color:var(--green);color:var(--green);}

/* ── Chart containers ── */
.chart-wrap{position:relative;padding:4px 0;}
.chart-wrap canvas{max-height:200px!important;}
.chart-sm canvas{max-height:140px!important;}
.chart-lg canvas{max-height:260px!important;}

/* ── Progress bars ── */
.prog-bar{height:8px;border-radius:4px;background:var(--border);overflow:hidden;margin-top:4px;}
.prog-fill{height:100%;border-radius:4px;transition:width .5s;}

/* ── Alert feed ── */
.alert-feed{display:flex;flex-direction:column;gap:6px;max-height:400px;overflow-y:auto;}
.alert-item{display:flex;align-items:flex-start;gap:10px;padding:10px 12px;
  border-radius:8px;border:1px solid;font-size:.78rem;}
.alert-item.crit{background:#ef444410;border-color:#ef444430;}
.alert-item.warn{background:#f59e0b10;border-color:#f59e0b30;}
.alert-item.info{background:#3b82f610;border-color:#3b82f630;}
.alert-icon{font-size:1rem;flex-shrink:0;margin-top:1px;}
.alert-body{flex:1;}
.alert-server{font-weight:700;font-size:.72rem;color:var(--sub2);}
.alert-msg{margin-top:1px;}
.alert-time{font-size:.68rem;color:var(--sub);margin-top:2px;}

/* ── Uptime indicator ── */
.uptime-blocks{display:flex;gap:3px;flex-wrap:wrap;margin-top:8px;}
.ub{width:14px;height:14px;border-radius:3px;}

/* ── Misc ── */
.no-data{color:var(--sub);font-size:.8rem;font-style:italic;padding:12px 0;}
.section-title{font-size:.72rem;text-transform:uppercase;letter-spacing:.07em;color:var(--sub);
  font-weight:700;margin-bottom:10px;}
.risk-row{display:flex;align-items:center;gap:8px;padding:6px 0;border-bottom:1px solid #131720;font-size:.78rem;}
.risk-row:last-child{border-bottom:none;}
.risk-indicator{width:10px;height:10px;border-radius:50%;flex-shrink:0;}
.tag{display:inline-block;background:var(--border);border-radius:4px;padding:1px 7px;
  font-size:.68rem;color:var(--sub2);margin:2px;}
</style>
</head>
<body>

<!-- TOP BAR -->
<div id="topbar">
  <div class="logo">
    <svg width="28" height="28" viewBox="0 0 28 28" fill="none">
      <ellipse cx="14" cy="8" rx="10" ry="4" fill="#3b82f6" opacity=".9"/>
      <path d="M4 8v6c0 2.2 4.5 4 10 4s10-1.8 10-4V8" fill="#3b82f6" opacity=".6"/>
      <path d="M4 14v6c0 2.2 4.5 4 10 4s10-1.8 10-4v-6" fill="#3b82f6" opacity=".35"/>
    </svg>
    DBLens Monitor
  </div>
  <div class="top-stats" id="top-stats"></div>
  <div id="live-dot"></div>
  <span id="last-update">Connecting...</span>
</div>

<!-- NAV -->
<div id="nav">
  <div class="nav-tab active" onclick="showPage('overview',this)">📊 Overview</div>
  <div class="nav-tab" onclick="showPage('servers',this)">🖥 Servers</div>
  <div class="nav-tab" onclick="showPage('performance',this)">⚡ Performance</div>
  <div class="nav-tab" onclick="showPage('queries',this)">🔍 Queries</div>
  <div class="nav-tab" onclick="showPage('backups',this)">💾 Backups</div>
  <div class="nav-tab" onclick="showPage('security',this)">🔒 Security</div>
  <div class="nav-tab" onclick="showPage('capacity',this)">📈 Capacity</div>
  <div class="nav-tab" onclick="showPage('inventory',this)">📋 Inventory</div>
  <div class="nav-tab" onclick="showPage('alerts',this)">🚨 Alerts</div>
</div>

<!-- ═══════════════════════════════ OVERVIEW ═══════════════════════════════ -->
<div class="page active" id="page-overview">
  <div class="kpi-bar" id="kpi-bar"></div>
  <div class="grid-2" style="margin-bottom:16px">
    <div class="card">
      <div class="card-header"><div class="card-icon ci-blue">📊</div><div class="card-title">Health Score Trend (All Servers)</div></div>
      <div class="card-body chart-wrap"><canvas id="chart-overview-health"></canvas></div>
    </div>
    <div class="card">
      <div class="card-header"><div class="card-icon ci-amber">⏱</div><div class="card-title">Active Sessions vs Blocked</div></div>
      <div class="card-body chart-wrap"><canvas id="chart-overview-sessions"></canvas></div>
    </div>
  </div>
  <div class="grid-3" style="margin-bottom:16px">
    <div class="card">
      <div class="card-header"><div class="card-icon ci-red">🔥</div><div class="card-title">CPU Utilisation</div></div>
      <div class="card-body chart-sm"><canvas id="chart-cpu-bar"></canvas></div>
    </div>
    <div class="card">
      <div class="card-header"><div class="card-icon ci-purple">🧠</div><div class="card-title">Memory Used %</div></div>
      <div class="card-body chart-sm"><canvas id="chart-mem-bar"></canvas></div>
    </div>
    <div class="card">
      <div class="card-header"><div class="card-icon ci-cyan">⚡</div><div class="card-title">Transactions/sec</div></div>
      <div class="card-body chart-sm"><canvas id="chart-tps-bar"></canvas></div>
    </div>
  </div>
  <div class="grid-2">
    <div class="card">
      <div class="card-header"><div class="card-icon ci-amber">🥧</div><div class="card-title">Server Grade Distribution</div></div>
      <div class="card-body" style="display:flex;align-items:center;gap:24px;justify-content:center">
        <canvas id="chart-grade-pie" style="max-width:180px;max-height:180px"></canvas>
        <div id="grade-legend" style="font-size:.8rem"></div>
      </div>
    </div>
    <div class="card">
      <div class="card-header"><div class="card-icon ci-red">🚨</div><div class="card-title">Recent Alerts</div></div>
      <div class="card-body"><div class="alert-feed" id="overview-alerts"></div></div>
    </div>
  </div>
</div>

<!-- ═══════════════════════════════ SERVERS ═══════════════════════════════ -->
<div class="page" id="page-servers">
  <div class="srv-grid" id="srv-grid"></div>
</div>

<!-- ═══════════════════════════════ PERFORMANCE ═══════════════════════════ -->
<div class="page" id="page-performance">
  <div class="grid-2" style="margin-bottom:16px">
    <div class="card">
      <div class="card-header"><div class="card-icon ci-red">🔥</div><div class="card-title">CPU History (All Servers)</div></div>
      <div class="card-body chart-lg"><canvas id="chart-perf-cpu"></canvas></div>
    </div>
    <div class="card">
      <div class="card-header"><div class="card-icon ci-purple">🧠</div><div class="card-title">Memory Used % History</div></div>
      <div class="card-body chart-lg"><canvas id="chart-perf-mem"></canvas></div>
    </div>
  </div>
  <div class="grid-2" style="margin-bottom:16px">
    <div class="card">
      <div class="card-header"><div class="card-icon ci-cyan">⚡</div><div class="card-title">Batch Requests/sec History</div></div>
      <div class="card-body chart-lg"><canvas id="chart-perf-batch"></canvas></div>
    </div>
    <div class="card">
      <div class="card-header"><div class="card-icon ci-amber">🔒</div><div class="card-title">Active Transactions History</div></div>
      <div class="card-body chart-lg"><canvas id="chart-perf-txn"></canvas></div>
    </div>
  </div>
  <div class="card">
    <div class="card-header"><div class="card-icon ci-blue">📊</div><div class="card-title">Top Wait Types (All Servers)</div></div>
    <div class="card-body"><div id="waits-table-all"></div></div>
  </div>
</div>

<!-- ═══════════════════════════════ QUERIES ═══════════════════════════════ -->
<div class="page" id="page-queries">
  <div class="card" style="margin-bottom:16px">
    <div class="card-header"><div class="card-icon ci-red">🐢</div><div class="card-title">Active Long-Running Queries</div></div>
    <div class="card-body"><div id="active-queries-table"></div></div>
  </div>
  <div class="grid-2">
    <div class="card">
      <div class="card-header"><div class="card-icon ci-amber">📉</div><div class="card-title">Plan Cache — Slowest Queries</div></div>
      <div class="card-body"><div id="slow-queries-table"></div></div>
    </div>
    <div class="card">
      <div class="card-header"><div class="card-icon ci-purple">🗂</div><div class="card-title">Missing Index Recommendations</div></div>
      <div class="card-body"><div id="missing-index-table"></div></div>
    </div>
  </div>
</div>

<!-- ═══════════════════════════════ BACKUPS ═══════════════════════════════ -->
<div class="page" id="page-backups">
  <div class="kpi-bar" id="backup-kpis"></div>
  <div class="card">
    <div class="card-header"><div class="card-icon ci-blue">💾</div><div class="card-title">Backup Status — All Databases</div></div>
    <div class="card-body"><div id="backups-table"></div></div>
  </div>
</div>

<!-- ═══════════════════════════════ SECURITY ══════════════════════════════ -->
<div class="page" id="page-security">
  <div class="kpi-bar" id="security-kpis"></div>
  <div class="grid-2" style="margin-bottom:16px">
    <div class="card">
      <div class="card-header"><div class="card-icon ci-red">⚠️</div><div class="card-title">Configuration Risks</div></div>
      <div class="card-body" id="security-risks"></div>
    </div>
    <div class="card">
      <div class="card-header"><div class="card-icon ci-purple">👑</div><div class="card-title">Privileged Accounts (sysadmin/securityadmin)</div></div>
      <div class="card-body"><div id="priv-users-table"></div></div>
    </div>
  </div>
  <div class="grid-2">
    <div class="card">
      <div class="card-header"><div class="card-icon ci-amber">🔗</div><div class="card-title">Linked Servers</div></div>
      <div class="card-body"><div id="linked-servers-table"></div></div>
    </div>
    <div class="card">
      <div class="card-header"><div class="card-icon ci-cyan">🕵️</div><div class="card-title">Suspicious Connections</div></div>
      <div class="card-body"><div id="suspicious-conns-table"></div></div>
    </div>
  </div>
</div>

<!-- ═══════════════════════════════ CAPACITY ══════════════════════════════ -->
<div class="page" id="page-capacity">
  <div class="card" style="margin-bottom:16px">
    <div class="card-header"><div class="card-icon ci-blue">📈</div><div class="card-title">Resource Headroom by Server</div></div>
    <div class="card-body chart-wrap"><canvas id="chart-capacity-radar"></canvas></div>
  </div>
  <div class="card">
    <div class="card-header"><div class="card-icon ci-amber">💿</div><div class="card-title">Database Disk — Days Until Full (estimated)</div></div>
    <div class="card-body"><div id="capacity-disk-table"></div></div>
  </div>
</div>

<!-- ═══════════════════════════════ INVENTORY ══════════════════════════════ -->
<div class="page" id="page-inventory">
  <div class="card" style="margin-bottom:16px">
    <div class="card-header"><div class="card-icon ci-green">🖥</div><div class="card-title">Server Inventory</div></div>
    <div class="card-body"><div id="inventory-table"></div></div>
  </div>
  <div class="card">
    <div class="card-header"><div class="card-icon ci-blue">🗄</div><div class="card-title">Database Inventory</div></div>
    <div class="card-body"><div id="db-inventory-table"></div></div>
  </div>
</div>

<!-- ═══════════════════════════════ ALERTS ══════════════════════════════ -->
<div class="page" id="page-alerts">
  <div class="grid-2" style="margin-bottom:16px">
    <div class="card">
      <div class="card-header"><div class="card-icon ci-red">🔔</div><div class="card-title">Alert Volume (last 30 polls)</div></div>
      <div class="card-body chart-wrap"><canvas id="chart-alert-history"></canvas></div>
    </div>
    <div class="card">
      <div class="card-header"><div class="card-icon ci-amber">🥧</div><div class="card-title">Alert Type Distribution</div></div>
      <div class="card-body" style="display:flex;align-items:center;gap:24px;justify-content:center">
        <canvas id="chart-alert-pie" style="max-width:180px;max-height:180px"></canvas>
        <div id="alert-type-legend" style="font-size:.8rem"></div>
      </div>
    </div>
  </div>
  <div class="card">
    <div class="card-header"><div class="card-icon ci-red">📋</div><div class="card-title">Full Alert Log</div></div>
    <div class="card-body"><div class="alert-feed" id="full-alert-feed" style="max-height:600px"></div></div>
  </div>
</div>

<script>
/* ═══════════════════ STATE ═══════════════════ */
let allResults = {}, allHistory = {};
let charts = {};
const alertLog = [];
const MAX_ALERTS = 200;

/* ═══════════════════ NAV ═══════════════════ */
function showPage(id, el) {
  document.querySelectorAll('.page').forEach(p => p.classList.remove('active'));
  document.querySelectorAll('.nav-tab').forEach(t => t.classList.remove('active'));
  document.getElementById('page-' + id).classList.add('active');
  el.classList.add('active');
  renderCurrentPage(id);
}
function renderCurrentPage(id) {
  const fns = { overview: renderOverview, servers: renderServers,
    performance: renderPerformance, queries: renderQueries,
    backups: renderBackups, security: renderSecurity,
    capacity: renderCapacity, inventory: renderInventory, alerts: renderAlerts };
  if (fns[id]) fns[id]();
}

/* ═══════════════════ HELPERS ═══════════════════ */
const gc = s => s >= 90 ? '#10b981' : s >= 75 ? '#3b82f6' : s >= 60 ? '#f59e0b' : s >= 40 ? '#f97316' : '#ef4444';
const bc = (v, w, c) => v >= c ? 'b-crit' : v >= w ? 'b-warn' : 'b-ok';
const badge = (t, cls) => '<span class="badge ' + cls + '">' + t + '</span>';
const fmtDate = d => d ? new Date(d).toLocaleString() : '—';
const fmtNum = n => n == null ? '—' : Number(n).toLocaleString(undefined, {maximumFractionDigits: 1});

function mkChart(id, type, data, opts = {}) {
  const el = document.getElementById(id);
  if (!el) return;
  if (charts[id]) { charts[id].destroy(); delete charts[id]; }
  charts[id] = new Chart(el, {
    type, data,
    options: {
      responsive: true, maintainAspectRatio: true, animation: false,
      plugins: { legend: { labels: { color: '#94a3b8', font: { size: 11 }, boxWidth: 12 } } },
      scales: type === 'pie' || type === 'doughnut' ? {} : {
        x: { ticks: { color: '#64748b', font: { size: 10 }, maxTicksLimit: 8 }, grid: { color: '#1e2535' } },
        y: { ticks: { color: '#64748b', font: { size: 10 } }, grid: { color: '#1e2535' } }
      },
      ...opts
    }
  });
}

function serverList() { return Object.values(allResults); }
const COLORS = ['#3b82f6','#10b981','#f59e0b','#ef4444','#8b5cf6','#06b6d4','#f97316','#ec4899','#84cc16','#14b8a6'];

/* ═══════════════════ ALERT LOG ═══════════════════ */
function collectAlerts(results) {
  Object.values(results).forEach(r => {
    const srv = r.ServerName;
    const ts = new Date(r.Timestamp).toLocaleTimeString();
    if (r.Health && (r.Health.Grade === 'D' || r.Health.Grade === 'F'))
      pushAlert('crit', srv, 'Health grade ' + r.Health.Grade + ' (score ' + r.Health.Score + ')', ts);
    if (r.Connections && r.Connections.BlockedSessions > 0)
      pushAlert('warn', srv, r.Connections.BlockedSessions + ' blocked sessions', ts);
    if (r.Queries && r.Queries.ActiveLongRunning && r.Queries.ActiveLongRunning.length > 0)
      pushAlert('warn', srv, r.Queries.ActiveLongRunning.length + ' long-running queries', ts);
    if (r.Resources) {
      if (r.Resources.SQLCPUPercent > 80) pushAlert('crit', srv, 'CPU ' + r.Resources.SQLCPUPercent + '%', ts);
      if (r.Resources.TotalMemoryMB > 0) {
        const avail = r.Resources.AvailableMemoryMB / r.Resources.TotalMemoryMB * 100;
        if (avail < 10) pushAlert('crit', srv, 'Memory critically low (' + avail.toFixed(1) + '% free)', ts);
      }
    }
    if (r.Deadlocks && r.Deadlocks.length > 0)
      pushAlert('crit', srv, r.Deadlocks.length + ' deadlock(s) detected', ts);
    if (r.Jobs && r.Jobs.FailedJobs && r.Jobs.FailedJobs.length > 0)
      pushAlert('warn', srv, r.Jobs.FailedJobs.length + ' SQL Agent job failures', ts);
    if (r.Backups && r.Backups.Databases)
      r.Backups.Databases.filter(d => d.IsAlertFull).forEach(d =>
        pushAlert('warn', srv, 'Backup overdue: ' + d.DatabaseName, ts));
  });
}
function pushAlert(sev, server, msg, ts) {
  alertLog.unshift({ sev, server, msg, ts });
  if (alertLog.length > MAX_ALERTS) alertLog.pop();
}
function alertItemHTML(a) {
  const icons = { crit: '🔴', warn: '🟡', info: '🔵' };
  return '<div class="alert-item ' + a.sev + '">' +
    '<div class="alert-icon">' + (icons[a.sev]||'⚪') + '</div>' +
    '<div class="alert-body"><div class="alert-server">' + a.server + '</div>' +
    '<div class="alert-msg">' + a.msg + '</div>' +
    '<div class="alert-time">' + a.ts + '</div></div></div>';
}

/* ═══════════════════ TOPBAR ═══════════════════ */
function renderTopbar() {
  const srvs = serverList();
  const online = srvs.filter(s => s.Health).length;
  const degraded = srvs.filter(s => s.Health && (s.Health.Grade === 'D' || s.Health.Grade === 'F')).length;
  const sessions = srvs.reduce((a, s) => a + (s.Connections ? s.Connections.TotalSessions : 0), 0);
  const avgScore = online ? Math.round(srvs.filter(s => s.Health).reduce((a, s) => a + s.Health.Score, 0) / online) : 0;
  const deadlocks = srvs.reduce((a, s) => a + (s.Deadlocks ? s.Deadlocks.length : 0), 0);

  document.getElementById('top-stats').innerHTML =
    topStat(online, 'Servers', online === srvs.length ? '#10b981' : '#f59e0b') +
    topStat(degraded, 'Degraded', degraded > 0 ? '#ef4444' : '#10b981') +
    topStat(avgScore, 'Avg Score', gc(avgScore)) +
    topStat(sessions, 'Sessions', '#94a3b8') +
    topStat(deadlocks, 'Deadlocks', deadlocks > 0 ? '#ef4444' : '#10b981');
}
function topStat(v, l, c) {
  return '<div class="top-stat"><div class="tv" style="color:' + c + '">' + v + '</div><div class="tl">' + l + '</div></div>';
}

/* ═══════════════════ OVERVIEW ═══════════════════ */
function renderOverview() {
  const srvs = serverList();
  // KPIs
  const sessions = srvs.reduce((a, s) => a + (s.Connections ? s.Connections.TotalSessions : 0), 0);
  const blocked  = srvs.reduce((a, s) => a + (s.Connections ? s.Connections.BlockedSessions : 0), 0);
  const slowQ    = srvs.reduce((a, s) => a + (s.Queries && s.Queries.ActiveLongRunning ? s.Queries.ActiveLongRunning.length : 0), 0);
  const deadlocks= srvs.reduce((a, s) => a + (s.Deadlocks ? s.Deadlocks.length : 0), 0);
  const avgCPU   = srvs.length ? Math.round(srvs.reduce((a, s) => a + (s.Resources ? Math.max(0, s.Resources.SQLCPUPercent) : 0), 0) / srvs.length) : 0;
  const avgScore = srvs.filter(s => s.Health).length
    ? Math.round(srvs.filter(s => s.Health).reduce((a, s) => a + s.Health.Score, 0) / srvs.filter(s => s.Health).length) : 0;

  document.getElementById('kpi-bar').innerHTML =
    kpiCard(srvs.length, 'Servers', 'blue', 'Monitored') +
    kpiCard(avgScore + '/100', 'Avg Health', avgScore >= 75 ? 'green' : avgScore >= 50 ? 'amber' : 'red', 'Score') +
    kpiCard(sessions, 'Sessions', 'blue', 'Active') +
    kpiCard(blocked, 'Blocked', blocked > 0 ? 'red' : 'green', 'Sessions') +
    kpiCard(slowQ, 'Slow Queries', slowQ > 0 ? 'amber' : 'green', '> threshold') +
    kpiCard(avgCPU + '%', 'Avg CPU', avgCPU > 80 ? 'red' : avgCPU > 60 ? 'amber' : 'green', 'SQL Server') +
    kpiCard(deadlocks, 'Deadlocks', deadlocks > 0 ? 'red' : 'green', 'This cycle') +
    kpiCard(alertLog.length, 'Total Alerts', alertLog.length > 20 ? 'amber' : 'green', 'Logged');

  // Health trend chart
  const servers = Object.keys(allHistory);
  if (servers.length) {
    const labels = (allHistory[servers[0]] || []).slice(-20).map(h => new Date(h.ts).toLocaleTimeString());
    const datasets = servers.map((s, i) => ({
      label: s,
      data: (allHistory[s] || []).slice(-20).map(h => h.health),
      borderColor: COLORS[i % COLORS.length],
      backgroundColor: COLORS[i % COLORS.length] + '20',
      fill: false, tension: .3, pointRadius: 2
    }));
    mkChart('chart-overview-health', 'line', { labels, datasets },
      { scales: { y: { min: 0, max: 100, ticks: { color: '#64748b' }, grid: { color: '#1e2535' } }, x: { ticks: { color: '#64748b' }, grid: { display: false } } } });
  }

  // Sessions chart
  if (servers.length) {
    const labels = (allHistory[servers[0]] || []).slice(-20).map(h => new Date(h.ts).toLocaleTimeString());
    const sesDatasets = servers.flatMap((s, i) => [
      { label: s + ' Sessions', data: (allHistory[s] || []).slice(-20).map(h => h.sess), borderColor: COLORS[i % COLORS.length], backgroundColor: COLORS[i % COLORS.length] + '20', fill: false, tension: .3, pointRadius: 2 },
      { label: s + ' Blocked', data: (allHistory[s] || []).slice(-20).map(h => h.blocked), borderColor: '#ef4444', backgroundColor: '#ef444420', fill: false, tension: .3, pointRadius: 2, borderDash: [4, 4] }
    ]);
    mkChart('chart-overview-sessions', 'line', { labels, datasets: sesDatasets });
  }

  // CPU bar
  const cpuData = srvs.map(s => s.Resources ? Math.max(0, s.Resources.SQLCPUPercent) : 0);
  mkChart('chart-cpu-bar', 'bar', {
    labels: srvs.map(s => s.ServerName),
    datasets: [{ label: 'SQL CPU %', data: cpuData,
      backgroundColor: cpuData.map(v => v > 80 ? '#ef4444cc' : v > 60 ? '#f59e0bcc' : '#3b82f6cc'),
      borderRadius: 6 }]
  }, { scales: { y: { min: 0, max: 100, grid: { color: '#1e2535' }, ticks: { color: '#64748b' } }, x: { ticks: { color: '#64748b' }, grid: { display: false } } } });

  // Memory bar
  const memData = srvs.map(s => s.Resources && s.Resources.TotalMemoryMB > 0
    ? Math.round((1 - s.Resources.AvailableMemoryMB / s.Resources.TotalMemoryMB) * 100) : 0);
  mkChart('chart-mem-bar', 'bar', {
    labels: srvs.map(s => s.ServerName),
    datasets: [{ label: 'Memory Used %', data: memData,
      backgroundColor: memData.map(v => v > 90 ? '#ef4444cc' : v > 80 ? '#f59e0bcc' : '#8b5cf6cc'),
      borderRadius: 6 }]
  }, { scales: { y: { min: 0, max: 100, grid: { color: '#1e2535' }, ticks: { color: '#64748b' } }, x: { ticks: { color: '#64748b' }, grid: { display: false } } } });

  // TPS bar
  const tpsData = srvs.map(s => s.Transactions ? s.Transactions.TransactionsPerSec : 0);
  mkChart('chart-tps-bar', 'bar', {
    labels: srvs.map(s => s.ServerName),
    datasets: [{ label: 'TPS', data: tpsData, backgroundColor: '#06b6d4cc', borderRadius: 6 }]
  }, { scales: { y: { grid: { color: '#1e2535' }, ticks: { color: '#64748b' } }, x: { ticks: { color: '#64748b' }, grid: { display: false } } } });

  // Grade pie
  const grades = { A:0, B:0, C:0, D:0, F:0 };
  srvs.forEach(s => { if (s.Health) grades[s.Health.Grade] = (grades[s.Health.Grade] || 0) + 1; });
  const gradeColors = { A:'#10b981', B:'#3b82f6', C:'#f59e0b', D:'#f97316', F:'#ef4444' };
  const gradeEntries = Object.entries(grades).filter(([, v]) => v > 0);
  mkChart('chart-grade-pie', 'doughnut', {
    labels: gradeEntries.map(([k]) => 'Grade ' + k),
    datasets: [{ data: gradeEntries.map(([, v]) => v), backgroundColor: gradeEntries.map(([k]) => gradeColors[k]), borderWidth: 0, hoverOffset: 6 }]
  }, { plugins: { legend: { display: false } } });

  document.getElementById('grade-legend').innerHTML = gradeEntries.map(([k, v]) =>
    '<div style="display:flex;align-items:center;gap:8px;margin-bottom:8px">' +
    '<div style="width:12px;height:12px;border-radius:3px;background:' + gradeColors[k] + '"></div>' +
    '<span style="color:' + gradeColors[k] + ';font-weight:700">Grade ' + k + '</span>' +
    '<span style="color:#64748b"> — ' + v + ' server' + (v > 1 ? 's' : '') + '</span></div>'
  ).join('');

  // Alerts feed
  document.getElementById('overview-alerts').innerHTML =
    alertLog.slice(0, 8).map(alertItemHTML).join('') || '<div class="no-data">No alerts recorded yet</div>';
}

function kpiCard(val, label, color, sub) {
  return '<div class="kpi ' + color + '"><div class="kpi-label">' + label + '</div>' +
    '<div class="kpi-val">' + val + '</div><div class="kpi-sub">' + sub + '</div></div>';
}

/* ═══════════════════ SERVERS ═══════════════════ */
function renderServers() {
  const grid = document.getElementById('srv-grid');
  grid.innerHTML = '';
  Object.entries(allResults).forEach(([name, r]) => {
    const h = r.Health || { Grade: '?', Score: 0, Penalties: [] };
    const conn = r.Connections || {};
    const res = r.Resources || {};
    const q = r.Queries || {};
    const txn = r.Transactions || {};
    const dl = r.Deadlocks || [];
    const color = gc(h.Score);
    const memPct = res.TotalMemoryMB > 0
      ? Math.round((1 - res.AvailableMemoryMB / res.TotalMemoryMB) * 100) : 0;
    const cid = 'srv-chart-' + name.replace(/\W/g, '_');

    const card = document.createElement('div');
    card.className = 'srv-card';
    card.innerHTML =
      '<div class="srv-card-head">' +
      '<div class="grade-ring" style="border-color:' + color + ';color:' + color + '">' + h.Grade + '</div>' +
      '<div class="srv-meta"><div class="srv-name">' + name + '</div>' +
      '<div class="srv-detail">' + (r.Inventory ? r.Inventory.Edition || '' : '') + (r.Inventory ? ' • v' + (r.Inventory.ProductVersion || '') : '') + ' • ' + new Date(r.Timestamp).toLocaleTimeString() + '</div></div>' +
      '<div class="score-wrap"><div class="score-big" style="color:' + color + '">' + h.Score + '</div>' +
      '<div class="score-track"><div class="score-fill" style="width:' + h.Score + '%;background:' + color + '"></div></div></div>' +
      '</div>' +
      '<div class="mini-stats">' +
      miniStat(res.SQLCPUPercent >= 0 ? res.SQLCPUPercent + '%' : 'N/A', 'SQL CPU', res.SQLCPUPercent > 80 ? '#ef4444' : res.SQLCPUPercent > 60 ? '#f59e0b' : '#10b981') +
      miniStat(memPct + '%', 'Mem Used', memPct > 90 ? '#ef4444' : memPct > 80 ? '#f59e0b' : '#10b981') +
      miniStat(conn.TotalSessions || 0, 'Sessions', conn.TotalSessions > 150 ? '#f59e0b' : '#94a3b8') +
      miniStat(conn.BlockedSessions || 0, 'Blocked', conn.BlockedSessions > 0 ? '#ef4444' : '#10b981') +
      miniStat(q.ActiveLongRunning ? q.ActiveLongRunning.length : 0, 'Slow Q', (q.ActiveLongRunning && q.ActiveLongRunning.length > 0) ? '#f59e0b' : '#10b981') +
      miniStat(dl.length, 'Deadlocks', dl.length > 0 ? '#ef4444' : '#10b981') +
      '</div>' +
      '<div class="srv-tabs">' +
      ['Health', 'Waits', 'Disk I/O', 'Transactions', 'Trend'].map((t, i) =>
        '<div class="stab' + (i === 0 ? ' active' : '') + '" onclick="switchStab(this,\'' + (cid + '-t' + i) + '\')">' + t + '</div>'
      ).join('') + '</div>' +
      stabPane(cid + '-t0', true, penaltiesHTML(h)) +
      stabPane(cid + '-t1', false, waitsHTML(r)) +
      stabPane(cid + '-t2', false, diskIOHTML(r)) +
      stabPane(cid + '-t3', false, transactionHTML(r, txn)) +
      stabPane(cid + '-t4', false, '<canvas id="' + cid + '"></canvas>');

    grid.appendChild(card);

    // Draw trend chart
    const hist = allHistory[name] || [];
    if (hist.length > 1) {
      setTimeout(() => {
        const el = document.getElementById(cid);
        if (!el) return;
        const pts = hist.slice(-20);
        mkChart(cid, 'line', {
          labels: pts.map(p => new Date(p.ts).toLocaleTimeString()),
          datasets: [
            { label: 'Health', data: pts.map(p => p.health), borderColor: color, backgroundColor: color + '20', fill: true, tension: .3, pointRadius: 2, yAxisID: 'y' },
            { label: 'CPU%', data: pts.map(p => p.cpu), borderColor: '#ef4444', fill: false, tension: .3, pointRadius: 2, borderDash: [4, 3], yAxisID: 'y' }
          ]
        }, {
          scales: {
            y: { min: 0, max: 100, ticks: { color: '#64748b' }, grid: { color: '#1e2535' } },
            x: { ticks: { color: '#64748b', font: { size: 9 } }, grid: { display: false } }
          }
        });
      }, 50);
    }
  });
}

function miniStat(v, l, c) {
  return '<div class="ms"><div class="ms-v" style="color:' + c + '">' + v + '</div><div class="ms-l">' + l + '</div></div>';
}
function stabPane(id, active, content) {
  return '<div id="' + id + '" class="stab-content' + (active ? ' active' : '') + '">' + content + '</div>';
}
function switchStab(el, id) {
  const card = el.closest('.srv-card');
  card.querySelectorAll('.stab').forEach(t => t.classList.remove('active'));
  card.querySelectorAll('.stab-content').forEach(t => t.classList.remove('active'));
  el.classList.add('active');
  const t = document.getElementById(id); if (t) t.classList.add('active');
}
function penaltiesHTML(h) {
  if (!h.Penalties || h.Penalties.length === 0)
    return '<div class="pen-item ok">✓ All health checks passing</div>';
  return '<div class="pen-list">' + h.Penalties.map(p =>
    '<div class="pen-item">⚠ ' + p + '</div>').join('') + '</div>';
}
function waitsHTML(r) {
  if (!r.Waits || !r.Waits.TopWaits || !r.Waits.TopWaits.length)
    return '<div class="no-data">No wait data</div>';
  return '<table class="dt"><thead><tr><th>Wait Type</th><th>Category</th><th>Avg ms</th><th>% Total</th></tr></thead><tbody>' +
    r.Waits.TopWaits.slice(0, 6).map(w =>
      '<tr><td>' + w.WaitType + '</td><td><span class="tag">' + w.Category + '</span></td>' +
      '<td>' + w.AvgWaitMS.toFixed(1) + '</td>' +
      '<td>' + badge(w.PctOfTotal.toFixed(1) + '%', w.PctOfTotal > 30 ? 'b-crit' : w.PctOfTotal > 15 ? 'b-warn' : 'b-ok') + '</td></tr>'
    ).join('') + '</tbody></table>';
}
function diskIOHTML(r) {
  if (!r.Resources || !r.Resources.DiskStats || !r.Resources.DiskStats.length)
    return '<div class="no-data">No disk I/O data</div>';
  return '<table class="dt"><thead><tr><th>Database</th><th>Type</th><th>Avg Read ms</th><th>Avg Write ms</th></tr></thead><tbody>' +
    r.Resources.DiskStats.slice(0, 6).map(d =>
      '<tr><td>' + d.Database + '</td><td>' + d.FileType + '</td>' +
      '<td>' + badge(d.AvgReadMS.toFixed(1), d.AvgReadMS > 50 ? 'b-crit' : d.AvgReadMS > 20 ? 'b-warn' : 'b-ok') + '</td>' +
      '<td>' + badge(d.AvgWriteMS.toFixed(1), d.AvgWriteMS > 50 ? 'b-crit' : d.AvgWriteMS > 20 ? 'b-warn' : 'b-ok') + '</td></tr>'
    ).join('') + '</tbody></table>';
}
function transactionHTML(r, txn) {
  return '<table class="dt"><tbody>' +
    '<tr><td>Transactions/sec</td><td>' + badge(fmtNum(txn.TransactionsPerSec), 'b-blue') + '</td></tr>' +
    '<tr><td>Batch Requests/sec</td><td>' + badge(fmtNum(txn.BatchRequestsPerSec), 'b-blue') + '</td></tr>' +
    '<tr><td>Active Transactions</td><td>' + badge(txn.ActiveTransactions || 0, txn.ActiveTransactions > 50 ? 'b-warn' : 'b-ok') + '</td></tr>' +
    '<tr><td>Longest Transaction</td><td>' + badge((txn.LongestTxnSec || 0) + 's', txn.LongestTxnSec > 30 ? 'b-crit' : 'b-ok') + '</td></tr>' +
    '<tr><td>TempDB Used MB</td><td>' + badge(fmtNum(txn.TempDBUsedMB), 'b-purple') + '</td></tr>' +
    '</tbody></table>';
}

/* ═══════════════════ PERFORMANCE ═══════════════════ */
function renderPerformance() {
  const servers = Object.keys(allHistory);
  if (!servers.length) return;
  const colors = COLORS;

  ['perf-cpu', 'perf-mem', 'perf-batch', 'perf-txn'].forEach((id, idx) => {
    const fields = ['cpu', 'mem', 'batch_req', 'txns'];
    const labels_arr = ['SQL CPU %', 'Memory Used %', 'Batch Req/sec', 'Active Txns'];
    const labels = (allHistory[servers[0]] || []).slice(-30).map(h => new Date(h.ts).toLocaleTimeString());
    const datasets = servers.map((s, i) => ({
      label: s,
      data: (allHistory[s] || []).slice(-30).map(h => h[fields[idx]] || 0),
      borderColor: colors[i % colors.length],
      backgroundColor: colors[i % colors.length] + '20',
      fill: false, tension: .3, pointRadius: 2
    }));
    mkChart('chart-' + id, 'line', { labels, datasets },
      { scales: { y: { grid: { color: '#1e2535' }, ticks: { color: '#64748b' } }, x: { ticks: { color: '#64748b' }, grid: { display: false } } } });
  });

  // Waits table
  let html = '';
  serverList().forEach(r => {
    if (!r.Waits || !r.Waits.TopWaits || !r.Waits.TopWaits.length) return;
    html += '<div class="section-title" style="margin-top:12px">' + r.ServerName + '</div>' +
      '<table class="dt"><thead><tr><th>Wait Type</th><th>Category</th><th>Total (s)</th><th>Avg ms</th><th>% of Total</th></tr></thead><tbody>' +
      r.Waits.TopWaits.slice(0, 8).map(w =>
        '<tr><td><strong>' + w.WaitType + '</strong></td><td><span class="tag">' + w.Category + '</span></td>' +
        '<td>' + w.WaitTimeSec.toFixed(0) + '</td><td>' + w.AvgWaitMS.toFixed(1) + '</td>' +
        '<td><div style="display:flex;align-items:center;gap:8px"><div class="prog-bar" style="width:80px"><div class="prog-fill" style="width:' + Math.min(100, w.PctOfTotal) + '%;background:' + (w.PctOfTotal > 30 ? '#ef4444' : w.PctOfTotal > 15 ? '#f59e0b' : '#3b82f6') + '"></div></div>' + w.PctOfTotal.toFixed(1) + '%</div></td></tr>'
      ).join('') + '</tbody></table>';
  });
  document.getElementById('waits-table-all').innerHTML = html || '<div class="no-data">No wait data available</div>';
}

/* ═══════════════════ QUERIES ═══════════════════ */
function renderQueries() {
  let activeHTML = '';
  serverList().forEach(r => {
    if (!r.Queries || !r.Queries.ActiveLongRunning || !r.Queries.ActiveLongRunning.length) return;
    activeHTML += '<div class="section-title" style="margin-top:10px">' + r.ServerName + '</div>' +
      '<table class="dt"><thead><tr><th>Session</th><th>DB</th><th>Login</th><th>Elapsed ms</th><th>CPU ms</th><th>Wait</th><th>Command</th></tr></thead><tbody>' +
      r.Queries.ActiveLongRunning.map(q =>
        '<tr><td>' + q.SessionID + '</td><td>' + q.Database + '</td><td>' + q.LoginName + '</td>' +
        '<td>' + badge(fmtNum(q.ElapsedMS), q.ElapsedMS > 30000 ? 'b-crit' : 'b-warn') + '</td>' +
        '<td>' + fmtNum(q.CPUTime) + '</td><td><span class="tag">' + (q.WaitType || '—') + '</span></td>' +
        '<td>' + q.Command + '</td></tr>'
      ).join('') + '</tbody></table>';
  });
  document.getElementById('active-queries-table').innerHTML = activeHTML || '<div class="no-data" style="padding:16px">✓ No long-running queries detected</div>';

  let slowHTML = '';
  serverList().forEach(r => {
    if (!r.Queries || !r.Queries.SlowQueries || !r.Queries.SlowQueries.length) return;
    slowHTML += '<div class="section-title" style="margin-top:10px">' + r.ServerName + '</div>' +
      '<table class="dt"><thead><tr><th>DB</th><th>Avg ms</th><th>Exec Count</th><th>Avg Reads</th></tr></thead><tbody>' +
      r.Queries.SlowQueries.slice(0, 8).map(q =>
        '<tr><td>' + q.Database + '</td>' +
        '<td>' + badge(fmtNum(q.AvgElapsedMS), q.AvgElapsedMS > 10000 ? 'b-crit' : 'b-warn') + '</td>' +
        '<td>' + fmtNum(q.ExecutionCount) + '</td><td>' + fmtNum(q.AvgLogicalReads) + '</td></tr>'
      ).join('') + '</tbody></table>';
  });
  document.getElementById('slow-queries-table').innerHTML = slowHTML || '<div class="no-data">No slow queries in plan cache</div>';

  let idxHTML = '';
  serverList().forEach(r => {
    if (!r.Indexes || !r.Indexes.MissingIndexes || !r.Indexes.MissingIndexes.length) return;
    idxHTML += '<div class="section-title" style="margin-top:10px">' + r.ServerName + '</div>' +
      '<table class="dt"><thead><tr><th>Table</th><th>DB</th><th>Impact</th><th>Seeks</th></tr></thead><tbody>' +
      r.Indexes.MissingIndexes.slice(0, 8).map(ix =>
        '<tr><td><strong>' + ix.TableName + '</strong></td><td>' + ix.Database + '</td>' +
        '<td>' + badge(Math.round(ix.ImpactScore).toLocaleString(), ix.ImpactScore > 100000 ? 'b-crit' : ix.ImpactScore > 10000 ? 'b-warn' : 'b-blue') + '</td>' +
        '<td>' + fmtNum(ix.UserSeeks) + '</td></tr>'
      ).join('') + '</tbody></table>';
  });
  document.getElementById('missing-index-table').innerHTML = idxHTML || '<div class="no-data">No missing indexes detected</div>';
}

/* ═══════════════════ BACKUPS ═══════════════════ */
function renderBackups() {
  let overdueCount = 0, okCount = 0, totalDBs = 0;
  serverList().forEach(r => {
    if (!r.Backups || !r.Backups.Databases) return;
    r.Backups.Databases.forEach(d => {
      totalDBs++;
      if (d.IsAlertFull || d.IsAlertLog) overdueCount++; else okCount++;
    });
  });
  document.getElementById('backup-kpis').innerHTML =
    kpiCard(totalDBs, 'Total DBs', 'blue', 'Monitored') +
    kpiCard(okCount, 'Backups OK', 'green', 'Within threshold') +
    kpiCard(overdueCount, 'Overdue', overdueCount > 0 ? 'red' : 'green', 'Need attention');

  let html = '<table class="dt"><thead><tr><th>Server</th><th>Database</th><th>Recovery</th><th>Last Full</th><th>Last Log</th><th>Size MB</th><th>Status</th></tr></thead><tbody>';
  serverList().forEach(r => {
    if (!r.Backups || !r.Backups.Databases) return;
    r.Backups.Databases.forEach(d => {
      html += '<tr><td>' + r.ServerName + '</td><td><strong>' + d.DatabaseName + '</strong></td>' +
        '<td><span class="tag">' + d.RecoveryModel + '</span></td>' +
        '<td>' + (d.LastFullBackup ? fmtDate(d.LastFullBackup) : badge('Never', 'b-crit')) + '</td>' +
        '<td>' + (d.LastLogBackup ? fmtDate(d.LastLogBackup) : '—') + '</td>' +
        '<td>' + fmtNum(d.SizeMB) + '</td>' +
        '<td>' + (d.IsAlertFull ? badge('OVERDUE', 'b-crit') : d.IsAlertLog ? badge('LOG DUE', 'b-warn') : badge('OK', 'b-ok')) + '</td></tr>';
    });
  });
  document.getElementById('backups-table').innerHTML = html + '</tbody></table>';
}

/* ═══════════════════ SECURITY ═══════════════════ */
function renderSecurity() {
  let totalPriv = 0, riskCount = 0;
  serverList().forEach(r => {
    if (!r.Security) return;
    if (r.Security.XPCmdShellOn || r.Security.SALoginEnabled) riskCount++;
    totalPriv += (r.Security.PrivilegedUsers || []).length;
  });
  document.getElementById('security-kpis').innerHTML =
    kpiCard(riskCount, 'Config Risks', riskCount > 0 ? 'red' : 'green', 'Servers affected') +
    kpiCard(totalPriv, 'Privileged Users', totalPriv > 5 ? 'amber' : 'green', 'sysadmin/secadmin') +
    kpiCard(serverList().reduce((a, r) => a + (r.Security && r.Security.LinkedServers ? r.Security.LinkedServers.length : 0), 0), 'Linked Servers', 'blue', 'Potential risk');

  // Risks
  let risksHTML = '';
  serverList().forEach(r => {
    if (!r.Security) return;
    const checks = [
      { label: 'xp_cmdshell enabled', ok: !r.Security.XPCmdShellOn, text: r.Security.XPCmdShellOn ? '🔴 ENABLED — allows OS command execution' : '✅ Disabled' },
      { label: 'SA login', ok: !r.Security.SALoginEnabled, text: r.Security.SALoginEnabled ? '🟡 Enabled — recommend disabling' : '✅ Disabled' },
      { label: 'Auth mode', ok: !r.Security.MixedAuthMode, text: r.Security.MixedAuthMode ? '🟡 Mixed mode (SQL + Windows)' : '✅ Windows-only' },
    ];
    risksHTML += '<div class="section-title" style="margin-top:8px">' + r.ServerName + '</div>';
    checks.forEach(c => {
      risksHTML += '<div class="risk-row"><div class="risk-indicator" style="background:' + (c.ok ? '#10b981' : '#ef4444') + '"></div><strong>' + c.label + '</strong><span style="margin-left:8px;color:#94a3b8">' + c.text + '</span></div>';
    });
  });
  document.getElementById('security-risks').innerHTML = risksHTML || '<div class="no-data">No data</div>';

  // Privileged users
  let privHTML = '<table class="dt"><thead><tr><th>Server</th><th>Login</th><th>Role</th><th>Status</th></tr></thead><tbody>';
  serverList().forEach(r => {
    if (!r.Security || !r.Security.PrivilegedUsers) return;
    r.Security.PrivilegedUsers.forEach(u => {
      privHTML += '<tr><td>' + r.ServerName + '</td><td><strong>' + u.LoginName + '</strong></td>' +
        '<td>' + badge(u.Role, u.Role === 'sysadmin' ? 'b-crit' : 'b-warn') + '</td>' +
        '<td>' + (u.IsDisabled ? badge('Disabled', 'b-crit') : badge('Active', 'b-ok')) + '</td></tr>';
    });
  });
  document.getElementById('priv-users-table').innerHTML = privHTML + '</tbody></table>';

  // Linked servers
  let lsHTML = '<table class="dt"><thead><tr><th>Server</th><th>Linked Server</th><th>Provider</th><th>Data Source</th><th>Risk</th></tr></thead><tbody>';
  let hasLS = false;
  serverList().forEach(r => {
    if (!r.Security || !r.Security.LinkedServers || !r.Security.LinkedServers.length) return;
    hasLS = true;
    r.Security.LinkedServers.forEach(ls => {
      lsHTML += '<tr><td>' + r.ServerName + '</td><td><strong>' + ls.Name + '</strong></td>' +
        '<td>' + ls.Provider + '</td><td>' + ls.DataSource + '</td>' +
        '<td>' + (ls.IsRemoteLogin ? badge('Remote Login', 'b-warn') : badge('Self Credential', 'b-ok')) + '</td></tr>';
    });
  });
  document.getElementById('linked-servers-table').innerHTML = hasLS ? (lsHTML + '</tbody></table>') : '<div class="no-data">No linked servers configured</div>';

  // Suspicious connections
  let scHTML = '<table class="dt"><thead><tr><th>Server</th><th>Session</th><th>Login</th><th>Host</th><th>Program</th></tr></thead><tbody>';
  let hasSC = false;
  serverList().forEach(r => {
    if (!r.Security || !r.Security.SuspiciousConns || !r.Security.SuspiciousConns.length) return;
    hasSC = true;
    r.Security.SuspiciousConns.forEach(sc => {
      scHTML += '<tr><td>' + r.ServerName + '</td><td>' + sc.SessionID + '</td>' +
        '<td>' + sc.LoginName + '</td><td>' + sc.HostName + '</td>' +
        '<td>' + badge(sc.ProgramName || 'Unknown', 'b-warn') + '</td></tr>';
    });
  });
  document.getElementById('suspicious-conns-table').innerHTML = hasSC ? (scHTML + '</tbody></table>') : '<div class="no-data">No suspicious connections detected</div>';
}

/* ═══════════════════ CAPACITY ═══════════════════ */
function renderCapacity() {
  const srvs = serverList().filter(s => s.Capacity);
  if (!srvs.length) { return; }

  // Radar chart — CPU / Memory headroom
  const labels = ['CPU Headroom', 'Memory Headroom', 'Disk Headroom', 'Conn Headroom', 'Health'];
  const datasets = srvs.map((r, i) => {
    const c = r.Capacity;
    const memAvail = r.Resources && r.Resources.TotalMemoryMB > 0
      ? r.Resources.AvailableMemoryMB / r.Resources.TotalMemoryMB * 100 : 50;
    const diskAvg = c.DiskTrend && c.DiskTrend.length
      ? Math.min(100, c.DiskTrend.reduce((a, d) => a + d.UtilisationPct, 0) / c.DiskTrend.length)
      : 50;
    const connH = r.Connections
      ? Math.max(0, 100 - (r.Connections.TotalSessions / 200 * 100)) : 80;
    return {
      label: r.ServerName,
      data: [
        Math.max(0, 100 - (r.Resources ? Math.max(0, r.Resources.SQLCPUPercent) : 0)),
        memAvail,
        Math.max(0, 100 - diskAvg),
        connH,
        r.Health ? r.Health.Score : 0
      ],
      borderColor: COLORS[i % COLORS.length],
      backgroundColor: COLORS[i % COLORS.length] + '30',
      pointBackgroundColor: COLORS[i % COLORS.length]
    };
  });
  mkChart('chart-capacity-radar', 'radar', { labels, datasets }, {
    scales: { r: { min: 0, max: 100, ticks: { color: '#64748b', backdropColor: 'transparent' }, grid: { color: '#1e2535' }, pointLabels: { color: '#94a3b8' } } },
    plugins: { legend: { labels: { color: '#94a3b8' } } }
  });

  // Disk table
  let html = '<table class="dt"><thead><tr><th>Server</th><th>Database</th><th>Current MB</th><th>Max MB</th><th>Used %</th><th>Days Until Full*</th></tr></thead><tbody>';
  srvs.forEach(r => {
    if (!r.Capacity || !r.Capacity.DiskTrend) return;
    r.Capacity.DiskTrend.forEach(d => {
      const duf = d.DaysUntilFull >= 9999 ? '∞' : d.DaysUntilFull + 'd';
      html += '<tr><td>' + r.ServerName + '</td><td><strong>' + d.DatabaseName + '</strong></td>' +
        '<td>' + fmtNum(d.CurrentSizeMB) + '</td><td>' + fmtNum(d.MaxSizeMB) + '</td>' +
        '<td><div style="display:flex;align-items:center;gap:8px"><div class="prog-bar" style="width:60px"><div class="prog-fill" style="width:' + Math.min(100, d.UtilisationPct) + '%;background:' + (d.UtilisationPct > 85 ? '#ef4444' : d.UtilisationPct > 70 ? '#f59e0b' : '#10b981') + '"></div></div>' + d.UtilisationPct.toFixed(0) + '%</div></td>' +
        '<td>' + badge(duf, d.DaysUntilFull < 30 ? 'b-crit' : d.DaysUntilFull < 90 ? 'b-warn' : 'b-ok') + '</td></tr>';
    });
  });
  html += '</tbody></table><div style="font-size:.72rem;color:#64748b;margin-top:8px">* Based on estimated 3%/week growth rate. Actual growth may vary.</div>';
  document.getElementById('capacity-disk-table').innerHTML = html;
}

/* ═══════════════════ INVENTORY ═══════════════════ */
function renderInventory() {
  let srvHTML = '<table class="dt"><thead><tr><th>Server</th><th>Version</th><th>Edition</th><th>CPUs</th><th>RAM</th><th>Uptime</th><th>DBs</th><th>Features</th></tr></thead><tbody>';
  serverList().forEach(r => {
    const inv = r.Inventory;
    if (!inv) { srvHTML += '<tr><td>' + r.ServerName + '</td><td colspan="7" style="color:#64748b">Inventory not available</td></tr>'; return; }
    const features = [inv.IsHADREnabled ? badge('AG', 'b-purple') : '', inv.IsClustered ? badge('Cluster', 'b-blue') : ''].filter(Boolean).join(' ') || badge('Standalone', 'b-ok');
    srvHTML += '<tr><td><strong>' + r.ServerName + '</strong></td>' +
      '<td>' + inv.ProductVersion + '</td><td>' + (inv.Edition || '—') + '</td>' +
      '<td>' + (inv.ProcessorCount || '—') + '</td>' +
      '<td>' + (inv.TotalMemoryMB ? Math.round(inv.TotalMemoryMB / 1024) + ' GB' : '—') + '</td>' +
      '<td>' + (inv.UptimeDays ? inv.UptimeDays.toFixed(1) + 'd' : '—') + '</td>' +
      '<td>' + inv.DatabaseCount + '</td><td>' + features + '</td></tr>';
  });
  document.getElementById('inventory-table').innerHTML = srvHTML + '</tbody></table>';

  let dbHTML = '<table class="dt"><thead><tr><th>Server</th><th>Database</th><th>State</th><th>Recovery</th><th>Compat</th><th>Size MB</th><th>Read Only</th></tr></thead><tbody>';
  serverList().forEach(r => {
    if (!r.Inventory || !r.Inventory.Databases) return;
    r.Inventory.Databases.forEach(d => {
      dbHTML += '<tr><td>' + r.ServerName + '</td><td><strong>' + d.Name + '</strong></td>' +
        '<td>' + badge(d.State, d.State === 'ONLINE' ? 'b-ok' : 'b-crit') + '</td>' +
        '<td><span class="tag">' + d.RecoveryModel + '</span></td>' +
        '<td>' + d.CompatLevel + '</td><td>' + fmtNum(d.SizeMB) + '</td>' +
        '<td>' + (d.IsReadOnly ? badge('Read Only', 'b-warn') : '—') + '</td></tr>';
    });
  });
  document.getElementById('db-inventory-table').innerHTML = dbHTML + '</tbody></table>';
}

/* ═══════════════════ ALERTS PAGE ═══════════════════ */
function renderAlerts() {
  // Alert volume history
  const pts = Math.min(30, alertLog.length);
  mkChart('chart-alert-history', 'bar', {
    labels: Array.from({ length: pts }, (_, i) => 'T-' + (pts - i)),
    datasets: [{ label: 'Alerts', data: new Array(pts).fill(1), backgroundColor: '#ef4444cc', borderRadius: 4 }]
  });

  // Alert type pie
  const types = { CPU: 0, Memory: 0, Blocking: 0, Queries: 0, Backups: 0, Jobs: 0, Deadlocks: 0, Other: 0 };
  alertLog.forEach(a => {
    if (a.msg.includes('CPU')) types.CPU++;
    else if (a.msg.includes('emory')) types.Memory++;
    else if (a.msg.includes('lock')) types.Blocking++;
    else if (a.msg.includes('quer') || a.msg.includes('Query')) types.Queries++;
    else if (a.msg.includes('ackup')) types.Backups++;
    else if (a.msg.includes('job') || a.msg.includes('Job')) types.Jobs++;
    else if (a.msg.includes('eadlock')) types.Deadlocks++;
    else types.Other++;
  });
  const typeEntries = Object.entries(types).filter(([, v]) => v > 0);
  if (typeEntries.length) {
    mkChart('chart-alert-pie', 'doughnut', {
      labels: typeEntries.map(([k]) => k),
      datasets: [{ data: typeEntries.map(([, v]) => v),
        backgroundColor: COLORS.slice(0, typeEntries.length), borderWidth: 0 }]
    }, { plugins: { legend: { display: false } } });
    document.getElementById('alert-type-legend').innerHTML = typeEntries.map(([k, v], i) =>
      '<div style="display:flex;align-items:center;gap:8px;margin-bottom:6px">' +
      '<div style="width:10px;height:10px;border-radius:2px;background:' + COLORS[i] + '"></div>' +
      '<span style="color:#94a3b8">' + k + ' <strong style="color:#e2e8f0">' + v + '</strong></span></div>'
    ).join('');
  }

  document.getElementById('full-alert-feed').innerHTML =
    alertLog.map(alertItemHTML).join('') || '<div class="no-data" style="padding:16px">No alerts recorded yet</div>';
}

/* ═══════════════════ REFRESH LOOP ═══════════════════ */
async function refresh() {
  try {
    const [res, hist] = await Promise.all([
      fetch('/api/results').then(r => r.json()),
      fetch('/api/history').then(r => r.json())
    ]);
    allResults = res; allHistory = hist;
    collectAlerts(res);
    renderTopbar();

    const activePage = document.querySelector('.nav-tab.active');
    const pageMap = { '📊 Overview': 'overview', '🖥 Servers': 'servers',
      '⚡ Performance': 'performance', '🔍 Queries': 'queries',
      '💾 Backups': 'backups', '🔒 Security': 'security',
      '📈 Capacity': 'capacity', '📋 Inventory': 'inventory', '🚨 Alerts': 'alerts' };
    if (activePage) renderCurrentPage(pageMap[activePage.textContent.trim()] || 'overview');

    document.getElementById('last-update').textContent = 'Updated ' + new Date().toLocaleTimeString();
  } catch (e) {
    document.getElementById('last-update').textContent = 'Connection error...';
  }
}

refresh();
setInterval(refresh, 15000);
</script>
</body>
</html>`

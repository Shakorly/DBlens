package main

const dashHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>DBLens — SQL Intelligence Platform</title>
<script src="https://cdn.jsdelivr.net/npm/chart.js@4.4.0/dist/chart.umd.min.js"></script>
<style>
/* ═══════════════════════════════════════════════════
   DESIGN SYSTEM — Datadog-inspired dark theme
   ═══════════════════════════════════════════════════ */
:root {
  --bg0:#06080f; --bg1:#0b0f1c; --bg2:#10162a; --bg3:#161d33;
  --border:#1f2d4a; --border2:#253660;
  --accent:#6366f1; --accent2:#818cf8;
  --green:#22d3a0; --green2:#10b981;
  --red:#f87171; --red2:#ef4444;
  --orange:#fb923c; --yellow:#fbbf24;
  --blue:#60a5fa; --purple:#c084fc; --cyan:#22d3ee; --pink:#f472b6;
  --text:#f0f4ff; --text2:#94a3b8; --text3:#475569;
  --font:'SF Mono','Fira Code','Cascadia Code',monospace;
  --sans:-apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif;
}
*{box-sizing:border-box;margin:0;padding:0;}
html,body{height:100%;background:var(--bg0);color:var(--text);font-family:var(--sans);overflow-x:hidden;font-size:13px;}
::-webkit-scrollbar{width:4px;height:4px;}
::-webkit-scrollbar-thumb{background:var(--border2);border-radius:2px;}

/* ═══ APP SHELL ═══ */
#app{display:flex;height:100vh;}
#sidebar{
  width:200px;background:var(--bg1);border-right:1px solid var(--border);
  display:flex;flex-direction:column;flex-shrink:0;z-index:20;
  transition:width .2s;overflow:hidden;
}
#sidebar.collapsed{width:44px;}
#main{flex:1;display:flex;flex-direction:column;overflow:hidden;min-width:0;}

/* ═══ SIDEBAR ═══ */
.sb-header{
  height:52px;display:flex;align-items:center;gap:10px;
  padding:0 12px;border-bottom:1px solid var(--border);flex-shrink:0;
}
.sb-logo{font-size:1.1rem;font-weight:800;white-space:nowrap;
  background:linear-gradient(135deg,var(--accent),var(--cyan));
  -webkit-background-clip:text;-webkit-text-fill-color:transparent;}
.sb-toggle{
  margin-left:auto;background:none;border:none;cursor:pointer;
  color:var(--text3);padding:4px;border-radius:4px;flex-shrink:0;font-size:14px;
}
.sb-toggle:hover{color:var(--text);background:var(--bg3);}
.sb-section{padding:16px 8px 4px;font-size:10px;font-weight:700;
  color:var(--text3);letter-spacing:1.5px;text-transform:uppercase;white-space:nowrap;}
.sidebar.collapsed .sb-section{display:none;}
.nav-item{
  display:flex;align-items:center;gap:10px;padding:7px 12px;
  border-radius:6px;cursor:pointer;color:var(--text2);
  transition:.15s;white-space:nowrap;margin:1px 6px;font-size:12.5px;
}
.nav-item:hover{background:var(--bg3);color:var(--text);}
.nav-item.active{background:var(--accent)15;color:var(--accent2);border-left:2px solid var(--accent);}
.nav-item .ni-icon{font-size:13px;flex-shrink:0;width:18px;text-align:center;}
.nav-item .ni-label{overflow:hidden;text-overflow:ellipsis;}
#sidebar.collapsed .ni-label,.sb-collapsed-hide{display:none;}
#sidebar:not(.collapsed) .sb-collapsed-show{display:none;}
.nav-badge{
  margin-left:auto;background:var(--red2);color:#fff;border-radius:10px;
  padding:1px 6px;font-size:10px;font-weight:700;flex-shrink:0;
}

/* ═══ TOPBAR ═══ */
#topbar{
  height:52px;background:var(--bg1);border-bottom:1px solid var(--border);
  display:flex;align-items:center;padding:0 16px;gap:0;flex-shrink:0;
}
.tb-page-title{font-weight:700;font-size:14px;color:var(--text);margin-right:16px;}
.tb-divider{width:1px;height:28px;background:var(--border);margin:0 12px;flex-shrink:0;}
.tb-stat{
  display:flex;flex-direction:column;align-items:center;padding:0 14px;
  border-right:1px solid var(--border);
}
.tb-stat:first-of-type{border-left:1px solid var(--border);}
.tbs-val{font-size:15px;font-weight:800;line-height:1;}
.tbs-lbl{font-size:9px;color:var(--text3);text-transform:uppercase;letter-spacing:1px;margin-top:1px;}
.tb-right{display:flex;align-items:center;gap:8px;margin-left:auto;}
.live-indicator{
  display:flex;align-items:center;gap:6px;background:var(--bg3);
  border:1px solid var(--border2);border-radius:20px;padding:4px 12px;font-size:11px;
}
.live-dot{width:6px;height:6px;border-radius:50%;background:var(--green);}
.live-dot.ok{animation:pulse 2s infinite;}
.live-dot.err{background:var(--red);animation:none;}
@keyframes pulse{0%,100%{opacity:1;box-shadow:0 0 0 0 var(--green)40}50%{opacity:.7;box-shadow:0 0 0 4px transparent}}
#refresh-timer{font-size:10px;color:var(--text3);font-variant-numeric:tabular-nums;}
.srv-filter{
  display:flex;align-items:center;gap:4px;margin-left:12px;
}
.srv-btn{
  background:var(--bg3);border:1px solid var(--border2);border-radius:5px;
  color:var(--text2);padding:3px 10px;font-size:11px;cursor:pointer;transition:.15s;
}
.srv-btn:hover{border-color:var(--accent);color:var(--accent);}
.srv-btn.active{background:var(--accent)20;border-color:var(--accent);color:var(--accent2);}

/* ═══ CONTENT ═══ */
#content{flex:1;overflow-y:auto;padding:16px;background:var(--bg0);}
.page{display:none;}
.page.active{display:block;animation:fi .2s ease;}
@keyframes fi{from{opacity:0;transform:translateY(4px)}to{opacity:1;transform:none}}

/* ═══ GRID SYSTEM ═══ */
.grid{display:grid;gap:12px;margin-bottom:12px;}
.g-2{grid-template-columns:repeat(2,1fr);}
.g-3{grid-template-columns:repeat(3,1fr);}
.g-4{grid-template-columns:repeat(4,1fr);}
.g-6{grid-template-columns:repeat(6,1fr);}
.g-1-2{grid-template-columns:1fr 2fr;}
.g-2-1{grid-template-columns:2fr 1fr;}
.g-1-3{grid-template-columns:1fr 3fr;}
@media(max-width:1200px){.g-4{grid-template-columns:repeat(2,1fr)}.g-6{grid-template-columns:repeat(3,1fr)}}
@media(max-width:900px){.g-2,.g-3,.g-1-2,.g-2-1,.g-1-3{grid-template-columns:1fr}.g-4,.g-6{grid-template-columns:repeat(2,1fr)}}

/* ═══ CARDS ═══ */
.card{
  background:var(--bg1);border:1px solid var(--border);border-radius:8px;overflow:hidden;
}
.card.highlight{border-color:var(--accent)50;box-shadow:0 0 20px var(--accent)08;}
.card-header{
  padding:10px 14px;display:flex;align-items:center;gap:8px;
  border-bottom:1px solid var(--border);
}
.card-icon{
  width:24px;height:24px;border-radius:5px;display:flex;align-items:center;
  justify-content:center;font-size:.8rem;flex-shrink:0;
}
.card-title{font-weight:600;font-size:12.5px;color:var(--text);}
.card-sub{font-size:11px;color:var(--text3);margin-top:1px;}
.card-action{
  margin-left:auto;background:none;border:1px solid var(--border2);
  border-radius:4px;color:var(--text3);padding:3px 8px;font-size:11px;cursor:pointer;
}
.card-action:hover{color:var(--accent);border-color:var(--accent);}
.card-body{padding:12px 14px;}

/* ═══ METRIC TILES (Datadog style) ═══ */
.metric-tile{
  background:var(--bg1);border:1px solid var(--border);border-radius:8px;
  padding:14px;position:relative;overflow:hidden;
}
.metric-tile::before{
  content:'';position:absolute;top:0;left:0;right:0;height:2px;
  background:var(--tile-color,var(--accent));
}
.mt-label{font-size:10px;color:var(--text3);text-transform:uppercase;letter-spacing:1px;font-weight:600;}
.mt-value{font-size:28px;font-weight:900;line-height:1.1;margin:6px 0 2px;color:var(--tile-color,var(--text));}
.mt-sub{font-size:11px;color:var(--text3);}
.mt-delta{font-size:11px;margin-top:4px;}
.mt-delta.up{color:var(--green);}
.mt-delta.down{color:var(--red);}
.mt-spark{height:36px;margin-top:8px;}

/* ═══ RING GAUGE ═══ */
.ring-gauge{display:flex;flex-direction:column;align-items:center;padding:8px 0;}
.ring-canvas{position:relative;}
.ring-canvas canvas{display:block;}
.ring-center-text{
  position:absolute;bottom:8px;left:0;right:0;
  text-align:center;
}
.ring-pct{font-size:18px;font-weight:900;}
.ring-lbl{font-size:9px;color:var(--text3);text-transform:uppercase;letter-spacing:1px;}
.gauge-row{display:flex;gap:0;justify-content:space-around;padding:8px 0;}
.gauge-item{flex:1;display:flex;flex-direction:column;align-items:center;padding:0 4px;}

/* ═══ SERVER CARD ═══ */
.srv-card{
  background:var(--bg1);border:1px solid var(--border);border-radius:8px;overflow:hidden;
}
.srv-card-head{
  padding:12px 14px;display:flex;align-items:center;gap:10px;
  border-bottom:1px solid var(--border);
}
.srv-grade{
  width:40px;height:40px;border-radius:50%;display:flex;align-items:center;
  justify-content:center;font-size:1.1rem;font-weight:900;flex-shrink:0;border:2px solid;
}
.srv-meta{flex:1;min-width:0;}
.srv-name{font-weight:700;font-size:13px;white-space:nowrap;overflow:hidden;text-overflow:ellipsis;}
.srv-detail{font-size:10px;color:var(--text3);margin-top:2px;}
.srv-score{text-align:right;flex-shrink:0;}
.srv-score-val{font-size:22px;font-weight:900;}
.srv-score-bar{height:4px;width:70px;background:var(--border);border-radius:2px;overflow:hidden;margin-top:4px;margin-left:auto;}
.srv-score-fill{height:100%;border-radius:2px;transition:width .5s;}
.srv-metrics{display:grid;grid-template-columns:repeat(6,1fr);border-bottom:1px solid var(--border);}
.srv-metric{
  padding:8px 4px;text-align:center;border-right:1px solid var(--border);
}
.srv-metric:last-child{border-right:none;}
.srv-metric-val{font-size:13px;font-weight:800;}
.srv-metric-lbl{font-size:9px;color:var(--text3);margin-top:1px;}
.srv-tabs{display:flex;overflow-x:auto;border-bottom:1px solid var(--border);}
.srv-tab{
  padding:6px 12px;font-size:11px;cursor:pointer;color:var(--text3);
  border-bottom:2px solid transparent;white-space:nowrap;transition:.15s;
}
.srv-tab.active{color:var(--accent2);border-bottom-color:var(--accent);}
.srv-pane{display:none;padding:10px 14px;min-height:80px;}
.srv-pane.active{display:block;}

/* ═══ TABLES ═══ */
.dt{width:100%;border-collapse:collapse;}
.dt th{
  font-size:10px;color:var(--text3);text-align:left;padding:5px 8px;
  border-bottom:1px solid var(--border);text-transform:uppercase;letter-spacing:.5px;font-weight:600;
}
.dt td{padding:6px 8px;border-bottom:1px solid var(--bg2);font-size:12px;vertical-align:top;}
.dt tr:last-child td{border-bottom:none;}
.dt tr:hover td{background:var(--bg2);}
.dt td.mono{font-family:var(--font);font-size:11px;color:var(--text2);}
.dt td.code{font-family:var(--font);font-size:10px;color:var(--cyan);max-width:300px;word-break:break-all;}

/* ═══ BADGES ═══ */
.b{display:inline-flex;align-items:center;border-radius:4px;padding:1px 6px;font-size:10px;font-weight:700;}
.bok{background:var(--green2)20;color:var(--green);}
.bwarn{background:var(--yellow)20;color:var(--yellow);}
.bcrit{background:var(--red2)20;color:var(--red);}
.bblue{background:var(--blue)20;color:var(--blue);}
.bpurple{background:var(--purple)20;color:var(--purple);}
.bcyan{background:var(--cyan)20;color:var(--cyan);}
.bgray{background:var(--bg3);color:var(--text3);}

/* ═══ PROGRESS BAR ═══ */
.prog{height:6px;border-radius:3px;background:var(--border);overflow:hidden;}
.prog-fill{height:100%;border-radius:3px;transition:width .5s;}
.progress-row{display:flex;align-items:center;gap:8px;padding:4px 0;}
.progress-label{font-size:11px;color:var(--text2);min-width:80px;}
.progress-val{font-size:11px;color:var(--text2);min-width:35px;text-align:right;}

/* ═══ ALERT FEED ═══ */
.alert-feed{display:flex;flex-direction:column;gap:4px;overflow-y:auto;}
.alert-item{
  display:flex;gap:8px;padding:7px 10px;border-radius:5px;border:1px solid;
  font-size:11px;align-items:flex-start;
}
.alert-item.crit{background:var(--red)0a;border-color:var(--red)30;}
.alert-item.warn{background:var(--yellow)0a;border-color:var(--yellow)30;}
.alert-item.info{background:var(--blue)0a;border-color:var(--blue)30;}
.alert-srv{font-weight:700;font-size:10px;color:var(--text3);}
.alert-time{font-size:9px;color:var(--text3);margin-top:1px;}

/* ═══ ADVISORY ═══ */
.adv-item{
  border:1px solid;border-radius:8px;margin-bottom:8px;overflow:hidden;
  transition:box-shadow .2s;
}
.adv-item:hover{box-shadow:0 0 0 1px currentColor;}
.adv-head{padding:10px 14px;display:flex;align-items:flex-start;gap:8px;cursor:pointer;}
.adv-sev{border-radius:3px;padding:1px 6px;font-size:9px;font-weight:800;letter-spacing:.5px;flex-shrink:0;margin-top:2px;}
.adv-title{font-weight:600;font-size:12.5px;flex:1;}
.adv-meta-row{font-size:10px;color:var(--text3);margin-top:2px;}
.adv-chevron{color:var(--text3);font-size:12px;flex-shrink:0;margin-top:2px;transition:transform .2s;}
.adv-chevron.open{transform:rotate(180deg);}
.adv-body{display:none;border-top:1px solid var(--border);}
.adv-body.open{display:block;}
.adv-grid{display:grid;grid-template-columns:1fr 1fr;gap:12px;padding:12px 14px;}
.adv-section-label{font-size:9px;font-weight:700;color:var(--text3);text-transform:uppercase;letter-spacing:1px;margin-bottom:4px;}
.adv-section-text{font-size:11px;color:var(--text2);line-height:1.55;}
.adv-sql-wrap{padding:0 14px 12px;}
.adv-sql-header{display:flex;justify-content:space-between;align-items:center;margin-bottom:6px;}
.adv-sql-label{font-size:9px;font-weight:700;color:var(--text3);text-transform:uppercase;letter-spacing:1px;}
.adv-sql-copy{
  background:var(--blue)15;border:1px solid var(--blue)30;color:var(--blue);
  border-radius:4px;padding:2px 8px;font-size:10px;cursor:pointer;transition:.15s;
}
.adv-sql-copy:hover{background:var(--blue)25;}
.adv-sql-pre{
  background:var(--bg0);border:1px solid var(--border);border-radius:5px;
  padding:10px;font-size:11px;color:var(--cyan);font-family:var(--font);
  overflow:auto;max-height:220px;white-space:pre-wrap;
}
.adv-impact{
  display:inline-flex;align-items:center;gap:5px;
  background:var(--green2)12;color:var(--green);border:1px solid var(--green2)25;
  border-radius:4px;padding:3px 10px;font-size:11px;margin:0 14px 10px;
}

/* ═══ TIMELINE CHART ═══ */
.timeline-wrap{overflow-x:auto;padding:8px 0;}
.timeline-inner{min-width:500px;}

/* ═══ WAIT BAR ═══ */
.wait-bar-row{display:flex;align-items:center;gap:8px;padding:4px 0;border-bottom:1px solid var(--bg2);}
.wait-bar-row:last-child{border-bottom:none;}
.wait-name{font-family:var(--font);font-size:10px;color:var(--text2);min-width:150px;}
.wait-bar-wrap{flex:1;height:5px;background:var(--border);border-radius:3px;overflow:hidden;}
.wait-bar-fill{height:100%;border-radius:3px;}
.wait-pct{font-size:10px;color:var(--text2);min-width:38px;text-align:right;}
.wait-cat{font-size:9px;color:var(--text3);min-width:80px;}

/* ═══ SECTION LABEL ═══ */
.section-label{
  font-size:10px;font-weight:700;color:var(--text3);text-transform:uppercase;
  letter-spacing:1.5px;display:flex;align-items:center;gap:8px;
  margin-bottom:8px;
}
.section-label::after{content:'';flex:1;height:1px;background:var(--border);}

/* ═══ PLAN ANALYSIS BOX ═══ */
.plan-box{
  background:var(--orange)0a;border:1px solid var(--orange)25;border-radius:5px;
  padding:5px 8px;font-size:10px;color:var(--orange);margin-top:4px;
}
.plan-box.ok{background:var(--green)0a;border-color:var(--green)25;color:var(--green);}

/* ═══ NO DATA ═══ */
.nodata{color:var(--text3);font-size:11px;font-style:italic;padding:16px 0;text-align:center;}
</style>
</head>
<body>
<div id="app">

<!-- ═══════════ SIDEBAR ═══════════ -->
<div id="sidebar">
  <div class="sb-header">
    <span style="font-size:1.2rem">⬡</span>
    <span class="sb-logo sb-collapsed-hide">DBLens</span>
    <button class="sb-toggle" onclick="toggleSidebar()" title="Toggle sidebar">◀</button>
  </div>

  <div style="overflow-y:auto;flex:1;padding:6px 0;">
    <div class="sb-section sb-collapsed-hide">MONITOR</div>
    <div class="nav-item active" onclick="showPage('overview',this)"><span class="ni-icon">📊</span><span class="ni-label">Overview</span></div>
    <div class="nav-item" onclick="showPage('servers',this)"><span class="ni-icon">🖥</span><span class="ni-label">Servers</span><span class="nav-badge" id="badge-deg" style="display:none">0</span></div>
    <div class="nav-item" onclick="showPage('performance',this)"><span class="ni-icon">⚡</span><span class="ni-label">Performance</span></div>

    <div class="sb-section sb-collapsed-hide">QUERIES</div>
    <div class="nav-item" onclick="showPage('queries',this)"><span class="ni-icon">🔍</span><span class="ni-label">Query Analytics</span></div>
    <div class="nav-item" onclick="showPage('waits',this)"><span class="ni-icon">⏳</span><span class="ni-label">Wait Statistics</span></div>

    <div class="sb-section sb-collapsed-hide">OPERATIONS</div>
    <div class="nav-item" onclick="showPage('backups',this)"><span class="ni-icon">💾</span><span class="ni-label">Backups</span></div>
    <div class="nav-item" onclick="showPage('jobs',this)"><span class="ni-icon">🤖</span><span class="ni-label">SQL Agent Jobs</span></div>
    <div class="nav-item" onclick="showPage('security',this)"><span class="ni-icon">🔒</span><span class="ni-label">Security Audit</span></div>

    <div class="sb-section sb-collapsed-hide">PLANNING</div>
    <div class="nav-item" onclick="showPage('capacity',this)"><span class="ni-icon">📈</span><span class="ni-label">Capacity</span></div>
    <div class="nav-item" onclick="showPage('inventory',this)"><span class="ni-icon">📋</span><span class="ni-label">Inventory</span></div>

    <div class="sb-section sb-collapsed-hide">INTELLIGENCE</div>
    <div class="nav-item" onclick="showPage('advisor',this)"><span class="ni-icon">🧠</span><span class="ni-label">AI Advisor</span><span class="nav-badge" id="badge-adv" style="display:none">0</span></div>
    <div class="nav-item" onclick="showPage('alerts',this)"><span class="ni-icon">🚨</span><span class="ni-label">Alert Log</span></div>
  </div>

  <div style="padding:10px 8px;border-top:1px solid var(--border);flex-shrink:0;">
    <div class="nav-item" onclick="showPage('settings',this)"><span class="ni-icon">⚙️</span><span class="ni-label">Settings</span></div>
  </div>
</div>

<!-- ═══════════ MAIN ═══════════ -->
<div id="main">

<!-- TOPBAR -->
<div id="topbar">
  <div class="tb-page-title" id="page-title">Overview</div>
  <div class="tb-divider"></div>
  <div id="top-stats" style="display:flex;"></div>
  <div class="tb-right">
    <div class="srv-filter" id="srv-filter"></div>
    <div class="tb-divider"></div>
    <div class="live-indicator">
      <div class="live-dot ok" id="live-dot"></div>
      <span id="live-status" style="font-size:11px">Live</span>
    </div>
    <span id="refresh-timer" style="margin-left:8px;">⟳ 15s</span>
    <span id="last-update" style="font-size:10px;color:var(--text3);margin-left:8px;">Connecting...</span>
  </div>
</div>

<!-- ═══ CONTENT ═══ -->
<div id="content">

<!-- ████████ OVERVIEW ████████ -->
<div class="page active" id="page-overview">
  <!-- KPI row -->
  <div class="grid g-6" id="kpi-tiles" style="margin-bottom:12px;"></div>

  <!-- Charts row -->
  <div class="grid g-2" style="margin-bottom:12px;">
    <div class="card">
      <div class="card-header">
        <div class="card-icon" style="background:var(--accent)20">📊</div>
        <div><div class="card-title">Health Score Trend</div><div class="card-sub">All servers · last 20 polls</div></div>
      </div>
      <div class="card-body" style="height:160px;padding:8px 12px;"><canvas id="ch-health-trend"></canvas></div>
    </div>
    <div class="card">
      <div class="card-header">
        <div class="card-icon" style="background:var(--orange)20">📡</div>
        <div><div class="card-title">Active vs Blocked Sessions</div><div class="card-sub">Real-time session state</div></div>
      </div>
      <div class="card-body" style="height:160px;padding:8px 12px;"><canvas id="ch-sessions-live"></canvas></div>
    </div>
  </div>

  <!-- Gauge row -->
  <div class="grid g-4" style="margin-bottom:12px;" id="gauge-tiles"></div>

  <!-- Bottom row -->
  <div class="grid g-1-2">
    <div class="card">
      <div class="card-header"><div class="card-icon" style="background:var(--red)20">🚨</div><div class="card-title">Live Alerts</div><div class="card-action" onclick="showPage('alerts',null)">View All</div></div>
      <div class="card-body" style="padding:8px;"><div class="alert-feed" style="max-height:200px;" id="ov-alert-feed"></div></div>
    </div>
    <div class="card">
      <div class="card-header"><div class="card-icon" style="background:var(--purple)20">📊</div><div class="card-title">Grade Distribution</div></div>
      <div class="card-body" style="display:flex;align-items:center;gap:20px;justify-content:center;height:200px;">
        <canvas id="ch-grade-dist" style="max-width:160px;max-height:160px;"></canvas>
        <div id="grade-legend" style="font-size:11px;"></div>
      </div>
    </div>
  </div>
</div>

<!-- ████████ SERVERS ████████ -->
<div class="page" id="page-servers">
  <div id="server-grid" style="display:grid;grid-template-columns:repeat(auto-fill,minmax(520px,1fr));gap:12px;"></div>
</div>

<!-- ████████ PERFORMANCE ████████ -->
<div class="page" id="page-performance">
  <div class="grid g-2" style="margin-bottom:12px;">
    <div class="card"><div class="card-header"><div class="card-icon" style="background:var(--red)20">🔥</div><div class="card-title">SQL CPU %</div></div><div class="card-body" style="height:180px;padding:8px 12px;"><canvas id="ch-p-cpu"></canvas></div></div>
    <div class="card"><div class="card-header"><div class="card-icon" style="background:var(--purple)20">🧠</div><div class="card-title">Memory Used %</div></div><div class="card-body" style="height:180px;padding:8px 12px;"><canvas id="ch-p-mem"></canvas></div></div>
  </div>
  <div class="grid g-3" style="margin-bottom:12px;">
    <div class="card"><div class="card-header"><div class="card-icon" style="background:var(--cyan)20">⚡</div><div class="card-title">Batch Req/sec</div></div><div class="card-body" style="height:140px;padding:8px 12px;"><canvas id="ch-p-batch"></canvas></div></div>
    <div class="card"><div class="card-header"><div class="card-icon" style="background:var(--yellow)20">🔄</div><div class="card-title">Active Transactions</div></div><div class="card-body" style="height:140px;padding:8px 12px;"><canvas id="ch-p-txn"></canvas></div></div>
    <div class="card"><div class="card-header"><div class="card-icon" style="background:var(--red)20">🔒</div><div class="card-title">Blocked Sessions</div></div><div class="card-body" style="height:140px;padding:8px 12px;"><canvas id="ch-p-block"></canvas></div></div>
  </div>
</div>

<!-- ████████ QUERIES ████████ -->
<div class="page" id="page-queries">
  <div class="card" style="margin-bottom:12px;">
    <div class="card-header"><div class="card-icon" style="background:var(--orange)20">📉</div><div><div class="card-title">Query Duration Timeline</div><div class="card-sub">Active long-running queries — coloured by wait type</div></div></div>
    <div class="card-body" style="height:150px;padding:8px 12px;"><div class="timeline-wrap"><div class="timeline-inner"><canvas id="ch-q-timeline"></canvas></div></div></div>
  </div>
  <div class="grid g-2-1" style="margin-bottom:12px;">
    <div class="card">
      <div class="card-header"><div class="card-icon" style="background:var(--red)20">🐢</div><div class="card-title">Active Long-Running Queries</div><div class="card-sub" style="margin-left:6px;" id="lrq-sub"></div></div>
      <div class="card-body" style="padding:8px;"><div id="lrq-table"></div></div>
    </div>
    <div class="card">
      <div class="card-header"><div class="card-icon" style="background:var(--purple)20">🗂</div><div class="card-title">Missing Indexes</div></div>
      <div class="card-body" style="padding:8px;"><div id="miss-idx-table"></div></div>
    </div>
  </div>
  <div class="grid g-2">
    <div class="card"><div class="card-header"><div class="card-icon" style="background:var(--yellow)20">📉</div><div class="card-title">Plan Cache — Top Slow</div></div><div class="card-body" style="padding:8px;"><div id="slow-q-table"></div></div></div>
    <div class="card"><div class="card-header"><div class="card-icon" style="background:var(--orange)20">🔧</div><div class="card-title">Fragmented Indexes <span class="b bgray" style="margin-left:4px">Online Fix Available</span></div></div><div class="card-body" style="padding:8px;"><div id="frag-idx-table"></div></div></div>
  </div>
</div>

<!-- ████████ WAITS ████████ -->
<div class="page" id="page-waits">
  <div class="grid g-2" style="margin-bottom:12px;">
    <div class="card">
      <div class="card-header"><div class="card-icon" style="background:var(--yellow)20">⏳</div><div class="card-title">Wait Category Breakdown</div></div>
      <div class="card-body" style="height:200px;padding:8px 12px;"><canvas id="ch-wait-pie"></canvas></div>
    </div>
    <div class="card">
      <div class="card-header"><div class="card-icon" style="background:var(--cyan)20">📊</div><div class="card-title">Top Waits by Server</div></div>
      <div class="card-body" style="height:200px;padding:8px 12px;"><canvas id="ch-wait-bar"></canvas></div>
    </div>
  </div>
  <div class="card"><div class="card-header"><div class="card-icon" style="background:var(--yellow)20">📋</div><div class="card-title">Detailed Wait Analysis</div></div><div class="card-body" style="padding:8px;"><div id="wait-detail"></div></div></div>
</div>

<!-- ████████ BACKUPS ████████ -->
<div class="page" id="page-backups">
  <div class="grid g-4" id="bak-kpi-row" style="margin-bottom:12px;"></div>
  <div class="card"><div class="card-header"><div class="card-icon" style="background:var(--blue)20">💾</div><div class="card-title">Backup Status — All Databases</div></div><div class="card-body" style="padding:8px;"><div id="bak-table"></div></div></div>
</div>

<!-- ████████ JOBS ████████ -->
<div class="page" id="page-jobs">
  <div class="grid g-3" id="job-kpi-row" style="margin-bottom:12px;"></div>
  <div class="grid g-2">
    <div class="card"><div class="card-header"><div class="card-icon" style="background:var(--red)20">❌</div><div class="card-title">Failed Jobs</div></div><div class="card-body" style="padding:8px;"><div id="failed-jobs-table"></div></div></div>
    <div class="card"><div class="card-header"><div class="card-icon" style="background:var(--yellow)20">⏳</div><div class="card-title">Long-Running Jobs (&gt;30min)</div></div><div class="card-body" style="padding:8px;"><div id="long-jobs-table"></div></div></div>
  </div>
</div>

<!-- ████████ SECURITY ████████ -->
<div class="page" id="page-security">
  <div class="grid g-4" id="sec-kpi-row" style="margin-bottom:12px;"></div>
  <div class="grid g-2" style="margin-bottom:12px;">
    <div class="card"><div class="card-header"><div class="card-icon" style="background:var(--red)20">⚠️</div><div class="card-title">Configuration Risks</div></div><div class="card-body" style="padding:8px;"><div id="sec-risks"></div></div></div>
    <div class="card"><div class="card-header"><div class="card-icon" style="background:var(--purple)20">👑</div><div class="card-title">Privileged Accounts</div></div><div class="card-body" style="padding:8px;"><div id="priv-users-table"></div></div></div>
  </div>
  <div class="grid g-2">
    <div class="card"><div class="card-header"><div class="card-icon" style="background:var(--orange)20">🔗</div><div class="card-title">Linked Servers</div></div><div class="card-body" style="padding:8px;"><div id="linked-srv-table"></div></div></div>
    <div class="card"><div class="card-header"><div class="card-icon" style="background:var(--cyan)20">🕵️</div><div class="card-title">Suspicious Connections</div></div><div class="card-body" style="padding:8px;"><div id="susp-conn-table"></div></div></div>
  </div>
</div>

<!-- ████████ CAPACITY ████████ -->
<div class="page" id="page-capacity">
  <div class="grid g-1-2" style="margin-bottom:12px;">
    <div class="card">
      <div class="card-header"><div class="card-icon" style="background:var(--green)20">📡</div><div class="card-title">Resource Headroom Radar</div></div>
      <div class="card-body" style="height:280px;padding:8px;display:flex;justify-content:center;align-items:center;"><canvas id="ch-radar" style="max-width:250px;max-height:250px;"></canvas></div>
    </div>
    <div class="card"><div class="card-header"><div class="card-icon" style="background:var(--orange)20">💿</div><div class="card-title">Database Disk — Days Until Full</div></div><div class="card-body" style="padding:8px;"><div id="cap-disk-table"></div></div></div>
  </div>
</div>

<!-- ████████ INVENTORY ████████ -->
<div class="page" id="page-inventory">
  <div class="card" style="margin-bottom:12px;"><div class="card-header"><div class="card-icon" style="background:var(--green)20">🖥</div><div class="card-title">Server Inventory</div></div><div class="card-body" style="padding:8px;"><div id="inv-srv-table"></div></div></div>
  <div class="card"><div class="card-header"><div class="card-icon" style="background:var(--blue)20">🗄</div><div class="card-title">Database Inventory</div></div><div class="card-body" style="padding:8px;"><div id="inv-db-table"></div></div></div>
</div>

<!-- ████████ ADVISOR ████████ -->
<div class="page" id="page-advisor">
  <div class="grid g-6" id="adv-kpi-row" style="margin-bottom:12px;"></div>
  <div class="grid g-2" style="margin-bottom:12px;">
    <div class="card"><div class="card-header"><div class="card-icon" style="background:var(--red)20">🥧</div><div class="card-title">Advisory Distribution</div></div><div class="card-body" style="height:180px;padding:8px;display:flex;justify-content:center;align-items:center;"><canvas id="ch-adv-dist" style="max-width:160px;max-height:160px;"></canvas></div></div>
    <div class="card"><div class="card-header"><div class="card-icon" style="background:var(--purple)20">📊</div><div class="card-title">By Category</div></div><div class="card-body" style="height:180px;padding:8px 12px;"><canvas id="ch-adv-cat"></canvas></div></div>
  </div>
  <div id="adv-list"></div>
</div>

<!-- ████████ ALERTS ████████ -->
<div class="page" id="page-alerts">
  <div class="grid g-3" style="margin-bottom:12px;">
    <div class="card"><div class="card-header"><div class="card-icon" style="background:var(--red)20">🥧</div><div class="card-title">Alert Distribution</div></div><div class="card-body" style="height:180px;padding:8px;display:flex;justify-content:center;align-items:center;"><canvas id="ch-alert-pie" style="max-width:160px;max-height:160px;"></canvas></div></div>
    <div class="card"><div class="card-header"><div class="card-icon" style="background:var(--yellow)20">📊</div><div class="card-title">By Server</div></div><div class="card-body" style="padding:8px;"><div id="alert-by-srv"></div></div></div>
    <div class="card"><div class="card-header"><div class="card-icon" style="background:var(--blue)20">📈</div><div class="card-title">Summary</div></div><div class="card-body"><div id="alert-summary-nums" style="display:grid;grid-template-columns:1fr 1fr;gap:12px;padding:4px;"></div></div></div>
  </div>
  <div class="card"><div class="card-header"><div class="card-icon" style="background:var(--red)20">📜</div><div class="card-title">Full Alert Log</div><button class="card-action" onclick="alerts=[];rAlerts()">Clear</button></div><div class="card-body" style="padding:8px;"><div class="alert-feed" style="max-height:500px;" id="alert-log-full"></div></div></div>
</div>

<!-- ████████ SETTINGS ████████ -->
<div class="page" id="page-settings">
  <div class="card" style="max-width:600px;">
    <div class="card-header"><div class="card-icon" style="background:var(--accent)20">⚙️</div><div class="card-title">Dashboard Settings</div></div>
    <div class="card-body">
      <div style="display:flex;flex-direction:column;gap:16px;">
        <div>
          <div style="font-size:11px;font-weight:700;color:var(--text2);margin-bottom:6px">Refresh Interval</div>
          <div style="display:flex;gap:6px;">
            <button class="card-action" onclick="setRefresh(10000)" id="ri-10">10s</button>
            <button class="card-action" onclick="setRefresh(15000)" id="ri-15">15s</button>
            <button class="card-action" onclick="setRefresh(30000)" id="ri-30">30s</button>
            <button class="card-action" onclick="setRefresh(60000)" id="ri-60">60s</button>
          </div>
        </div>
        <div>
          <div style="font-size:11px;font-weight:700;color:var(--text2);margin-bottom:6px">API Status</div>
          <div style="display:flex;gap:8px;">
            <button class="card-action" onclick="testAPI('/api/results')">Test /api/results</button>
            <button class="card-action" onclick="testAPI('/api/history')">Test /api/history</button>
            <button class="card-action" onclick="testAPI('/api/advisories')">Test /api/advisories</button>
          </div>
          <div id="api-test-result" style="margin-top:8px;font-size:11px;color:var(--text3);font-family:var(--font);"></div>
        </div>
        <div>
          <div style="font-size:11px;font-weight:700;color:var(--text2);margin-bottom:6px">DBLens Version</div>
          <div style="font-size:12px;color:var(--text3);font-family:var(--font);">v6.0 — Intelligent SQL Monitor</div>
        </div>
      </div>
    </div>
  </div>
</div>

</div><!-- /content -->
</div><!-- /main -->
</div><!-- /app -->

<script>
'use strict';

/* ═══════════════════════════════════════════
   STATE
   ═══════════════════════════════════════════ */
let R = {}, H = {}, advData = [], alerts = [], charts = {};
let activeSrv = null;
let refreshInterval = 15000;
let refreshTimer = null;
let countdown = 15;
let countdownTimer = null;
const C = ['#6366f1','#22d3a0','#fb923c','#f87171','#60a5fa','#c084fc','#22d3ee','#fbbf24','#84cc16','#f472b6'];
const W_COLORS = { 'LCK_M_X':'#f87171','LCK_M_S':'#fb923c','ASYNC_NETWORK_IO':'#fbbf24','PAGEIOLATCH_SH':'#60a5fa','PAGEIOLATCH_EX':'#c084fc','CXPACKET':'#22d3ee','WRITELOG':'#22d3a0','SOS_SCHEDULER_YIELD':'#fb923c' };

/* ═══ SAFE ACCESSORS ═══ */
const gs = (o,...k) => k.reduce((x,k) => x!=null&&typeof x==='object'?x[k]:null, o);
const gn = (o,...k) => { const v=gs(o,...k); return v!=null?Number(v):0; };
const ga = (o,...k) => { const v=gs(o,...k); return Array.isArray(v)?v:[]; };
const gt = (o,...k) => { const v=gs(o,...k); return v!=null?String(v):''; };
const esc = s => String(s||'').replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/"/g,'&quot;');
const fN = n => n==null?'—':Number(n).toLocaleString(undefined,{maximumFractionDigits:1});
const fD = d => { try { return d?new Date(d).toLocaleString():'—'; } catch(e) { return '—'; } };
const nd = (m) => '<div class="nodata">'+(m||'Waiting for data...')+'</div>';
const gc = s => s>=90?'var(--green)':s>=75?'var(--blue)':s>=60?'var(--yellow)':s>=40?'var(--orange)':'var(--red)';
const srvs = () => activeSrv ? Object.values(R).filter(r=>gt(r,'ServerName')===activeSrv) : Object.values(R);

/* ═══ SIDEBAR ═══ */
function toggleSidebar() {
  const sb = document.getElementById('sidebar');
  sb.classList.toggle('collapsed');
  document.querySelector('.sb-toggle').textContent = sb.classList.contains('collapsed') ? '▶' : '◀';
}

/* ═══ NAVIGATION ═══ */
function showPage(id, el) {
  document.querySelectorAll('.page').forEach(p => p.classList.remove('active'));
  document.querySelectorAll('.nav-item').forEach(n => n.classList.remove('active'));
  document.getElementById('page-'+id).classList.add('active');
  if(el) el.classList.add('active');
  document.getElementById('page-title').textContent = {
    overview:'Overview',servers:'Servers',performance:'Performance',
    queries:'Query Analytics',waits:'Wait Statistics',backups:'Backups',
    jobs:'SQL Agent Jobs',security:'Security Audit',capacity:'Capacity Planning',
    inventory:'Inventory',advisor:'AI Advisor',alerts:'Alert Log',settings:'Settings'
  }[id] || id;
  try { renderPage(id); } catch(e) { console.error('Page render error:', id, e); }
}

function renderPage(id) {
  const m = {
    overview:rOverview, servers:rServers, performance:rPerf,
    queries:rQueries, waits:rWaits, backups:rBackups,
    jobs:rJobs, security:rSecurity, capacity:rCapacity,
    inventory:rInventory, advisor:rAdvisor, alerts:rAlerts
  };
  if(m[id]) m[id]();
}

/* ═══ CHART HELPER ═══ */
function mkChart(id, type, data, opts) {
  try {
    const el = document.getElementById(id); if(!el) return;
    if(charts[id]) { try { charts[id].destroy(); } catch(e){} delete charts[id]; }
    const scales = (type==='pie'||type==='doughnut'||type==='radar') ? undefined : {
      x:{ticks:{color:'#475569',font:{size:9},maxTicksLimit:8},grid:{color:'#1f2d4a',lineWidth:.5}},
      y:{ticks:{color:'#475569',font:{size:9}},grid:{color:'#1f2d4a',lineWidth:.5}}
    };
    charts[id] = new Chart(el, {
      type, data,
      options: Object.assign({
        responsive:true, maintainAspectRatio:false, animation:{duration:300},
        plugins:{legend:{labels:{color:'#94a3b8',font:{size:10},boxWidth:10}}},
        scales
      }, opts||{})
    });
  } catch(e) { console.error('Chart error:', id, e); }
}

/* ═══ BADGE / KPI helpers ═══ */
function b(txt, cls) { return '<span class="b '+(cls||'bgray')+'">'+esc(String(txt||''))+'</span>'; }
function kpiTile(val, lbl, color, sub, spark) {
  return '<div class="metric-tile" style="--tile-color:'+color+'"><div class="mt-label">'+lbl+'</div><div class="mt-value">'+val+'</div>'+(sub?'<div class="mt-sub">'+sub+'</div>':'')+'</div>';
}
function miniTile(val,lbl,c){return'<div style="background:var(--bg2);border:1px solid var(--border);border-radius:6px;padding:8px;text-align:center"><div style="font-size:16px;font-weight:800;color:'+c+'">'+val+'</div><div style="font-size:9px;color:var(--text3);margin-top:2px">'+lbl+'</div></div>';}

/* ═══ ALERTS ═══ */
function collectAlerts() {
  srvs().forEach(r => {
    const srv = gt(r,'ServerName'), ts = new Date().toLocaleTimeString();
    const blk = gn(r,'Connections','BlockedSessions');
    const cpu = gn(r,'Resources','SQLCPUPercent');
    const lrq = ga(r,'Queries','ActiveLongRunning').length;
    const dl  = ga(r,'Deadlocks').length;
    if(blk > 0) pushAlert('crit', srv, blk+' blocked sessions', ts);
    if(lrq > 0) pushAlert('warn', srv, lrq+' long-running queries', ts);
    if(cpu > 80) pushAlert('crit', srv, 'CPU '+cpu+'%', ts);
    if(dl  > 0) pushAlert('crit', srv, 'Deadlock detected', ts);
    ga(gs(r,'Jobs'),'FailedJobs').forEach(j => pushAlert('warn', srv, 'Job failed: '+gt(j,'JobName'), ts));
    ga(gs(r,'Backups'),'Databases').filter(d=>d.IsAlertFull).forEach(d => pushAlert('warn', srv, 'Backup overdue: '+gt(d,'DatabaseName'), ts));
  });
}
function pushAlert(sev, srv, msg, ts) {
  alerts.unshift({sev,srv,msg,ts});
  if(alerts.length > 500) alerts.pop();
}
function alertHTML(a) {
  const ic = {crit:'🔴',warn:'🟡',info:'🔵'}[a.sev]||'⚪';
  return '<div class="alert-item '+a.sev+'"><span>'+ic+'</span><div><div class="alert-srv">'+esc(a.srv)+'</div><div>'+esc(a.msg)+'</div><div class="alert-time">'+a.ts+'</div></div></div>';
}

/* ═══ TOPBAR ═══ */
function rTopbar() {
  const all = Object.values(R);
  const online = all.filter(s=>gs(s,'Health')).length;
  const degrad = all.filter(s=>'DF'.includes(gt(s,'Health','Grade'))).length;
  const sess   = all.reduce((a,s)=>a+gn(s,'Connections','TotalSessions'),0);
  const sc     = all.filter(s=>gs(s,'Health'));
  const avgSc  = sc.length ? Math.round(sc.reduce((a,s)=>a+gn(s,'Health','Score'),0)/sc.length) : 0;
  const dl     = all.reduce((a,s)=>a+ga(s,'Deadlocks').length,0);
  const blk    = all.reduce((a,s)=>a+gn(s,'Connections','BlockedSessions'),0);

  document.getElementById('top-stats').innerHTML = [
    ts(online, 'Servers', 'var(--blue)'),
    ts(degrad, 'Degraded', degrad?'var(--red)':'var(--green)'),
    ts(avgSc, 'Avg Score', gc(avgSc)),
    ts(sess, 'Sessions', 'var(--text2)'),
    ts(blk, 'Blocked', blk?'var(--red)':'var(--green)'),
    ts(dl, 'Deadlocks', dl?'var(--red)':'var(--green)'),
  ].join('');

  // Nav badges
  const badgeDeg = document.getElementById('badge-deg');
  if(badgeDeg){if(degrad>0){badgeDeg.textContent=degrad;badgeDeg.style.display='';}else{badgeDeg.style.display='none';}}

  // Server filter
  document.getElementById('srv-filter').innerHTML =
    '<button class="srv-btn'+(activeSrv===null?' active':'')+'" onclick="setSrv(null)">All Servers</button>'
    +all.map(r=>'<button class="srv-btn'+(activeSrv===gt(r,'ServerName')?' active':'')+'" onclick="setSrv(\''+esc(gt(r,'ServerName'))+'\')">'+esc(gt(r,'ServerName'))+'</button>').join('');
}
function ts(v,l,c) { return '<div class="tb-stat"><div class="tbs-val" style="color:'+c+'">'+v+'</div><div class="tbs-lbl">'+l+'</div></div>'; }
function setSrv(name) {
  activeSrv = name;
  rTopbar();
  const active = document.querySelector('.nav-item.active');
  if(active) {
    const id = active.getAttribute('onclick')?.match(/'(\w+)'/)?.[1];
    if(id) try{renderPage(id);}catch(e){}
  }
}

/* ═══════════════════════════════════════════
   PAGE RENDERERS
   ═══════════════════════════════════════════ */

/* ████ OVERVIEW ████ */
function rOverview() {
  try {
    const all = srvs();
    const sess = all.reduce((a,s)=>a+gn(s,'Connections','TotalSessions'),0);
    const blk  = all.reduce((a,s)=>a+gn(s,'Connections','BlockedSessions'),0);
    const lrq  = all.reduce((a,s)=>a+ga(s,'Queries','ActiveLongRunning').length,0);
    const dl   = all.reduce((a,s)=>a+ga(s,'Deadlocks').length,0);
    const avgCPU = all.length ? Math.round(all.reduce((a,s)=>a+Math.max(0,gn(s,'Resources','SQLCPUPercent')),0)/all.length) : 0;
    const sc = all.filter(s=>gs(s,'Health'));
    const avgSc = sc.length ? Math.round(sc.reduce((a,s)=>a+gn(s,'Health','Score'),0)/sc.length) : 0;

    document.getElementById('kpi-tiles').innerHTML =
      kpiTile(all.length, 'SERVERS ONLINE', 'var(--blue)', 'Monitored') +
      kpiTile(avgSc+'/100', 'AVG HEALTH', gc(avgSc), 'Score') +
      kpiTile(sess, 'SESSIONS', 'var(--cyan)', 'Active connections') +
      kpiTile(blk, 'BLOCKED', blk?'var(--red)':'var(--green)', blk?'Needs attention':'All clear') +
      kpiTile(lrq, 'SLOW QUERIES', lrq?'var(--orange)':'var(--green)', 'Above threshold') +
      kpiTile(dl, 'DEADLOCKS', dl?'var(--red)':'var(--green)', 'This cycle');

    // Health trend
    const hKeys = Object.keys(H);
    if(hKeys.length) {
      const pts = (H[hKeys[0]]||[]).slice(-20);
      const labels = pts.map(p=>{try{return new Date(p.ts).toLocaleTimeString();}catch(e){return '';}});
      mkChart('ch-health-trend','line',{labels,datasets:hKeys.map((k,i)=>({
        label:k,data:(H[k]||[]).slice(-20).map(p=>p.health||0),
        borderColor:C[i%C.length],backgroundColor:C[i%C.length]+'20',
        fill:true,tension:.3,pointRadius:0,borderWidth:1.5
      }))},{scales:{y:{min:0,max:100,ticks:{color:'#475569'},grid:{color:'#1f2d4a',lineWidth:.5}},x:{ticks:{color:'#475569',font:{size:9},maxTicksLimit:7},grid:{display:false}}}});

      // Sessions chart
      mkChart('ch-sessions-live','line',{labels,datasets:hKeys.flatMap((k,i)=>[
        {label:k+' Sessions',data:(H[k]||[]).slice(-20).map(p=>p.sess||0),borderColor:C[i%C.length],backgroundColor:C[i%C.length]+'15',fill:true,tension:.3,pointRadius:0,borderWidth:1.5},
        {label:k+' Blocked',data:(H[k]||[]).slice(-20).map(p=>p.blocked||0),borderColor:'#f87171',fill:false,tension:.3,pointRadius:0,borderWidth:1,borderDash:[4,3]}
      ])},{scales:{y:{ticks:{color:'#475569'},grid:{color:'#1f2d4a',lineWidth:.5}},x:{ticks:{color:'#475569',font:{size:9},maxTicksLimit:7},grid:{display:false}}}});
    }

    // Gauge tiles
    document.getElementById('gauge-tiles').innerHTML = all.map((r,i)=>{
      const srv = gt(r,'ServerName');
      const cpu = Math.max(0,gn(r,'Resources','SQLCPUPercent'));
      const tot = gn(r,'Resources','TotalMemoryMB'), av = gn(r,'Resources','AvailableMemoryMB');
      const mem = tot>0?Math.round((1-av/tot)*100):0;
      const sc  = gn(r,'Health','Score');
      const gr  = gt(r,'Health','Grade')||'?';
      const col = gc(sc).replace('var(','').replace(')','');
      const cid = 'gg'+i;
      return '<div class="card"><div class="card-header" style="gap:8px">'
        +'<div style="width:32px;height:32px;border-radius:50%;border:2px solid '+gc(sc)+';display:flex;align-items:center;justify-content:center;font-weight:800;color:'+gc(sc)+'">'+esc(gr)+'</div>'
        +'<div><div class="card-title" style="font-size:12px">'+esc(srv)+'</div>'
          +'<div class="card-sub">Score: '+sc+'/100</div></div></div>'
        +'<div class="card-body" style="padding:8px;">'
          +'<div style="display:grid;grid-template-columns:1fr 1fr;gap:6px;margin-bottom:8px;">'
            +'<div style="text-align:center"><div style="height:70px;position:relative"><canvas id="'+cid+'_cpu"></canvas><div style="position:absolute;bottom:2px;left:0;right:0;text-align:center;font-size:13px;font-weight:800;color:'+(cpu>80?'var(--red)':cpu>60?'var(--yellow)':'var(--green)')+'">'+cpu+'%</div></div><div style="font-size:9px;color:var(--text3)">SQL CPU</div></div>'
            +'<div style="text-align:center"><div style="height:70px;position:relative"><canvas id="'+cid+'_mem"></canvas><div style="position:absolute;bottom:2px;left:0;right:0;text-align:center;font-size:13px;font-weight:800;color:'+(mem>90?'var(--red)':mem>80?'var(--yellow)':'var(--purple)')+'">'+mem+'%</div></div><div style="font-size:9px;color:var(--text3)">Memory</div></div>'
          +'</div>'
          +'<div style="display:grid;grid-template-columns:repeat(3,1fr);gap:4px;">'
            +miniTile(gn(r,'Connections','TotalSessions'),'Sessions',gn(r,'Connections','TotalSessions')>150?'var(--yellow)':'var(--text2)')
            +miniTile(gn(r,'Connections','BlockedSessions'),'Blocked',gn(r,'Connections','BlockedSessions')>0?'var(--red)':'var(--green)')
            +miniTile(ga(r,'Queries','ActiveLongRunning').length,'Slow Q',ga(r,'Queries','ActiveLongRunning').length>0?'var(--orange)':'var(--green)')
          +'</div>'
        +'</div></div>';
    }).join('');

    // Draw ring gauges after DOM
    setTimeout(()=>{
      all.forEach((r,i)=>{
        const cpu = Math.max(0,gn(r,'Resources','SQLCPUPercent'));
        const tot = gn(r,'Resources','TotalMemoryMB'), av = gn(r,'Resources','AvailableMemoryMB');
        const mem = tot>0?Math.round((1-av/tot)*100):0;
        drawRing('gg'+i+'_cpu',cpu,cpu>80?'#f87171':cpu>60?'#fbbf24':'#22d3a0');
        drawRing('gg'+i+'_mem',mem,mem>90?'#f87171':mem>80?'#fbbf24':'#c084fc');
      });
    },50);

    // Grade distribution
    const grades={A:0,B:0,C:0,D:0,F:0};
    all.forEach(r=>{const g=gt(r,'Health','Grade');if(g)grades[g]=(grades[g]||0)+1;});
    const gc2={A:'#22d3a0',B:'#60a5fa',C:'#fbbf24',D:'#fb923c',F:'#f87171'};
    const ge=Object.entries(grades).filter(([,v])=>v>0);
    mkChart('ch-grade-dist','doughnut',{labels:ge.map(([k])=>'Grade '+k),datasets:[{data:ge.map(([,v])=>v),backgroundColor:ge.map(([k])=>gc2[k]),borderWidth:0,hoverOffset:4}]},{plugins:{legend:{display:false}}});
    document.getElementById('grade-legend').innerHTML=ge.map(([k,v])=>'<div style="display:flex;align-items:center;gap:6px;margin-bottom:5px"><div style="width:8px;height:8px;border-radius:2px;background:'+gc2[k]+'"></div><span style="font-size:11px;color:'+gc2[k]+';font-weight:700">'+k+'</span><span style="font-size:11px;color:var(--text3)"> — '+v+'</span></div>').join('');

    // Alert feed
    document.getElementById('ov-alert-feed').innerHTML=alerts.slice(0,5).map(alertHTML).join('')||nd('No alerts');
  } catch(e) { console.error('overview',e); }
}

function drawRing(id,pct,color) {
  try {
    const el=document.getElementById(id);if(!el)return;
    const v=Math.max(0,Math.min(100,pct));
    if(charts[id])try{charts[id].destroy();}catch(e){}
    charts[id]=new Chart(el,{type:'doughnut',data:{datasets:[{data:[v,100-v],backgroundColor:[color,'#1f2d4a'],borderWidth:0,circumference:180,rotation:-90}]},options:{responsive:true,maintainAspectRatio:false,animation:{duration:500},cutout:'72%',plugins:{legend:{display:false},tooltip:{enabled:false}}}});
  }catch(e){}
}

/* ████ SERVERS ████ */
function rServers() {
  try {
    const grid = document.getElementById('server-grid');
    grid.innerHTML = '';
    const all = srvs();
    if(!all.length){grid.innerHTML='<div class="card"><div class="card-body">'+nd()+'</div></div>';return;}
    all.forEach((r,i)=>{try{buildSrvCard(r,i,grid);}catch(e){console.error('srv card',e);}});
  } catch(e) { console.error('servers',e); }
}

function buildSrvCard(r,si,wrap) {
  const srv=gt(r,'ServerName');
  const h=gs(r,'Health')||{Grade:'?',Score:0,Penalties:[]};
  const res=gs(r,'Resources')||{};
  const con=gs(r,'Connections')||{};
  const q=gs(r,'Queries')||{};
  const txn=gs(r,'Transactions')||{};
  const inv=gs(r,'Inventory');
  const col=gc(gn(h,'Score'));
  const mem=gn(res,'TotalMemoryMB')>0?Math.round((1-gn(res,'AvailableMemoryMB')/gn(res,'TotalMemoryMB'))*100):0;
  const cid='sc'+srv.replace(/\W/g,'');

  const pens=ga(h,'Penalties');
  const penH=pens.length?pens.map(p=>'<div style="display:flex;gap:6px;font-size:11px;padding:4px 0;border-bottom:1px solid var(--bg2)"><span style="color:var(--yellow);flex-shrink:0">⚠</span>'+esc(p)+'</div>').join(''):'<div style="color:var(--green);font-size:11px">✓ All health checks passing</div>';

  const waitsH=ga(res,'DiskStats').slice(0,5).map(d=>'<div class="wait-bar-row"><div class="wait-name">'+esc(gt(d,'Database'))+'</div>'
    +'<div style="flex:1;display:flex;align-items:center;gap:6px">'
    +'<span style="font-size:10px;color:'+(gn(d,'AvgReadMS')>50?'var(--red)':'var(--green)')+'">R:'+fN(gn(d,'AvgReadMS'))+'ms</span>'
    +'<span style="font-size:10px;color:'+(gn(d,'AvgWriteMS')>50?'var(--red)':'var(--green)')+'">W:'+fN(gn(d,'AvgWriteMS'))+'ms</span>'
    +'</div></div>').join('')||nd('No disk I/O data');

  const wst=ga(gs(r,'Waits'),'TopWaits').slice(0,5);
  const waitsTabH=wst.length?wst.map(w=>'<div class="wait-bar-row"><div class="wait-name">'+esc(gt(w,'WaitType'))+'</div><div class="wait-cat">'+b(gt(w,'Category'),'bgray')+'</div><div class="wait-bar-wrap"><div class="wait-bar-fill" style="width:'+Math.min(100,gn(w,'PctOfTotal'))+'%;background:'+(gn(w,'PctOfTotal')>30?'var(--red)':gn(w,'PctOfTotal')>15?'var(--yellow)':'var(--blue)')+'"></div></div><div class="wait-pct">'+gn(w,'PctOfTotal').toFixed(0)+'%</div></div>').join(''):nd('No wait data');

  const txnH='<div style="display:grid;grid-template-columns:1fr 1fr;gap:6px;">'
    +miniTile(fN(gn(txn,'TransactionsPerSec')),'TPS','var(--blue)')
    +miniTile(fN(gn(txn,'BatchRequestsPerSec')),'Batch/s','var(--cyan)')
    +miniTile(gn(txn,'ActiveTransactions'),'Active Txns',gn(txn,'ActiveTransactions')>50?'var(--yellow)':'var(--text2)')
    +miniTile(gn(txn,'LongestTxnSec')+'s','Longest Txn',gn(txn,'LongestTxnSec')>30?'var(--red)':'var(--text2)')
    +miniTile(fN(gn(txn,'TempDBUsedMB'))+' MB','TempDB Used','var(--purple)')
    +miniTile(fN(gn(txn,'TempDBFreeMB'))+' MB','TempDB Free','var(--text3)')
    +'</div>';

  const tcid=cid+'_trend';
  const el=document.createElement('div');
  el.className='srv-card';
  el.innerHTML=[
    '<div class="srv-card-head">',
      '<div class="srv-grade" style="border-color:'+col+';color:'+col+'">'+esc(gt(h,'Grade'))+'</div>',
      '<div class="srv-meta"><div class="srv-name">'+esc(srv)+'</div>',
        '<div class="srv-detail">'+(inv?esc(gt(inv,'Edition').replace(/\(64-bit\)/,'').trim())+' v'+esc(gt(inv,'ProductVersion')):'Connecting...')+'</div></div>',
      '<div class="srv-score"><div class="srv-score-val" style="color:'+col+'">'+gn(h,'Score')+'</div>',
        '<div style="font-size:9px;color:var(--text3)">/100</div>',
        '<div class="srv-score-bar"><div class="srv-score-fill" style="width:'+gn(h,'Score')+'%;background:'+col+'"></div></div></div>',
    '</div>',
    '<div class="srv-metrics">',
      srvMetric(gn(res,'SQLCPUPercent')>=0?gn(res,'SQLCPUPercent')+'%':'N/A','CPU',gn(res,'SQLCPUPercent')>80?'var(--red)':gn(res,'SQLCPUPercent')>60?'var(--yellow)':'var(--green)'),
      srvMetric(mem+'%','Memory',mem>90?'var(--red)':mem>80?'var(--yellow)':'var(--purple)'),
      srvMetric(gn(con,'TotalSessions'),'Sessions',gn(con,'TotalSessions')>150?'var(--yellow)':'var(--text2)'),
      srvMetric(gn(con,'BlockedSessions'),'Blocked',gn(con,'BlockedSessions')>0?'var(--red)':'var(--green)'),
      srvMetric(ga(q,'ActiveLongRunning').length,'Slow Q',ga(q,'ActiveLongRunning').length>0?'var(--orange)':'var(--green)'),
      srvMetric(ga(r,'Deadlocks').length,'Deadlocks',ga(r,'Deadlocks').length>0?'var(--red)':'var(--green)'),
    '</div>',
    '<div class="srv-tabs">',
      ['Health','Disk I/O','Wait Types','Transactions','Trend'].map((t,i)=>'<div class="srv-tab'+(i===0?' active':'')+'" onclick="sstab(this,\''+cid+'_p'+i+'\')">'+t+'</div>').join(''),
    '</div>',
    '<div id="'+cid+'_p0" class="srv-pane active">'+penH+'</div>',
    '<div id="'+cid+'_p1" class="srv-pane">'+waitsH+'</div>',
    '<div id="'+cid+'_p2" class="srv-pane">'+waitsTabH+'</div>',
    '<div id="'+cid+'_p3" class="srv-pane">'+txnH+'</div>',
    '<div id="'+cid+'_p4" class="srv-pane" style="height:120px;"><canvas id="'+tcid+'"></canvas></div>',
  ].join('');
  wrap.appendChild(el);

  const hist=H[srv]||[];
  if(hist.length>1) {
    setTimeout(()=>{
      const pts=hist.slice(-20);
      try{mkChart(tcid,'line',{
        labels:pts.map(p=>{try{return new Date(p.ts).toLocaleTimeString();}catch(e){return '';}}),
        datasets:[
          {label:'Health',data:pts.map(p=>p.health||0),borderColor:col,backgroundColor:col+'20',fill:true,tension:.3,pointRadius:0,borderWidth:1.5},
          {label:'CPU%',data:pts.map(p=>p.cpu||0),borderColor:'#f87171',fill:false,tension:.3,pointRadius:0,borderWidth:1,borderDash:[4,3]}
        ]
      },{scales:{y:{min:0,max:100,ticks:{color:'#475569'},grid:{color:'#1f2d4a',lineWidth:.5}},x:{ticks:{color:'#475569',font:{size:9}},grid:{display:false}}}});}catch(e){}
    },80);
  }
}
function srvMetric(v,l,c){return'<div class="srv-metric"><div class="srv-metric-val" style="color:'+c+'">'+v+'</div><div class="srv-metric-lbl">'+l+'</div></div>';}
function sstab(el,id){
  const card=el.closest('.srv-card');
  card.querySelectorAll('.srv-tab').forEach(t=>t.classList.remove('active'));
  card.querySelectorAll('.srv-pane').forEach(p=>p.classList.remove('active'));
  el.classList.add('active');
  const p=document.getElementById(id);if(p)p.classList.add('active');
}

/* ████ PERFORMANCE ████ */
function rPerf() {
  try {
    const keys=Object.keys(H);
    if(!keys.length){return;}
    const labels=(H[keys[0]]||[]).slice(-30).map(p=>{try{return new Date(p.ts).toLocaleTimeString();}catch(e){return '';}});
    const mkL=(id,field)=>mkChart(id,'line',{labels,datasets:keys.map((k,i)=>({label:k,data:(H[k]||[]).slice(-30).map(p=>p[field]||0),borderColor:C[i%C.length],backgroundColor:C[i%C.length]+'15',fill:true,tension:.3,pointRadius:0,borderWidth:1.5}))},{scales:{y:{ticks:{color:'#475569'},grid:{color:'#1f2d4a',lineWidth:.5}},x:{ticks:{color:'#475569',font:{size:9},maxTicksLimit:7},grid:{display:false}}}});
    mkL('ch-p-cpu','cpu');mkL('ch-p-mem','mem');mkL('ch-p-batch','batch_req');mkL('ch-p-txn','txns');mkL('ch-p-block','blocked');
  }catch(e){console.error('perf',e);}
}

/* ████ QUERIES ████ */
function rQueries() {
  try {
    let aq='',sq='',mi='',fi='',tdData=[];
    srvs().forEach(r=>{
      const srv=gt(r,'ServerName');
      const al=ga(gs(r,'Queries'),'ActiveLongRunning');
      al.forEach(q=>tdData.push({srv,sid:gn(q,'SessionID'),ms:gn(q,'ElapsedMS'),wait:gt(q,'WaitType'),db:gt(q,'Database')}));
      if(al.length){
        aq+='<div class="section-label">'+esc(srv)+'</div>'
          +'<table class="dt"><thead><tr><th>Sid</th><th>DB</th><th>Login</th><th>Elapsed ms</th><th>CPU ms</th><th>Wait Type</th><th>Plan Analysis</th></tr></thead><tbody>'
          +al.map(q=>'<tr><td>'+gn(q,'SessionID')+'</td><td>'+esc(gt(q,'Database'))+'</td><td>'+esc(gt(q,'LoginName'))+'</td>'
            +'<td>'+b(fN(gn(q,'ElapsedMS')),gn(q,'ElapsedMS')>30000?'bcrit':'bwarn')+'</td>'
            +'<td>'+fN(gn(q,'CPUTime'))+'</td>'
            +'<td class="mono">'+esc(gt(q,'WaitType')||'—')+'</td>'
            +'<td>'+(gt(q,'PlanAdvice')?'<div class="plan-box">'+esc(gt(q,'PlanAdvice'))+'</div>':'<span style="color:var(--text3);font-size:10px">Capturing plan...</span>')+'</td></tr>').join('')
          +'</tbody></table>';
      }
      const sl=ga(gs(r,'Queries'),'SlowQueries');
      if(sl.length)sq+='<div class="section-label">'+esc(srv)+'</div><table class="dt"><thead><tr><th>DB</th><th>Avg ms</th><th>Executions</th><th>Avg Reads</th><th>Avg CPU ms</th></tr></thead><tbody>'+sl.slice(0,8).map(q=>'<tr><td>'+esc(gt(q,'Database'))+'</td><td>'+b(fN(gn(q,'AvgElapsedMS')),gn(q,'AvgElapsedMS')>10000?'bcrit':'bwarn')+'</td><td>'+fN(gn(q,'ExecutionCount'))+'</td><td>'+fN(gn(q,'AvgLogicalReads'))+'</td><td>'+fN(gn(q,'AvgCPUMs'))+'</td></tr>').join('')+'</tbody></table>';
      const mix=ga(gs(r,'Indexes'),'MissingIndexes');
      if(mix.length)mi+='<div class="section-label">'+esc(srv)+'</div><table class="dt"><thead><tr><th>Table</th><th>DB</th><th>Equality Cols</th><th>Impact</th><th>Seeks</th></tr></thead><tbody>'+mix.slice(0,6).map(x=>'<tr><td><strong>'+esc(gt(x,'TableName'))+'</strong></td><td>'+esc(gt(x,'Database'))+'</td><td class="code">'+esc(gt(x,'EqualityColumns'))+'</td><td>'+b(Math.round(gn(x,'ImpactScore')).toLocaleString(),gn(x,'ImpactScore')>100000?'bcrit':gn(x,'ImpactScore')>10000?'bwarn':'bblue')+'</td><td>'+fN(gn(x,'UserSeeks'))+'</td></tr>').join('')+'</tbody></table>';
      const frix=ga(gs(r,'Indexes'),'FragmentedIndexes');
      if(frix.length)fi+='<div class="section-label">'+esc(srv)+'</div><table class="dt"><thead><tr><th>Index</th><th>Table</th><th>DB</th><th>Frag%</th><th>Pages</th><th>Action</th></tr></thead><tbody>'+frix.slice(0,8).map(x=>'<tr><td class="mono">'+esc(gt(x,'IndexName'))+'</td><td>'+esc(gt(x,'TableName'))+'</td><td>'+esc(gt(x,'Database'))+'</td><td>'+b(gn(x,'FragmentationPct').toFixed(0)+'%',gn(x,'FragmentationPct')>=30?'bcrit':'bwarn')+'</td><td>'+fN(gn(x,'PageCount'))+'</td><td>'+(gt(x,'RecommendedAction')==='REORGANIZE'?'<button onclick="applyFix(\''+esc(srv)+'\',\''+esc(gt(x,'Database'))+'\',\''+esc(gt(x,'TableName'))+'\',\''+esc(gt(x,'IndexName'))+'\')" style="background:var(--green)15;border:1px solid var(--green)30;color:var(--green);border-radius:4px;padding:2px 8px;font-size:10px;cursor:pointer;">Apply Reorganize</button>':'<span style="color:var(--text3);font-size:10px">Run in SSMS</span>')+'</td></tr>').join('')+'</tbody></table>';
    });
    document.getElementById('lrq-sub').textContent=tdData.length?tdData.length+' active':'';
    if(tdData.length){
      mkChart('ch-q-timeline','bar',{labels:tdData.map(d=>'Sid '+d.sid+' ('+d.db+')'),datasets:[{label:'Elapsed ms',data:tdData.map(d=>d.ms),backgroundColor:tdData.map(d=>W_COLORS[d.wait]||'#c084fc'),borderRadius:3}]},{indexAxis:'y',scales:{x:{ticks:{color:'#475569'},grid:{color:'#1f2d4a',lineWidth:.5}},y:{ticks:{color:'#475569',font:{size:9}},grid:{display:false}}},plugins:{legend:{display:false}}});
    } else {
      const el=document.getElementById('ch-q-timeline');if(el){const p=el.closest('.card').querySelector('.card-body');if(p)p.innerHTML='<div style="display:flex;align-items:center;justify-content:center;height:100%;color:var(--green);font-size:12px">✓ No long-running queries detected</div>';}
    }
    document.getElementById('lrq-table').innerHTML=aq||nd('✓ No queries exceeding threshold');
    document.getElementById('slow-q-table').innerHTML=sq||nd('No slow queries in plan cache');
    document.getElementById('miss-idx-table').innerHTML=mi||nd('No missing indexes');
    document.getElementById('frag-idx-table').innerHTML=fi||nd('No fragmented indexes');
  }catch(e){console.error('queries',e);}
}

async function applyFix(server,database,table,indexName){
  try{
    const btn=event.target;btn.textContent='Loading...';btn.disabled=true;
    const res=await fetch('/api/apply-fix',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({server,fix_type:'reorganize_index',database,object:table,index_name:indexName})});
    btn.textContent='Apply Reorganize';btn.disabled=false;
    if(res.ok){const data=await res.json();if(data.script)alert('Copy this to SSMS:\n\n'+data.script+'\n\nNote: '+data.note);}
  }catch(e){const btn=event.target;if(btn){btn.textContent='Apply Reorganize';btn.disabled=false;}alert('Error: '+e.message);}
}

/* ████ WAITS ████ */
function rWaits(){
  try{
    const allW=[];
    const catMap={};
    srvs().forEach(r=>{
      ga(gs(r,'Waits'),'TopWaits').forEach(w=>{
        allW.push({...w,srv:gt(r,'ServerName')});
        const cat=gt(w,'Category')||'Other';
        catMap[cat]=(catMap[cat]||0)+gn(w,'WaitTimeSec');
      });
    });
    const catE=Object.entries(catMap).sort((a,b)=>b[1]-a[1]).slice(0,8);
    if(catE.length){
      mkChart('ch-wait-pie','doughnut',{labels:catE.map(([k])=>k),datasets:[{data:catE.map(([,v])=>v),backgroundColor:C,borderWidth:0,hoverOffset:4}]},{plugins:{legend:{labels:{color:'#94a3b8',font:{size:10}}}}});
    }
    const srvNames=srvs().map(r=>gt(r,'ServerName'));
    if(srvNames.length){
      const topWaits=Array.from(new Set(allW.map(w=>gt(w,'WaitType')))).slice(0,6);
      mkChart('ch-wait-bar','bar',{
        labels:srvNames,
        datasets:topWaits.map((wt,i)=>({label:wt,data:srvNames.map(s=>{const w=allW.find(x=>gt(x,'WaitType')===wt&&x.srv===s);return w?gn(w,'WaitTimeSec'):0;}),backgroundColor:C[i%C.length],borderRadius:3}))
      },{scales:{x:{stacked:true,ticks:{color:'#475569'},grid:{display:false}},y:{stacked:true,ticks:{color:'#475569'},grid:{color:'#1f2d4a',lineWidth:.5}}},plugins:{legend:{labels:{color:'#94a3b8',font:{size:9}}}}});
    }
    let detail='';
    srvs().forEach(r=>{
      const ws=ga(gs(r,'Waits'),'TopWaits');if(!ws.length)return;
      const tot=ws.reduce((a,w)=>a+gn(w,'WaitTimeSec'),0);
      detail+='<div class="section-label">'+esc(gt(r,'ServerName'))+'</div>'
        +ws.map(w=>'<div class="wait-bar-row"><div class="wait-name">'+esc(gt(w,'WaitType'))+'</div>'
          +'<div class="wait-cat">'+b(gt(w,'Category'),'bgray')+'</div>'
          +'<div class="wait-bar-wrap"><div class="wait-bar-fill" style="width:'+Math.min(100,gn(w,'PctOfTotal'))+'%;background:'+(gn(w,'PctOfTotal')>30?'var(--red)':gn(w,'PctOfTotal')>15?'var(--yellow)':'var(--blue)')+'"></div></div>'
          +'<div style="font-size:10px;color:var(--text2);min-width:60px;text-align:right">'+fN(gn(w,'WaitTimeSec'))+'s</div>'
          +'<div class="wait-pct">'+gn(w,'PctOfTotal').toFixed(1)+'%</div></div>').join('');
    });
    document.getElementById('wait-detail').innerHTML=detail||nd();
  }catch(e){console.error('waits',e);}
}

/* ████ BACKUPS ████ */
function rBackups(){
  try{
    let total=0,ok=0,over=0,logOver=0,rows='';
    srvs().forEach(r=>ga(gs(r,'Backups'),'Databases').forEach(d=>{
      total++;
      if(d.IsAlertFull)over++;else if(d.IsAlertLog)logOver++;else ok++;
      rows+='<tr><td>'+esc(gt(r,'ServerName'))+'</td><td><strong>'+esc(gt(d,'DatabaseName'))+'</strong></td>'
        +'<td>'+b(gt(d,'RecoveryModel'),'bgray')+'</td>'
        +'<td>'+(d.LastFullBackup?fD(d.LastFullBackup):b('Never','bcrit'))+'</td>'
        +'<td>'+(d.LastLogBackup?fD(d.LastLogBackup):'—')+'</td>'
        +'<td>'+fN(gn(d,'SizeMB'))+'</td>'
        +'<td>'+(d.IsAlertFull?b('OVERDUE','bcrit'):d.IsAlertLog?b('LOG DUE','bwarn'):b('OK','bok'))+'</td></tr>';
    }));
    document.getElementById('bak-kpi-row').innerHTML=
      kpiTile(total,'DATABASES','var(--blue)','Monitored')+
      kpiTile(ok,'BACKUPS OK','var(--green)','Within threshold')+
      kpiTile(over,'FULL OVERDUE',over?'var(--red)':'var(--green)','Need attention')+
      kpiTile(logOver,'LOG OVERDUE',logOver?'var(--yellow)':'var(--green)','FULL recovery model');
    document.getElementById('bak-table').innerHTML=rows?'<table class="dt"><thead><tr><th>Server</th><th>Database</th><th>Recovery</th><th>Last Full</th><th>Last Log</th><th>Size MB</th><th>Status</th></tr></thead><tbody>'+rows+'</tbody></table>':nd('Backup data loads on first slow cycle (30 min)');
  }catch(e){console.error('backups',e);}
}

/* ████ JOBS ████ */
function rJobs(){
  try{
    let failed=0,running=0,fjRows='',ljRows='';
    srvs().forEach(r=>{
      const jobs=gs(r,'Jobs');
      if(!jobs)return;
      ga(jobs,'FailedJobs').forEach(j=>{
        failed++;
        fjRows+='<tr><td>'+esc(gt(r,'ServerName'))+'</td><td><strong>'+esc(gt(j,'JobName'))+'</strong></td><td>'+esc(gt(j,'StepName'))+'</td><td>'+fD(gs(j,'LastRunTime'))+'</td><td class="code" style="max-width:200px">'+esc(gt(j,'Message').substring(0,100))+'</td></tr>';
      });
      ga(jobs,'LongRunJobs').forEach(j=>{
        running++;
        ljRows+='<tr><td>'+esc(gt(r,'ServerName'))+'</td><td><strong>'+esc(gt(j,'JobName'))+'</strong></td><td>'+b(gn(j,'ElapsedMinutes')+'m','bwarn')+'</td></tr>';
      });
    });
    document.getElementById('job-kpi-row').innerHTML=
      kpiTile(failed,'FAILED JOBS',failed?'var(--red)':'var(--green)','Last 2 hours')+
      kpiTile(running,'LONG RUNNING',running?'var(--yellow)':'var(--green)','>30 minutes')+
      kpiTile(failed+running,'TOTAL ISSUES',failed+running?'var(--orange)':'var(--green)','Needs attention');
    document.getElementById('failed-jobs-table').innerHTML=fjRows?'<table class="dt"><thead><tr><th>Server</th><th>Job</th><th>Step</th><th>Time</th><th>Error</th></tr></thead><tbody>'+fjRows+'</tbody></table>':nd('No job failures in lookback window ✓');
    document.getElementById('long-jobs-table').innerHTML=ljRows?'<table class="dt"><thead><tr><th>Server</th><th>Job</th><th>Elapsed</th></tr></thead><tbody>'+ljRows+'</tbody></table>':nd('No long-running jobs ✓');
  }catch(e){console.error('jobs',e);}
}

/* ████ SECURITY ████ */
function rSecurity(){
  try{
    let riskC=0,privC=0,lsC=0,risks='',privs='',linked='',susp='';
    srvs().forEach(r=>{
      const sec=gs(r,'Security');if(!sec)return;
      if(sec.XPCmdShellOn||sec.SALoginEnabled)riskC++;
      privC+=ga(sec,'PrivilegedUsers').length;
      lsC+=ga(sec,'LinkedServers').length;
      const checks=[{l:'xp_cmdshell',ok:!sec.XPCmdShellOn,t:sec.XPCmdShellOn?'ENABLED — OS command risk':'Disabled'},{l:'SA login',ok:!sec.SALoginEnabled,t:sec.SALoginEnabled?'Enabled — rename/disable':'Disabled'},{l:'Auth mode',ok:!sec.MixedAuthMode,t:sec.MixedAuthMode?'Mixed (SQL+Windows)':'Windows only'}];
      risks+='<div class="section-label">'+esc(gt(r,'ServerName'))+'</div>'+checks.map(c=>'<div style="display:flex;align-items:center;gap:10px;padding:6px 0;border-bottom:1px solid var(--bg2);font-size:12px"><div style="width:8px;height:8px;border-radius:50%;background:'+(c.ok?'var(--green)':'var(--red)')+';flex-shrink:0"></div><strong style="min-width:90px">'+c.l+'</strong><span style="color:var(--text2)">'+c.t+'</span></div>').join('');
      ga(sec,'PrivilegedUsers').forEach(u=>privs+='<tr><td>'+esc(gt(r,'ServerName'))+'</td><td><strong>'+esc(gt(u,'LoginName'))+'</strong></td><td>'+b(gt(u,'Role'),gt(u,'Role')==='sysadmin'?'bcrit':'bwarn')+'</td><td>'+(u.IsDisabled?b('Disabled','bcrit'):b('Active','bok'))+'</td></tr>');
      ga(sec,'LinkedServers').forEach(ls=>linked+='<tr><td>'+esc(gt(r,'ServerName'))+'</td><td><strong>'+esc(gt(ls,'Name'))+'</strong></td><td>'+esc(gt(ls,'Provider'))+'</td><td>'+esc(gt(ls,'DataSource'))+'</td><td>'+(ls.IsRemoteLogin?b('Remote','bwarn'):b('Self','bok'))+'</td></tr>');
      ga(sec,'SuspiciousConns').forEach(sc=>susp+='<tr><td>'+esc(gt(r,'ServerName'))+'</td><td>'+gn(sc,'SessionID')+'</td><td>'+esc(gt(sc,'LoginName'))+'</td><td>'+esc(gt(sc,'HostName'))+'</td><td>'+b(gt(sc,'ProgramName')||'Unknown','bwarn')+'</td></tr>');
    });
    document.getElementById('sec-kpi-row').innerHTML=kpiTile(riskC,'CONFIG RISKS',riskC?'var(--red)':'var(--green)','Servers affected')+kpiTile(privC,'PRIVILEGED',privC>5?'var(--yellow)':'var(--green)','sysadmin+')+kpiTile(lsC,'LINKED SERVERS','var(--blue)','Potential risk')+kpiTile(0,'FAILED LOGINS','var(--text3)','System health XE');
    document.getElementById('sec-risks').innerHTML=risks||nd();
    document.getElementById('priv-users-table').innerHTML=privs?'<table class="dt"><thead><tr><th>Server</th><th>Login</th><th>Role</th><th>Status</th></tr></thead><tbody>'+privs+'</tbody></table>':nd();
    document.getElementById('linked-srv-table').innerHTML=linked?'<table class="dt"><thead><tr><th>Server</th><th>Name</th><th>Provider</th><th>Source</th><th>Auth</th></tr></thead><tbody>'+linked+'</tbody></table>':nd('No linked servers');
    document.getElementById('susp-conn-table').innerHTML=susp?'<table class="dt"><thead><tr><th>Server</th><th>Sid</th><th>Login</th><th>Host</th><th>Program</th></tr></thead><tbody>'+susp+'</tbody></table>':nd('No suspicious connections');
  }catch(e){console.error('security',e);}
}

/* ████ CAPACITY ████ */
function rCapacity(){
  try{
    const all=srvs().filter(s=>gs(s,'Capacity'));
    if(!all.length){document.getElementById('cap-disk-table').innerHTML=nd('Capacity data loads on first slow cycle (30 min)');return;}
    const ds=all.map((r,i)=>{
      const cap=gs(r,'Capacity'),res=gs(r,'Resources'),con=gs(r,'Connections');
      const mf=gn(res,'TotalMemoryMB')>0?gn(res,'AvailableMemoryMB')/gn(res,'TotalMemoryMB')*100:50;
      const da=ga(cap,'DiskTrend').reduce((a,d,_,arr)=>a+(gn(d,'UtilisationPct')||0)/arr.length,0)||50;
      const ch=gn(con,'TotalSessions')>0?Math.max(0,100-gn(con,'TotalSessions')/200*100):80;
      return{label:gt(r,'ServerName'),data:[Math.max(0,100-gn(res,'SQLCPUPercent')),mf,Math.max(0,100-da),ch,gn(gs(r,'Health'),'Score')],borderColor:C[i%C.length],backgroundColor:C[i%C.length]+'25',pointBackgroundColor:C[i%C.length]};
    });
    mkChart('ch-radar','radar',{labels:['CPU Room','Mem Free','Disk Free','Conn Room','Health'],datasets:ds},{scales:{r:{min:0,max:100,ticks:{color:'#475569',backdropColor:'transparent',font:{size:9}},grid:{color:'#1f2d4a'},pointLabels:{color:'#94a3b8',font:{size:10}}}},plugins:{legend:{labels:{color:'#94a3b8',font:{size:10}}}}});
    let rows='';
    all.forEach(r=>ga(gs(r,'Capacity'),'DiskTrend').forEach(d=>{
      const duf=gn(d,'DaysUntilFull')>=9999?'∞':gn(d,'DaysUntilFull')+'d';
      rows+='<tr><td>'+esc(gt(r,'ServerName'))+'</td><td><strong>'+esc(gt(d,'DatabaseName'))+'</strong></td><td>'+fN(gn(d,'CurrentSizeMB'))+'</td><td>'+fN(gn(d,'MaxSizeMB'))+'</td>'
        +'<td><div class="progress-row"><div class="prog" style="flex:1"><div class="prog-fill" style="width:'+Math.min(100,gn(d,'UtilisationPct'))+'%;background:'+(gn(d,'UtilisationPct')>85?'var(--red)':gn(d,'UtilisationPct')>70?'var(--yellow)':'var(--green)')+'"></div></div><span style="font-size:10px;min-width:35px;text-align:right">'+gn(d,'UtilisationPct').toFixed(0)+'%</span></div></td>'
        +'<td>'+b(duf,gn(d,'DaysUntilFull')<30?'bcrit':gn(d,'DaysUntilFull')<90?'bwarn':'bok')+'</td></tr>';
    }));
    document.getElementById('cap-disk-table').innerHTML=rows?'<table class="dt"><thead><tr><th>Server</th><th>Database</th><th>Current MB</th><th>Max MB</th><th>Used %</th><th>Days Until Full*</th></tr></thead><tbody>'+rows+'</tbody></table><div style="font-size:10px;color:var(--text3);margin-top:6px">* Estimated 3%/week growth rate</div>':nd();
  }catch(e){console.error('capacity',e);}
}

/* ████ INVENTORY ████ */
function rInventory(){
  try{
    let sr='',dr='';
    srvs().forEach(r=>{
      const inv=gs(r,'Inventory');
      sr+='<tr><td><strong>'+esc(gt(r,'ServerName'))+'</strong></td>'
        +'<td class="mono">'+(inv?esc(gt(inv,'ProductVersion')):'<span style="color:var(--text3)">Collecting...</span>')+'</td>'
        +'<td>'+(inv?esc(gt(inv,'Edition').replace(/\(64-bit\)/,'').trim()):'')+'</td>'
        +'<td>'+(inv?gn(inv,'ProcessorCount'):'')+'</td>'
        +'<td>'+(inv&&gn(inv,'TotalMemoryMB')?Math.round(gn(inv,'TotalMemoryMB')/1024)+' GB':'—')+'</td>'
        +'<td>'+(inv&&gn(inv,'UptimeDays')?gn(inv,'UptimeDays').toFixed(1)+'d':'—')+'</td>'
        +'<td>'+(inv?gn(inv,'DatabaseCount'):'')+'</td>'
        +'<td>'+(inv&&inv.IsHADREnabled?b('AG','bpurple'):'')+(inv&&inv.IsClustered?b('Cluster','bblue'):'')+(inv&&!inv.IsHADREnabled&&!inv.IsClustered?b('Standalone','bok'):'')+'</td></tr>';
      ga(inv,'Databases').forEach(d=>{
        dr+='<tr><td>'+esc(gt(r,'ServerName'))+'</td><td><strong>'+esc(gt(d,'Name'))+'</strong></td>'
          +'<td>'+b(gt(d,'State'),gt(d,'State')==='ONLINE'?'bok':'bcrit')+'</td>'
          +'<td>'+b(gt(d,'RecoveryModel'),'bgray')+'</td>'
          +'<td>'+gn(d,'CompatLevel')+'</td>'
          +'<td>'+fN(gn(d,'SizeMB'))+'</td>'
          +'<td>'+(d.IsReadOnly?b('Read Only','bwarn'):'—')+'</td></tr>';
      });
    });
    document.getElementById('inv-srv-table').innerHTML=sr?'<table class="dt"><thead><tr><th>Server</th><th>Version</th><th>Edition</th><th>CPUs</th><th>RAM</th><th>Uptime</th><th>DBs</th><th>Features</th></tr></thead><tbody>'+sr+'</tbody></table>':nd();
    document.getElementById('inv-db-table').innerHTML=dr?'<table class="dt"><thead><tr><th>Server</th><th>Database</th><th>State</th><th>Recovery</th><th>Compat</th><th>Size MB</th><th>Read Only</th></tr></thead><tbody>'+dr+'</tbody></table>':nd('Database list loads after first collection');
  }catch(e){console.error('inventory',e);}
}

/* ████ ADVISOR ████ */
const SV2C={CRITICAL:'var(--red)',HIGH:'var(--yellow)',MEDIUM:'var(--blue)',LOW:'var(--green)'};
const SV2BG={CRITICAL:'var(--red)0a',HIGH:'var(--yellow)0a',MEDIUM:'var(--blue)0a',LOW:'var(--green)0a'};
const SV2BD={CRITICAL:'var(--red)25',HIGH:'var(--yellow)25',MEDIUM:'var(--blue)25',LOW:'var(--green)25'};

async function rAdvisor(){
  try{
    try{const r=await fetch('/api/advisories');if(r.ok)advData=await r.json();}catch(e){}
    const adv=Array.isArray(advData)?advData:[];
    const crit=adv.filter(a=>a.severity==='CRITICAL').length;
    const high=adv.filter(a=>a.severity==='HIGH').length;
    const med=adv.filter(a=>a.severity==='MEDIUM').length;
    const low=adv.filter(a=>a.severity==='LOW').length;
    document.getElementById('adv-kpi-row').innerHTML=
      kpiTile(crit,'CRITICAL',crit?'var(--red)':'var(--green)','Immediate')+
      kpiTile(high,'HIGH',high?'var(--yellow)':'var(--green)','Fix soon')+
      kpiTile(med,'MEDIUM','var(--blue)','This week')+
      kpiTile(low,'LOW','var(--green)','Best practice')+
      kpiTile(adv.length,'TOTAL','var(--purple)','All servers')+
      kpiTile(adv.filter(a=>a.safe).length,'AUTO-FIXABLE','var(--cyan)','Safe to script');

    // Update nav badge
    const badge=document.getElementById('badge-adv');
    if(badge){if(crit+high>0){badge.textContent=crit+high;badge.style.display='';}else{badge.style.display='none';}}

    const cats={};
    adv.forEach(a=>{const c=a.category||'Other';cats[c]=(cats[c]||0)+1;});
    const catE=Object.entries(cats).sort((a,b)=>b[1]-a[1]);
    if(catE.length){
      mkChart('ch-adv-dist','doughnut',{labels:catE.map(([k])=>k),datasets:[{data:catE.map(([,v])=>v),backgroundColor:C,borderWidth:0,hoverOffset:4}]},{plugins:{legend:{display:false}}});
      mkChart('ch-adv-cat','bar',{labels:catE.map(([k])=>k),datasets:[{label:'Count',data:catE.map(([,v])=>v),backgroundColor:C,borderRadius:3}]},{indexAxis:'y',scales:{x:{ticks:{color:'#475569'},grid:{color:'#1f2d4a',lineWidth:.5}},y:{ticks:{color:'#475569',font:{size:9}},grid:{display:false}}},plugins:{legend:{display:false}}});
    }

    if(!adv.length){
      document.getElementById('adv-list').innerHTML='<div style="text-align:center;padding:60px;background:var(--bg1);border:1px solid var(--border);border-radius:8px;"><div style="font-size:2.5rem;margin-bottom:12px">✅</div><div style="font-size:14px;color:var(--green);font-weight:700">No advisories — all systems healthy</div><div style="font-size:12px;color:var(--text3);margin-top:6px">Advisories appear automatically when issues are detected</div></div>';
      return;
    }

    document.getElementById('adv-list').innerHTML=adv.map((a,i)=>{
      const c=SV2C[a.severity]||'var(--text3)';
      const sid='as'+i,did='ad'+i;
      return '<div class="adv-item" style="background:'+SV2BG[a.severity]+';border-color:'+SV2BD[a.severity]+'">'
        +'<div class="adv-head" onclick="toggleAdv(\''+did+'\',this)">'
          +'<span class="adv-sev" style="background:'+c+'20;color:'+c+';border:1px solid '+c+'30">'+esc(a.severity||'')+'</span>'
          +'<div style="flex:1"><div class="adv-title">'+esc(a.title||'')+'</div>'
            +'<div class="adv-meta-row">['+esc(a.id)+'] · '+esc(a.category||'')+' · '+esc(a.server||'')+(a.database?' / '+esc(a.database):'')+(a.metric?' · <span style="color:var(--text3)">'+esc(a.metric)+'</span>':'')+'</div></div>'
          +'<span class="adv-chevron" id="chv'+i+'">▼</span>'
        +'</div>'
        +'<div style="padding:8px 14px 10px;display:grid;grid-template-columns:1fr 1fr;gap:12px;border-top:1px solid '+SV2BD[a.severity]+'">'
          +'<div><div class="adv-section-label">What\'s happening</div><div style="font-size:11px;color:var(--text2);line-height:1.5">'+esc(a.what||'')+'</div></div>'
          +'<div><div class="adv-section-label">Why it matters</div><div style="font-size:11px;color:var(--text2);line-height:1.5">'+esc(a.why||'')+'</div></div>'
        +'</div>'
        +'<div id="'+did+'" class="adv-body">'
          +'<div class="adv-grid"><div><div class="adv-section-label">🔍 Root Cause</div><div class="adv-section-text">'+esc(a.cause||'')+'</div></div>'
            +'<div><div class="adv-section-label">🔧 How to Fix</div><div class="adv-section-text" style="white-space:pre-line">'+esc(a.fix||'')+'</div></div></div>'
          +(a.impact?'<div class="adv-impact">📈 '+esc(a.impact)+'</div>':'')
          +(a.sql?'<div class="adv-sql-wrap"><div class="adv-sql-header"><div class="adv-section-label">💻 Fix Script (T-SQL)</div><button class="adv-sql-copy" onclick="cpSQL(\''+sid+'\',this)">📋 Copy SQL</button></div><pre id="'+sid+'" class="adv-sql-pre">'+esc(a.sql)+'</pre></div>':'')
        +'</div>'
      +'</div>';
    }).join('');
  }catch(e){console.error('advisor',e);}
}
function toggleAdv(id,el){
  const body=document.getElementById(id);if(!body)return;
  const open=body.classList.toggle('open');
  const chv=el.querySelector('.adv-chevron');
  if(chv)chv.classList.toggle('open',open);
}
function cpSQL(id,btn){
  const el=document.getElementById(id);if(!el)return;
  navigator.clipboard.writeText(el.textContent).then(()=>{
    btn.textContent='✅ Copied!';setTimeout(()=>btn.textContent='📋 Copy SQL',2000);
  }).catch(()=>{btn.textContent='❌ Failed';setTimeout(()=>btn.textContent='📋 Copy SQL',2000);});
}

/* ████ ALERTS ████ */
function rAlerts(){
  try{
    const types={};alerts.forEach(a=>{const k=a.sev==='crit'?'Critical':'Warning';types[k]=(types[k]||0)+1;});
    const te=Object.entries(types);const tc={Critical:'#f87171',Warning:'#fbbf24'};
    if(te.length)mkChart('ch-alert-pie','doughnut',{labels:te.map(([k])=>k),datasets:[{data:te.map(([,v])=>v),backgroundColor:te.map(([k])=>tc[k]),borderWidth:0,hoverOffset:4}]},{plugins:{legend:{labels:{color:'#94a3b8',font:{size:10}}}}});
    const bySrv={};alerts.forEach(a=>{bySrv[a.srv]=(bySrv[a.srv]||0)+1;});
    document.getElementById('alert-by-srv').innerHTML=Object.entries(bySrv).length?'<table class="dt"><thead><tr><th>Server</th><th>Alert Count</th></tr></thead><tbody>'+Object.entries(bySrv).map(([s,c])=>'<tr><td>'+esc(s)+'</td><td>'+b(c,'bcrit')+'</td></tr>').join('')+'</tbody></table>':nd('No alerts yet');
    const crit=alerts.filter(a=>a.sev==='crit').length;
    const warn=alerts.filter(a=>a.sev==='warn').length;
    document.getElementById('alert-summary-nums').innerHTML=
      miniTile(crit,'Critical',crit?'var(--red)':'var(--green)')+miniTile(warn,'Warnings',warn?'var(--yellow)':'var(--green)')+miniTile(alerts.length,'Total','var(--blue)')+miniTile(Object.keys(bySrv).length,'Servers','var(--text2)');
    document.getElementById('alert-log-full').innerHTML=alerts.map(alertHTML).join('')||nd('No alerts recorded yet');
  }catch(e){console.error('alerts',e);}
}

/* ████ SETTINGS ████ */
function setRefresh(ms){
  refreshInterval=ms;countdown=ms/1000;
  clearInterval(refreshTimer);clearInterval(countdownTimer);
  startTimers();
  document.querySelectorAll('[id^="ri-"]').forEach(b=>b.style.color='');
  const btn=document.getElementById('ri-'+ms/1000);if(btn)btn.style.color='var(--accent)';
}
async function testAPI(url){
  const el=document.getElementById('api-test-result');
  el.textContent='Testing '+url+'...';
  try{const r=await fetch(url);const d=await r.json();el.textContent='✅ '+url+' — OK ('+JSON.stringify(d).length+' bytes)';}
  catch(e){el.textContent='❌ '+url+' — '+e.message;}
}

/* ═══════════════════════════════════════════
   REFRESH LOOP — independent endpoint fetches
   ═══════════════════════════════════════════ */
function startTimers(){
  refreshTimer=setInterval(refresh,refreshInterval);
  countdownTimer=setInterval(()=>{
    countdown=Math.max(0,countdown-1);
    const el=document.getElementById('refresh-timer');
    if(el)el.textContent='⟳ '+countdown+'s';
    if(countdown===0)countdown=refreshInterval/1000;
  },1000);
}

async function refresh(){
  try{
    const [rr,hr]=await Promise.allSettled([
      fetch('/api/results').then(r=>r.json()),
      fetch('/api/history').then(r=>r.json())
    ]);

    const dot=document.getElementById('live-dot');
    const stat=document.getElementById('live-status');

    if(rr.status==='fulfilled'&&rr.value){
      R=rr.value;
      collectAlerts();
      if(dot)dot.className='live-dot ok';
      if(stat)stat.textContent='Live';
    }else{
      if(dot)dot.className='live-dot err';
      if(stat)stat.textContent='Error';
    }
    if(hr.status==='fulfilled'&&hr.value)H=hr.value;

    rTopbar();

    // Re-render active page
    const active=document.querySelector('.nav-item.active');
    if(active){
      const m=active.getAttribute('onclick');
      const match=m&&m.match(/'(\w+)'/);
      if(match)try{renderPage(match[1]);}catch(e){console.error('render error:',e);}
    }

    const el=document.getElementById('last-update');
    if(el)el.textContent=new Date().toLocaleTimeString();
    countdown=refreshInterval/1000;
  }catch(e){
    const dot=document.getElementById('live-dot');
    if(dot)dot.className='live-dot err';
    console.error('refresh error:',e);
  }
}

// Start
refresh();
startTimers();
</script>
</body>
</html>`

const EVENTS = new Set(["app_started", "reader_scan_clicked", "card_scan_clicked", "read_clicked", "write_clicked", "dump_clicked", "key_recovery_clicked", "settings_opened", "about_opened", "update_check_clicked"]);
const OSES = new Set(["windows", "macos", "linux", "other"]);
const UUID_V4 = /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i;
const json = (body, status = 200) => new Response(JSON.stringify(body), { status, headers: { "content-type": "application/json", "cache-control": "no-store" } });

export default {
  async fetch(request, env) {
    const url = new URL(request.url);
    if (request.method === "POST" && url.pathname === "/api/telemetry") return ingest(request, env);
    if (request.method === "GET" && url.pathname === "/dashboard") return dashboard(request, env);
    return new Response("Not found", { status: 404 });
  },
};

async function ingest(request, env) {
  if (!request.headers.get("content-type")?.toLowerCase().startsWith("application/json")) return new Response("Bad request", { status: 400 });
  const size = Number(request.headers.get("content-length") || 0);
  if (size > 1024) return new Response("Bad request", { status: 400 });
  let payload;
  try { payload = await request.json(); } catch { return new Response("Bad request", { status: 400 }); }
  const { installation_id, version, os, event } = payload || {};
  if (!UUID_V4.test(installation_id || "") || typeof version !== "string" || version.length < 1 || version.length > 64 || !OSES.has(os) || !EVENTS.has(event)) return new Response("Bad request", { status: 400 });
  // Deliberately select fields instead of persisting the body, headers, IP, or client timestamp.
  await env.DB.prepare("INSERT INTO telemetry_events (installation_id, event, app_version, os) VALUES (?, ?, ?, ?)").bind(installation_id, event, version, os).run();
  return new Response(null, { status: 204 });
}

async function dashboard(request, env) {
  if (!env.TELEMETRY_DASHBOARD_SECRET || new URL(request.url).searchParams.get("secret") !== env.TELEMETRY_DASHBOARD_SECRET) return new Response("Forbidden", { status: 403 });
  const db = env.DB;
  const [installations, active, today, total, os, versions, features, daily] = await Promise.all([
    db.prepare("SELECT COUNT(DISTINCT installation_id) AS count FROM telemetry_events").first(),
    db.prepare("SELECT COUNT(DISTINCT installation_id) AS count FROM telemetry_events WHERE created_at >= datetime('now', '-30 days')").first(),
    db.prepare("SELECT COUNT(*) AS count FROM telemetry_events WHERE date(created_at) = date('now')").first(),
    db.prepare("SELECT COUNT(*) AS count FROM telemetry_events").first(),
    db.prepare("SELECT os, COUNT(*) AS count FROM telemetry_events GROUP BY os ORDER BY count DESC").all(),
    db.prepare("SELECT app_version AS version, COUNT(*) AS count FROM telemetry_events GROUP BY app_version ORDER BY count DESC").all(),
    db.prepare("SELECT event, COUNT(*) AS count FROM telemetry_events GROUP BY event ORDER BY count DESC").all(),
    db.prepare("SELECT date(created_at) AS day, COUNT(*) AS count, COUNT(DISTINCT installation_id) AS active FROM telemetry_events GROUP BY day ORDER BY day DESC LIMIT 30").all(),
  ]);
  const data = { installations: installations.count, active: active.count, today: today.count, total: total.count, os: os.results, versions: versions.results, features: features.results, daily: daily.results.reverse() };
  return new Response(page(data), { headers: { "content-type": "text/html; charset=utf-8", "cache-control": "no-store" } });
}

function page(data) { return `<!doctype html><meta name="viewport" content="width=device-width,initial-scale=1"><title>NFCX telemetry</title><style>body{font:15px system-ui;margin:2rem;max-width:900px;background:#101827;color:#e6edf7}.cards{display:grid;grid-template-columns:repeat(4,1fr);gap:1rem}.card,section{background:#182231;padding:1rem;border-radius:8px}.n{font-size:2rem}section{margin-top:1rem}pre{white-space:pre-wrap}@media(max-width:600px){.cards{grid-template-columns:repeat(2,1fr)}}</style><h1>NFCX telemetry</h1><div class="cards">${[["Installations",data.installations],["Active (30d)",data.active],["Events today",data.today],["Total events",data.total]].map(([n,v])=>`<div class=card>${n}<div class=n>${v}</div></div>`).join("")}</div><section><h2>OS distribution</h2><pre>${escape(JSON.stringify(data.os,null,2))}</pre></section><section><h2>Version distribution</h2><pre>${escape(JSON.stringify(data.versions,null,2))}</pre></section><section><h2>Feature usage</h2><pre>${escape(JSON.stringify(data.features,null,2))}</pre></section><section><h2>Daily usage</h2><pre>${escape(JSON.stringify(data.daily,null,2))}</pre></section>`; }
function escape(value) { return value.replace(/[&<>]/g, c => ({ "&":"&amp;", "<":"&lt;", ">":"&gt;" })[c]); }

(() => {
  "use strict";

  const byId = (id) => document.getElementById(id);
  const text = (id, value) => { const node = byId(id); if (node) node.textContent = value; };
  const formatInteger = (value) => new Intl.NumberFormat().format(Number(value || 0));
  const formatBytes = (value) => {
    let bytes = Number(value || 0);
    const units = ["B", "KB", "MB", "GB", "TB"];
    let index = 0;
    while (bytes >= 1024 && index < units.length - 1) { bytes /= 1024; index += 1; }
    const digits = index === 0 ? 0 : bytes < 10 ? 1 : 0;
    return `${bytes.toFixed(digits)} ${units[index]}`;
  };
  const formatDuration = (seconds) => {
    const value = Number(seconds || 0);
    if (value < 60) return `${value}s`;
    if (value < 3600) return `${Math.floor(value / 60)}m ${value % 60}s`;
    return `${Math.floor(value / 3600)}h ${Math.floor((value % 3600) / 60)}m`;
  };
  const formatTime = (value) => value ? new Intl.DateTimeFormat(undefined, { hour: "2-digit", minute: "2-digit", second: "2-digit" }).format(new Date(value)) : "not checked";

  let lastObserved = null;
  let request = null;

  const setDatabaseState = (upstream) => {
    const state = byId("database-state");
    state.className = `state-label ${upstream.status === "ok" ? "ok" : upstream.status === "error" ? "error" : "checking"}`;
    state.textContent = upstream.status;
    text("db-address", upstream.address || "—");
    text("db-checked", upstream.checked_at ? `checked ${formatTime(upstream.checked_at)}` : "not checked");
    text("upstream-latency", upstream.status === "ok" ? `${Number(upstream.latency_ms).toFixed(2)} ms` : "—");
    const metadata = upstream.metadata || {};
    text("db-server", metadata.version ? `${metadata.vendor || "server"} ${metadata.version}` : "—");
    text("db-comment", metadata.version_comment || "—");
    text("db-charset", metadata.character_set || "—");
    text("db-collation", metadata.collation || "—");
    text("db-isolation", metadata.transaction_isolation || "—");
    text("db-autocommit", metadata.version ? (metadata.autocommit ? "autocommit on" : "autocommit off") : "—");
    text("db-sql-mode", metadata.sql_mode || "—");
    const error = byId("database-error");
    error.hidden = !upstream.error;
    error.textContent = upstream.error || "";
  };

  const render = (data) => {
    lastObserved = new Date(data.observed_at);
    const healthy = data.lifecycle.state === "ready" && data.upstream.status === "ok";
    const healthDot = byId("health-dot");
    healthDot.className = `status-dot ${healthy ? "ok" : data.upstream.status === "error" ? "error" : "checking"}`;
    text("health-label", healthy ? "Runtime ready" : data.upstream.status === "error" ? "Database degraded" : "Runtime checking");
    text("health-detail", healthy ? `${data.upstream.metadata.vendor} ${data.upstream.metadata.version}` : data.upstream.error || data.lifecycle.reason);
    text("relay-address", data.relay.listen_address);
    text("relay-mode", data.relay.mode.replaceAll("-", " "));

    text("logical-clients", formatInteger(data.pressure.logical_clients));
    text("waiting-work", formatInteger(data.pressure.waiting_work));
    text("active-pool", formatInteger(data.pressure.active_pool));
    text("pinned-work", formatInteger(data.pressure.pinned_work));
    text("database-links", formatInteger(data.pressure.database_links));
    text("safe-limit", formatInteger(data.pressure.safe_limit));
    text("pressure-percent", `${Number(data.pressure.percent).toFixed(1)}%`);
    const progress = byId("pressure-progress");
    progress.value = data.pressure.percent;
    progress.textContent = `${Number(data.pressure.percent).toFixed(1)}%`;
    text("pressure-summary", `${data.pressure.logical_clients} logical clients, ${data.pressure.waiting_work} waiting, ${data.pressure.database_links} database links, ${Number(data.pressure.percent).toFixed(1)} percent of the safe limit.`);
    text("constraint-title", data.pressure.dominant_constraint);
    text("safe-action", data.pressure.safe_action);
    text("acceleration-reason", data.acceleration.reason);

    text("accepted-total", formatInteger(data.relay.accepted_total));
    text("rejected-total", formatInteger(data.relay.rejected_total));
    text("bytes-up", formatBytes(data.relay.client_to_db_bytes));
    text("bytes-down", formatBytes(data.relay.db_to_client_bytes));
    text("active-now", formatInteger(data.relay.active));
    text("dial-errors", formatInteger(data.relay.dial_errors_total));
    text("relay-errors", formatInteger(data.relay.relay_errors_total));
    text("uptime", formatDuration(data.uptime_seconds));

    setDatabaseState(data.upstream);
    text("build-short", `v${data.build.version}`);
    text("build-full", `${data.build.version} · ${data.build.commit} · ${data.build.go_version}`);
    byId("offline-banner").hidden = true;
  };

  const updateAge = () => {
    if (!lastObserved) return;
    const age = Math.max(0, Math.floor((Date.now() - lastObserved.getTime()) / 1000));
    text("observed-age", age < 2 ? "now" : `${age}s ago`);
  };

  const refresh = async () => {
    if (request) request.abort();
    request = new AbortController();
    try {
      const response = await fetch("/api/v1/status", { cache: "no-store", signal: request.signal });
      if (!response.ok) throw new Error(`status ${response.status}`);
      render(await response.json());
    } catch (error) {
      if (error.name !== "AbortError") byId("offline-banner").hidden = false;
    }
  };

  const root = document.documentElement;
  const applyTheme = (theme) => {
    root.dataset.theme = theme;
    byId("theme-toggle").textContent = theme === "dark" ? "Light" : "Dark";
  };
  let savedTheme = "light";
  try { savedTheme = localStorage.getItem("dba-theme") || (matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light"); } catch (_) { /* device storage may be blocked */ }
  applyTheme(savedTheme);
  byId("theme-toggle").addEventListener("click", () => {
    const next = root.dataset.theme === "dark" ? "light" : "dark";
    applyTheme(next);
    try { localStorage.setItem("dba-theme", next); } catch (_) { /* preference remains in memory */ }
  });

  refresh();
  setInterval(refresh, 2000);
  setInterval(updateAge, 1000);
})();

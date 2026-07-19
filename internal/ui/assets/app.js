(() => {
  "use strict";

  const byId = (id) => document.getElementById(id);
  const all = (selector) => Array.from(document.querySelectorAll(selector));
  const text = (id, value) => { const node = byId(id); if (node) node.textContent = value; };
  const integer = (value) => new Intl.NumberFormat().format(Number(value || 0));
  const number = (value) => Number(value || 0);
  const routes = ["overview", "performance", "connections", "database", "diagnostics"];
  const routeLabels = { overview: "Overview", performance: "Performance", connections: "Connections", database: "Database", diagnostics: "Diagnostics" };
  const samples = [];
  const activity = [];
  let previous = null;
  let lastObserved = null;
  let request = null;
  let toastTimer = null;
  let authenticationRequired = false;

  const formatBytes = (value) => {
    let bytes = number(value);
    const units = ["B", "KB", "MB", "GB", "TB"];
    let index = 0;
    while (bytes >= 1024 && index < units.length - 1) { bytes /= 1024; index += 1; }
    return `${bytes.toFixed(index === 0 ? 0 : bytes < 10 ? 1 : 0)} ${units[index]}`;
  };

  const formatDuration = (seconds) => {
    const value = number(seconds);
    if (value < 60) return `${value}s`;
    if (value < 3600) return `${Math.floor(value / 60)}m ${value % 60}s`;
    if (value < 86400) return `${Math.floor(value / 3600)}h ${Math.floor((value % 3600) / 60)}m`;
    return `${Math.floor(value / 86400)}d ${Math.floor((value % 86400) / 3600)}h`;
  };

  const formatClock = (value) => value
    ? new Intl.DateTimeFormat(undefined, { hour: "2-digit", minute: "2-digit", second: "2-digit" }).format(new Date(value))
    : "not checked";

  const formatDate = (value) => {
    if (!value || value === "unknown") return "unknown";
    const date = new Date(value);
    return Number.isNaN(date.getTime()) ? value : new Intl.DateTimeFormat(undefined, { dateStyle: "medium", timeStyle: "short" }).format(date);
  };

  const formatMilliseconds = (value) => number(value) === 0 ? "<0.001 ms" : `${number(value).toFixed(number(value) < 10 ? 3 : 2)} ms`;
  const formatRate = (value) => `${integer(Math.round(number(value)))} /s`;
  const signedPercent = (value) => `${number(value) >= 0 ? "+" : "−"}${Math.abs(number(value)).toFixed(2)}%`;

  const setOutcome = (id, value, positiveLabel = "improved", negativeLabel = "regressed") => {
    const node = byId(id);
    if (!node) return;
    const result = number(value);
    node.className = `comparison-outcome ${result >= 0 ? "is-win" : "is-loss"}`;
    node.textContent = `${signedPercent(result)} ${result >= 0 ? positiveLabel : negativeLabel}`;
  };

  const setPairBars = (directID, acceleratedID, direct, accelerated) => {
    const maximum = Math.max(number(direct), number(accelerated), 0.000001);
    byId(directID).style.width = `${Math.max(1, number(direct) / maximum * 100)}%`;
    byId(acceleratedID).style.width = `${Math.max(1, number(accelerated) / maximum * 100)}%`;
  };

  const setSignal = (id, state) => {
    const node = byId(id);
    if (!node) return;
    node.className = `signal ${state === "ok" ? "signal-ok" : state === "error" ? "signal-error" : "signal-checking"}`;
  };

  const setStateBadge = (id, state) => {
    const node = byId(id);
    if (!node) return;
    node.className = `state-badge ${state === "ok" ? "is-ok" : state === "error" ? "is-error" : "is-checking"}`;
    node.textContent = state;
  };

  const showToast = (message) => {
    const toast = byId("toast");
    toast.textContent = message;
    toast.hidden = false;
    clearTimeout(toastTimer);
    toastTimer = setTimeout(() => { toast.hidden = true; }, 2400);
  };

  const addActivity = (title, detail, time = new Date()) => {
    activity.unshift({ title, detail, time });
    if (activity.length > 12) activity.length = 12;
    const list = byId("activity-list");
    list.replaceChildren(...activity.map((item) => {
      const row = document.createElement("li");
      const stamp = document.createElement("time");
      stamp.dateTime = item.time.toISOString();
      stamp.textContent = formatClock(item.time);
      const strong = document.createElement("strong");
      strong.textContent = item.title;
      const small = document.createElement("small");
      small.textContent = item.detail;
      row.append(stamp, strong, small);
      return row;
    }));
  };

  const recordChanges = (data) => {
    if (!previous) {
      addActivity("Runtime snapshot loaded", `${data.upstream.status} upstream · ${data.relay.mode}`);
      if (data.benchmark?.available) addActivity("Measured benchmark loaded", data.benchmark.report.run_id);
      previous = data;
      return;
    }
    const accepted = number(data.relay.accepted_total) - number(previous.relay.accepted_total);
    const rejected = number(data.relay.rejected_total) - number(previous.relay.rejected_total);
    const relayErrors = number(data.relay.relay_errors_total) - number(previous.relay.relay_errors_total);
    const dialErrors = number(data.relay.dial_errors_total) - number(previous.relay.dial_errors_total);
    if (accepted > 0) addActivity(`${accepted} client connection${accepted === 1 ? "" : "s"} accepted`, `${integer(data.relay.accepted_total)} total`);
    if (rejected > 0) addActivity(`${rejected} client connection${rejected === 1 ? "" : "s"} rejected`, "upstream safety limit");
    if (relayErrors > 0) addActivity(`${relayErrors} relay error${relayErrors === 1 ? "" : "s"}`, "inspect client and database logs");
    if (dialErrors > 0) addActivity(`${dialErrors} upstream dial failure${dialErrors === 1 ? "" : "s"}`, data.upstream.address);
    if (data.upstream.status !== previous.upstream.status) addActivity(`Upstream changed to ${data.upstream.status}`, data.upstream.error || data.upstream.address);
    if (data.benchmark?.report?.run_id && data.benchmark.report.run_id !== previous.benchmark?.report?.run_id) addActivity("New benchmark evidence loaded", data.benchmark.report.run_id);
    previous = data;
  };

  const drawChart = () => {
    const canvas = byId("pressure-chart");
    if (!canvas) return;
    const rect = canvas.getBoundingClientRect();
    if (rect.width < 20) return;
    const ratio = Math.min(window.devicePixelRatio || 1, 2);
    canvas.width = Math.round(rect.width * ratio);
    canvas.height = Math.round(rect.height * ratio);
    const ctx = canvas.getContext("2d");
    ctx.scale(ratio, ratio);
    const style = getComputedStyle(document.documentElement);
    const line = style.getPropertyValue("--line").trim();
    const quiet = style.getPropertyValue("--ink-3").trim();
    const pressure = style.getPropertyValue("--primary-light").trim();
    const links = style.getPropertyValue("--accent-light").trim();
    const width = rect.width;
    const height = rect.height;
    const pad = 8;

    ctx.clearRect(0, 0, width, height);
    ctx.strokeStyle = line;
    ctx.lineWidth = 1;
    for (let row = 0; row <= 4; row += 1) {
      const y = pad + ((height - pad * 2) * row / 4);
      ctx.beginPath(); ctx.moveTo(0, y + .5); ctx.lineTo(width, y + .5); ctx.stroke();
    }

    if (samples.length < 2) {
      ctx.fillStyle = quiet;
      ctx.font = "10px IBM Plex Mono, Cascadia Mono, monospace";
      ctx.fillText("Collecting live samples…", 8, height / 2);
      return;
    }

    const hasTraffic = samples.some((sample) => sample.percent > 0 || sample.links > 0);
    if (!hasTraffic) {
      ctx.strokeStyle = links;
      ctx.lineWidth = 2;
      ctx.beginPath(); ctx.moveTo(pad, height - pad - .5); ctx.lineTo(width - pad, height - pad - .5); ctx.stroke();
      ctx.fillStyle = quiet;
      ctx.font = "10px IBM Plex Mono, Cascadia Mono, monospace";
      ctx.fillText(`No active connections across ${samples.length} live samples`, 8, height / 2);
      return;
    }

    const x = (index) => pad + (width - pad * 2) * index / (samples.length - 1);
    const yPercent = (value) => height - pad - (height - pad * 2) * Math.min(100, Math.max(0, value)) / 100;
    const maxLinks = Math.max(4, ...samples.map((sample) => sample.links));
    const yLinks = (value) => height - pad - (height - pad * 2) * value / maxLinks;

    ctx.strokeStyle = pressure;
    ctx.lineWidth = 2;
    ctx.beginPath();
    samples.forEach((sample, index) => index === 0 ? ctx.moveTo(x(index), yPercent(sample.percent)) : ctx.lineTo(x(index), yPercent(sample.percent)));
    ctx.stroke();

    ctx.strokeStyle = links;
    ctx.lineWidth = 1.5;
    ctx.setLineDash([4, 4]);
    ctx.beginPath();
    samples.forEach((sample, index) => index === 0 ? ctx.moveTo(x(index), yLinks(sample.links)) : ctx.lineTo(x(index), yLinks(sample.links)));
    ctx.stroke();
    ctx.setLineDash([]);
  };

  const renderDatabase = (upstream) => {
    const metadata = upstream.metadata || {};
    setStateBadge("database-state", upstream.status);
    setStateBadge("database-state-detail", upstream.status);
    text("db-address", upstream.address || "—");
    text("db-address-detail", upstream.address || "—");
    text("db-checked", upstream.checked_at ? `checked ${formatClock(upstream.checked_at)}` : "not checked");
    text("upstream-latency", upstream.status === "ok" ? `${number(upstream.latency_ms).toFixed(2)} ms` : "—");
    text("probe-latency-large", upstream.status === "ok" ? `${number(upstream.latency_ms).toFixed(2)} ms` : "—");
    text("db-server", metadata.version ? `${metadata.vendor || "server"} ${metadata.version}` : "—");
    text("db-vendor", metadata.vendor || "Database");
    text("db-version", metadata.version || "—");
    text("db-comment", metadata.version_comment || upstream.error || "Waiting for metadata.");
    text("db-database", metadata.database || "default");
    text("db-charset", metadata.character_set || "—");
    text("db-charset-detail", metadata.character_set || "—");
    text("db-collation", metadata.collation || "—");
    text("db-isolation", metadata.transaction_isolation || "—");
    text("db-isolation-detail", metadata.transaction_isolation || "—");
    text("db-autocommit", metadata.version ? (metadata.autocommit ? "on" : "off") : "—");
    text("db-autocommit-detail", metadata.version ? (metadata.autocommit ? "on" : "off") : "—");
    text("db-timezone", metadata.time_zone || "—");
    text("db-sql-mode", metadata.sql_mode || "—");
    const error = byId("database-error");
    error.hidden = !upstream.error;
    error.textContent = upstream.error || "";
  };

  const renderDecision = (data) => {
    const upstreamError = data.upstream.status === "error";
    const pressureWarning = number(data.pressure.percent) >= 90;
    const severity = byId("decision-severity");
    severity.className = `decision-severity ${upstreamError ? "is-error" : pressureWarning ? "is-warn" : "is-ok"}`;
    severity.textContent = upstreamError ? "Act now" : pressureWarning ? "Reduce load" : "Observe";
    const active = number(data.pressure.database_links);
    const limit = number(data.pressure.safe_limit);
    const title = upstreamError
      ? "Upstream database is unreachable."
      : pressureWarning
        ? "Connection budget is nearly exhausted."
        : active > 0
          ? `${integer(active)} of ${integer(limit)} database links are in use.`
          : "Connection budget has full headroom.";
    text("decision-title", title);
    text("safe-action", data.pressure.safe_action || "Continue observing live traffic.");
    text("acceleration-reason", data.acceleration.reason);
    if (upstreamError) text("blocking-announcement", `Database degraded. ${data.pressure.safe_action}`);
  };

  const renderBenchmark = (status) => {
    const available = Boolean(status?.available && status.report);
    byId("benchmark-empty").hidden = available;
    byId("benchmark-report").hidden = !available;
    text("benchmark-confidence", status?.error ? "evidence error" : available ? "local evidence" : "not measured");
    if (!available) {
      if (status?.error) {
        text("benchmark-empty-title", "Saved evidence could not be read.");
        const detail = byId("benchmark-empty").querySelector("p");
        if (detail) detail.textContent = status.error;
      }
      return;
    }

    const report = status.report;
    const direct = report.direct;
    const accelerated = report.accelerator;
    const gains = report.gains;
    const workload = report.workload;
    const environment = report.environment;
    const totalOperations = number(workload.operations_per_path) * (number(workload.direct_runs) + 1);
    const totalErrors = number(direct.errors) + number(accelerated.errors);

    text("benchmark-reduction", `${number(gains.connection_reduction_percent).toFixed(1)}%`);
    text("benchmark-connection-path", `${integer(direct.peak_database_connections)} direct → ${integer(accelerated.peak_database_connections)} accelerated`);
    text("benchmark-saved", integer(gains.connections_saved));
    text("benchmark-fanin", `${number(gains.fan_in_ratio).toFixed(1)}×`);
    text("benchmark-ready", `${number(gains.client_ready_speedup).toFixed(2)}×`);
    text("benchmark-ready-detail", number(gains.client_ready_speedup) >= 1 ? "faster client readiness" : "slower client readiness");
    text("benchmark-errors", integer(totalErrors));
    text("benchmark-operations", `across ${integer(totalOperations)} measured operations`);

    text("connections-direct", integer(direct.peak_database_connections));
    text("connections-accelerated", integer(accelerated.peak_database_connections));
    text("connections-outcome", `−${number(gains.connection_reduction_percent).toFixed(1)}% fewer`);
    setPairBars("bar-connections-direct", "bar-connections-accelerated", direct.peak_database_connections, accelerated.peak_database_connections);

    text("ready-direct", formatMilliseconds(direct.client_ready_ms));
    text("ready-accelerated", formatMilliseconds(accelerated.client_ready_ms));
    const readyChange = direct.client_ready_ms === 0 ? 0 : (direct.client_ready_ms - accelerated.client_ready_ms) / direct.client_ready_ms * 100;
    setOutcome("ready-outcome", readyChange, "faster", "slower");
    setPairBars("bar-ready-direct", "bar-ready-accelerated", direct.client_ready_ms, accelerated.client_ready_ms);

    text("throughput-direct", formatRate(direct.throughput_per_second));
    text("throughput-accelerated", formatRate(accelerated.throughput_per_second));
    setOutcome("throughput-outcome", gains.throughput_change_percent);
    setPairBars("bar-throughput-direct", "bar-throughput-accelerated", direct.throughput_per_second, accelerated.throughput_per_second);

    text("p95-direct", formatMilliseconds(direct.p95_ms));
    text("p95-accelerated", formatMilliseconds(accelerated.p95_ms));
    setOutcome("p95-outcome", gains.p95_latency_change_percent);
    setPairBars("bar-p95-direct", "bar-p95-accelerated", direct.p95_ms, accelerated.p95_ms);

    text("benchmark-server", `${environment.server_product} ${environment.server_version}`);
    text("benchmark-workload", `${integer(workload.open_clients)} clients / ${integer(workload.active_concurrency)} active`);
    text("benchmark-dataset", `${integer(workload.dataset_rows)} rows / ${integer(workload.payload_bytes)} B payload`);
    text("benchmark-query", workload.query_shape);
    text("benchmark-completed", formatDate(report.completed_at));
    text("benchmark-run-id", report.run_id);
    text("benchmark-scope", report.evidence.scope);
    text("benchmark-caveat", report.evidence.caveat);
  };

  const render = (data) => {
    recordChanges(data);
    lastObserved = new Date(data.observed_at);
    const healthy = data.lifecycle.state === "ready" && data.upstream.status === "ok";
    const state = healthy ? "ok" : data.upstream.status === "error" ? "error" : "checking";
    setSignal("health-signal", state);
    setSignal("rail-signal", state);
    text("rail-state", healthy ? "Runtime ready" : data.upstream.status === "error" ? "Degraded" : "Checking");
    text("health-label", healthy ? "Runtime ready" : data.upstream.status === "error" ? "Database degraded" : "Runtime checking");
    text("health-detail", healthy ? `${data.upstream.metadata.vendor} ${data.upstream.metadata.version}` : data.upstream.error || data.lifecycle.reason);
    text("relay-address", data.relay.listen_address);
    text("diag-relay-address", data.relay.listen_address);
    text("rail-upstream", data.relay.upstream_address);
    text("diag-upstream-address", data.relay.upstream_address);
    const clientTLSMode = data.relay.client_tls_mode || "disabled";
    text("diag-client-tls", clientTLSMode === "required" ? "TLS 1.2+ required" : clientTLSMode === "passthrough" ? "Database TLS passthrough" : "Plaintext / protected boundary");
    text("diag-client-tls-expiry", data.relay.client_tls_expires_at ? `expires ${formatDate(data.relay.client_tls_expires_at)}` : "not terminated here");
    text("relay-mode", data.relay.mode.replaceAll("-", " "));

    const pooled = data.relay.mode === "protocol-pooled";
    text("gateway-node-code", pooled ? "POOL" : "RELAY");
    text("gateway-node-detail", pooled ? "pool connections" : "direct links");
    text("gateway-link-action", pooled ? "execute" : "forward");
    text("gateway-error-label", pooled ? "Query errors" : "Relay errors");
    text("relation-title", pooled ? "Logical clients share a bounded upstream pool." : "One client. One upstream link.");
    text("connection-mode-chip", pooled ? "pooled mode" : "compatibility mode");
    text("relation-symbol", pooled ? "→" : "=");
    text("relation-copy", pooled
      ? "Idle clients hold no database socket. Safe autocommit text queries borrow a connection; transactions keep one pinned until commit or rollback."
      : "Connection reduction is disabled in this build. The safety limit still prevents an unbounded client spike from reaching MariaDB.");
    text("wire-capability-title", pooled ? "SQL protocol gateway" : "Native wire relay");
    text("wire-capability-detail", pooled ? "Authenticated text and prepared queries execute through the bounded upstream pool." : "Byte-for-byte MySQL/MariaDB traffic forwarding.");
    const poolingGate = byId("pooling-gate-state");
    poolingGate.className = `gate-state ${pooled ? "pass" : "lock"}`;
    poolingGate.textContent = pooled ? "Live" : "Locked";
    text("pooling-gate-detail", pooled ? "Safe text queries reuse reset connections; transactions and prepared handles remain pinned." : "Requires protocol-aware session reset and pinning.");
    text("session-inventory-copy", pooled
      ? "Pooled mode tracks aggregate clients, waits, and pin reasons. Prepared SQL lives only while its client handle exists; values are never exposed."
      : "The compatibility relay forwards wire bytes without parsing client identity, transaction state, or query text.");
    text("upstream-limit-behavior", pooled ? "Queries wait at the pool boundary" : "Hard rejection at limit");
    text("queue-limit-behavior", pooled ? "Request count and queued SQL bytes are hard bounded" : "Not active in relay mode");

    text("logical-clients", integer(data.pressure.logical_clients));
    text("active-pool", integer(data.pressure.active_pool));
    text("database-links", integer(data.pressure.database_links));
    text("connections-clients", integer(data.pressure.logical_clients));
    text("connections-links", integer(data.pressure.database_links));
    text("waiting-work", integer(data.pressure.waiting_work));
    text("safe-limit", integer(data.pressure.safe_limit));
    text("pressure-percent", `${number(data.pressure.percent).toFixed(1)}%`);
    text("pressure-summary", `${data.pressure.logical_clients} logical clients, ${data.pressure.waiting_work} waiting, and ${data.pressure.database_links} database links. ${number(data.pressure.percent).toFixed(1)} percent of the safe limit is used.`);
    const progress = byId("pressure-progress");
    progress.setAttribute("aria-valuenow", String(number(data.pressure.percent).toFixed(1)));
    byId("capacity-fill").style.width = `${Math.min(100, number(data.pressure.percent))}%`;

    text("accepted-total", integer(data.relay.accepted_total));
    text("rejected-total", integer(data.relay.rejected_total));
    text("bytes-up", formatBytes(data.relay.client_to_db_bytes));
    text("bytes-down", formatBytes(data.relay.db_to_client_bytes));
    text("active-now", integer(data.relay.active));
    text("dial-errors", integer(data.relay.dial_errors_total));
    text("relay-errors", integer(data.relay.relay_errors_total));
    text("uptime", formatDuration(data.uptime_seconds));

    text("limit-logical", integer(data.limits.logical_connections));
    text("limit-upstream", integer(data.limits.upstream_connections));
    text("limit-queued", integer(data.limits.queued_requests));
    text("limit-bytes", formatBytes(data.limits.queued_bytes));
    text("observed-logical", integer(data.pressure.logical_clients));
    text("observed-upstream", integer(data.pressure.database_links));
    text("observed-queued", integer(data.pressure.waiting_work));

    renderDatabase(data.upstream);
    renderDecision(data);
    renderBenchmark(data.benchmark);
    text("build-short", `v${data.build.version}`);
    text("build-version", data.build.version);
    text("build-commit", data.build.commit);
    text("build-go", data.build.go_version);
    text("build-date", formatDate(data.build.build_date));
    const reasonSummary = (reasons) => Object.entries(reasons || {}).sort((a, b) => b[1] - a[1]).map(([reason, count]) => `${reason.replaceAll("_", " ")} ${count}`).join(" · ") || "none";
    text("diag-pin-reasons", reasonSummary(data.relay.pin_reasons));
    text("diag-rejection-reasons", reasonSummary(data.relay.rejection_reasons));
    text("diag-reset-discards", integer(data.relay.reset_discards_total));

    samples.push({ percent: number(data.pressure.percent), links: number(data.pressure.database_links), at: lastObserved });
    if (samples.length > 60) samples.shift();
    text("sample-count", `${samples.length} sample${samples.length === 1 ? "" : "s"}`);
    drawChart();
    byId("offline-banner").hidden = true;
  };

  const updateAge = () => {
    if (!lastObserved) return;
    const age = Math.max(0, Math.floor((Date.now() - lastObserved.getTime()) / 1000));
    text("observed-age", age < 2 ? "now" : `${age}s ago`);
  };

  const refresh = async (manual = false) => {
    if (request) request.abort();
    request = new AbortController();
    const button = byId("refresh-button");
    if (manual) { button.classList.add("is-busy"); button.textContent = "Loading"; }
    try {
      const response = await fetch("/api/v1/status", { cache: "no-store", signal: request.signal });
      if (response.status === 401) {
        showAuthentication();
        return;
      }
      if (!response.ok) throw new Error(`status ${response.status}`);
      render(await response.json());
      if (manual) showToast("Live metrics refreshed");
    } catch (error) {
      if (error.name !== "AbortError") {
        byId("offline-banner").hidden = false;
        if (manual) showToast("Refresh failed; retrying automatically");
      }
    } finally {
      request = null;
      if (manual) { button.classList.remove("is-busy"); button.textContent = "Refresh"; }
    }
  };

  const showAuthentication = (message = "") => {
    const gate = byId("auth-gate");
    document.body.classList.add("auth-locked");
    gate.hidden = false;
    byId("auth-loading").hidden = true;
    byId("auth-form").hidden = false;
    const error = byId("auth-error");
    error.textContent = message;
    error.hidden = !message;
    byId("sign-out").hidden = true;
    setTimeout(() => byId("admin-token").focus(), 0);
  };

  const hideAuthentication = () => {
    document.body.classList.remove("auth-locked");
    byId("auth-gate").hidden = true;
    byId("auth-error").hidden = true;
    byId("admin-token").value = "";
    byId("sign-out").hidden = !authenticationRequired;
  };

  const checkAuthentication = async () => {
    try {
      const response = await fetch("/api/v1/session", { cache: "no-store" });
      if (!response.ok) throw new Error(`status ${response.status}`);
      const session = await response.json();
      authenticationRequired = Boolean(session.required);
      if (session.authenticated) {
        hideAuthentication();
        await refresh();
      } else {
        showAuthentication();
      }
    } catch (_) {
      showAuthentication("The local control plane did not answer. Check that the accelerator is running, then try again.");
    }
  };

  const setRoute = (requested, updateHash = true) => {
    const route = routes.includes(requested) ? requested : "overview";
    all("[data-view]").forEach((view) => {
      const active = view.dataset.view === route;
      view.hidden = !active;
      view.classList.toggle("is-active", active);
    });
    all("[data-route]").forEach((button) => {
      const active = button.dataset.route === route;
      button.classList.toggle("is-active", active);
      if (active) button.setAttribute("aria-current", "page"); else button.removeAttribute("aria-current");
    });
    text("route-title", routeLabels[route]);
    document.title = `Database Accelerator — ${routeLabels[route]}`;
    if (updateHash && location.hash !== `#${route}`) history.pushState(null, "", `#${route}`);
    byId("mobile-menu").setAttribute("aria-expanded", "false");
    document.querySelector(".rail").classList.remove("is-open");
    if (route === "overview") requestAnimationFrame(drawChart);
  };

  const applyTheme = (theme) => {
    document.documentElement.dataset.theme = theme;
    const next = theme === "dark" ? "Light" : "Dark";
    byId("theme-toggle").textContent = next;
    byId("theme-toggle").setAttribute("aria-label", `Switch to ${next.toLowerCase()} theme`);
    requestAnimationFrame(drawChart);
  };

  let savedTheme = "dark";
  try { savedTheme = localStorage.getItem("dba-theme") || (matchMedia("(prefers-color-scheme: light)").matches ? "light" : "dark"); } catch (_) { /* local preference is optional */ }
  applyTheme(savedTheme);

  all("[data-route]").forEach((button) => button.addEventListener("click", () => setRoute(button.dataset.route)));
  all("[data-route-jump]").forEach((button) => button.addEventListener("click", () => setRoute(button.dataset.routeJump)));
  addEventListener("hashchange", () => setRoute(location.hash.slice(1), false));
  byId("refresh-button").addEventListener("click", () => refresh(true));
  byId("auth-form").addEventListener("submit", async (event) => {
    event.preventDefault();
    const submit = byId("auth-submit");
    const error = byId("auth-error");
    submit.disabled = true;
    submit.textContent = "Checking";
    error.hidden = true;
    try {
      const response = await fetch("/api/v1/session", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ token: byId("admin-token").value }),
      });
      const body = await response.json().catch(() => ({}));
      if (!response.ok) throw new Error(body.error || `Login failed (${response.status})`);
      hideAuthentication();
      addActivity("Operator session opened", "admin control plane authenticated");
      await refresh();
    } catch (loginError) {
      showAuthentication(loginError.message || "Login failed");
      byId("admin-token").select();
    } finally {
      submit.disabled = false;
      submit.textContent = "Continue";
    }
  });
  byId("sign-out").addEventListener("click", async () => {
    await fetch("/api/v1/session", { method: "DELETE" }).catch(() => {});
    previous = null;
    showAuthentication();
  });
  byId("theme-toggle").addEventListener("click", () => {
    const next = document.documentElement.dataset.theme === "dark" ? "light" : "dark";
    applyTheme(next);
    try { localStorage.setItem("dba-theme", next); } catch (_) { /* preference remains in memory */ }
  });
  byId("mobile-menu").addEventListener("click", () => {
    const rail = document.querySelector(".rail");
    const open = !rail.classList.contains("is-open");
    rail.classList.toggle("is-open", open);
    byId("mobile-menu").setAttribute("aria-expanded", String(open));
  });
  byId("copy-endpoint").addEventListener("click", async () => {
    const value = byId("relay-address").textContent;
    try { await navigator.clipboard.writeText(value); showToast(`Copied ${value}`); }
    catch (_) { showToast(`Application endpoint: ${value}`); }
  });
  addEventListener("keydown", (event) => {
    if (event.key === "Escape") {
      document.querySelector(".rail").classList.remove("is-open");
      byId("mobile-menu").setAttribute("aria-expanded", "false");
    }
    if (event.altKey && /^[1-5]$/.test(event.key)) { event.preventDefault(); setRoute(routes[number(event.key) - 1]); }
  });
  addEventListener("resize", drawChart);

  setRoute(location.hash.slice(1), false);
  checkAuthentication();
  setInterval(() => { if (byId("auth-gate").hidden) refresh(); }, 2000);
  setInterval(updateAge, 1000);
})();

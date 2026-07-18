(() => {
  "use strict";

  const byId = (id) => document.getElementById(id);
  const all = (selector) => Array.from(document.querySelectorAll(selector));
  const text = (id, value) => { const node = byId(id); if (node) node.textContent = value; };
  const integer = (value) => new Intl.NumberFormat().format(Number(value || 0));
  const number = (value) => Number(value || 0);
  const routes = ["overview", "connections", "database", "diagnostics"];
  const routeLabels = { overview: "Overview", connections: "Connections", database: "Database", diagnostics: "Diagnostics" };
  const samples = [];
  const activity = [];
  let previous = null;
  let lastObserved = null;
  let request = null;
  let toastTimer = null;

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
    text("relay-mode", data.relay.mode.replaceAll("-", " "));

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

    renderDatabase(data.upstream);
    renderDecision(data);
    text("build-short", `v${data.build.version}`);
    text("build-version", data.build.version);
    text("build-commit", data.build.commit);
    text("build-go", data.build.go_version);
    text("build-date", formatDate(data.build.build_date));

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
    if (event.altKey && /^[1-4]$/.test(event.key)) { event.preventDefault(); setRoute(routes[number(event.key) - 1]); }
  });
  addEventListener("resize", drawChart);

  setRoute(location.hash.slice(1), false);
  refresh();
  setInterval(refresh, 2000);
  setInterval(updateAge, 1000);
})();

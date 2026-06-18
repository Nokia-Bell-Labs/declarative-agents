import "./style.css";

const paths = {
  state: "/monitor/state",
  machine: "/monitor/machine",
  tools: "/monitor/tools",
  events: "/monitor/events",
  stream: "/monitor/events/stream",
};

function $(id: string): HTMLElement {
  const el = document.getElementById(id);
  if (!el) {
    throw new Error(`missing #${id}`);
  }
  return el;
}

async function fetchJSON(url: string): Promise<unknown> {
  const res = await fetch(url);
  if (!res.ok) {
    throw new Error(`${url} -> HTTP ${res.status}`);
  }
  return res.json();
}

function setText(id: string, text: string): void {
  $(id).textContent = text;
}

function renderJSON(id: string, data: unknown): void {
  $(id).textContent = JSON.stringify(data, null, 2);
}

function renderRunSummary(state: Record<string, unknown>): void {
  const run = state.run as Record<string, unknown> | undefined;
  if (!run) {
    setText("run-summary", "No run snapshot yet.");
    return;
  }
  const parts = ["run_id", "status", "state", "signal", "iteration"].map(
    (k) => `${k}=${String(run[k] ?? "")}`,
  );
  setText("run-summary", parts.join(" | "));
}

function renderTransitions(machine: Record<string, unknown>): void {
  const body = $("transition-rows");
  body.replaceChildren();
  const rows = machine.transitions as Array<Record<string, unknown>> | undefined;
  if (!rows?.length) {
    const tr = document.createElement("tr");
    const td = document.createElement("td");
    td.colSpan = 5;
    td.textContent = "No transitions in machine snapshot.";
    tr.appendChild(td);
    body.appendChild(tr);
    return;
  }
  for (const t of rows) {
    const tr = document.createElement("tr");
    for (const col of ["state", "signal", "next", "action", "metric_labels"]) {
      const td = document.createElement("td");
      const v = t[col];
      td.textContent =
        typeof v === "object" && v !== null ? JSON.stringify(v) : String(v ?? "");
      tr.appendChild(td);
    }
    body.appendChild(tr);
  }
}

function renderToolsTable(toolsRoot: Record<string, unknown>): void {
  const body = $("tool-rows");
  body.replaceChildren();
  const tools = toolsRoot.tools as Array<Record<string, unknown>> | undefined;
  if (!tools?.length) {
    const tr = document.createElement("tr");
    const td = document.createElement("td");
    td.colSpan = 4;
    td.textContent = "No tools registered.";
    tr.appendChild(td);
    body.appendChild(tr);
    return;
  }
  for (const t of tools) {
    const tr = document.createElement("tr");
    for (const col of ["name", "category", "visibility", "emits"]) {
      const td = document.createElement("td");
      const v = t[col];
      td.textContent = Array.isArray(v) ? v.join(", ") : String(v ?? "");
      tr.appendChild(td);
    }
    body.appendChild(tr);
  }
}

function renderRecentEvents(eventsRoot: Record<string, unknown>): void {
  const list = eventsRoot.recent_events as Array<Record<string, unknown>> | undefined;
  renderJSON("recent-events", list ?? []);
}

function prependFeed(text: string): void {
  const feed = $("stream-feed");
  const row = document.createElement("div");
  row.className = "feed-row";
  row.textContent = text;
  feed.prepend(row);
  while (feed.childNodes.length > 120) {
    feed.lastChild?.remove();
  }
}

function wireEventSource(): void {
  const es = new EventSource(paths.stream);
  const log = (kind: string) => (ev: MessageEvent) => {
    prependFeed(`${new Date().toISOString()} [${kind}] ${ev.data}`);
  };
  es.addEventListener("run_event", log("run_event"));
  es.addEventListener("metric_sample", log("metric_sample"));
  es.onerror = () => {
    prependFeed(`${new Date().toISOString()} [eventsource] connection error (retrying)`);
  };
}

function shellHTML(): string {
  return `
    <h1>Knowledge Manager Monitor</h1>
    <div id="status" class="banner">Loading monitor snapshot…</div>
    <div class="grid">
      <section class="panel">
        <h2>Run</h2>
        <div id="run-summary" class="feed"></div>
      </section>
      <section class="panel">
        <h2>State (JSON)</h2>
        <pre id="state-json"></pre>
      </section>
      <section class="panel full-width">
        <h2>Machine transitions</h2>
        <table>
          <thead><tr><th>state</th><th>signal</th><th>next</th><th>action</th><th>metric_labels</th></tr></thead>
          <tbody id="transition-rows"></tbody>
        </table>
        <h2 style="margin-top:0.75rem">Machine (JSON)</h2>
        <pre id="machine-json"></pre>
      </section>
      <section class="panel">
        <h2>Tools</h2>
        <table>
          <thead><tr><th>name</th><th>category</th><th>visibility</th><th>emits</th></tr></thead>
          <tbody id="tool-rows"></tbody>
        </table>
      </section>
      <section class="panel">
        <h2>Recent events (JSON)</h2>
        <pre id="recent-events"></pre>
      </section>
      <section class="panel full-width">
        <h2>Live stream (/monitor/events/stream)</h2>
        <div id="stream-feed" class="feed"></div>
      </section>
    </div>
  `;
}

async function refresh(): Promise<void> {
  try {
    const [state, machine, tools, events] = await Promise.all([
      fetchJSON(paths.state),
      fetchJSON(paths.machine),
      fetchJSON(paths.tools),
      fetchJSON(paths.events),
    ]);
    const s = state as Record<string, unknown>;
    const m = machine as Record<string, unknown>;
    const t = tools as Record<string, unknown>;
    const e = events as Record<string, unknown>;
    setText("status", "Connected to monitor API.");
    renderRunSummary(s);
    renderJSON("state-json", s);
    renderJSON("machine-json", m);
    renderTransitions(m);
    renderToolsTable(t);
    renderRecentEvents(e);
  } catch (err) {
    const msg = err instanceof Error ? err.message : String(err);
    setText("status", `Monitor API error: ${msg}`);
  }
}

function bootstrap(): void {
  $("app").innerHTML = shellHTML();
  wireEventSource();
  void refresh();
  window.setInterval(() => void refresh(), 4000);
}

bootstrap();

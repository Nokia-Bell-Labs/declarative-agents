import { useState, type ReactNode } from "react";
import { useMonitor, type EventsSnapshot, type RunSnapshot, type ToolsSnapshot } from "./api";
import StateMachineGraph from "./StateMachineGraph";

type TabId = "graph" | "run" | "tools" | "events" | "raw";

const TABS: { id: TabId; label: string }[] = [
  { id: "graph", label: "Graph" },
  { id: "run", label: "Run" },
  { id: "tools", label: "Tools" },
  { id: "events", label: "Events" },
  { id: "raw", label: "Raw" },
];

function statusBadgeClass(status?: string): string {
  switch ((status ?? "").toLowerCase()) {
    case "running":
      return "badge badge-running";
    case "done":
    case "completed":
      return "badge badge-done";
    case "failed":
    case "error":
      return "badge badge-failed";
    default:
      return "badge badge-idle";
  }
}

function StatusBanner({ status, error }: { status: string; error?: string }) {
  const cls =
    status === "connected" ? "banner banner-connected" : status === "error" ? "banner banner-error" : "banner";
  const text =
    status === "connected"
      ? "Connected to monitor API."
      : status === "error"
        ? `Monitor API error: ${error ?? "unknown"}`
        : "Loading monitor snapshot…";
  return (
    <div className={cls}>
      <span className="banner-dot" />
      {text}
    </div>
  );
}

function RunPanel({ run }: { run?: RunSnapshot }) {
  if (!run) return <div className="empty">No run snapshot yet.</div>;
  const cards: { label: string; value: ReactNode }[] = [
    { label: "run id", value: run.run_id ?? "—" },
    { label: "status", value: <span className={statusBadgeClass(run.status)}>{run.status ?? "—"}</span> },
    { label: "state", value: run.state ?? "—" },
    { label: "signal", value: run.signal ?? "—" },
    { label: "iteration", value: String(run.iteration ?? "—") },
    { label: "updated", value: run.updated_at ?? "—" },
  ];
  return (
    <div className="panel-grid">
      {cards.map((c) => (
        <div className="stat-card" key={c.label}>
          <div className="stat-label">{c.label}</div>
          <div className="stat-value">{c.value}</div>
        </div>
      ))}
    </div>
  );
}

function ToolsPanel({ tools }: { tools?: ToolsSnapshot }) {
  const rows = tools?.tools ?? [];
  if (!rows.length) return <div className="empty">No tools registered.</div>;
  return (
    <div className="table-container">
      <table>
        <thead>
          <tr>
            <th>name</th>
            <th>category</th>
            <th>visibility</th>
            <th>emits</th>
          </tr>
        </thead>
        <tbody>
          {rows.map((t, i) => (
            <tr key={t.name ?? i}>
              <td className="cell-mono">{t.name ?? "—"}</td>
              <td>{t.category ?? "—"}</td>
              <td>{t.visibility ?? "—"}</td>
              <td className="cell-mono">{(t.emits ?? []).join(", ") || "—"}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function EventsPanel({
  feed,
  events,
}: {
  feed: { id: number; kind: string; receivedAt: string; data: string }[];
  events?: EventsSnapshot;
}) {
  const recent = events?.recent_events ?? [];
  return (
    <>
      <div className="panel">
        <h2>Live stream</h2>
        {feed.length ? (
          <div className="feed">
            {feed.map((f) => (
              <div className="feed-row" key={f.id}>
                <span className="feed-time">{f.receivedAt.slice(11, 23)}</span>
                <span className={`feed-kind feed-kind-${f.kind}`}>[{f.kind}]</span> {f.data}
              </div>
            ))}
          </div>
        ) : (
          <div className="empty">Waiting for events…</div>
        )}
      </div>
      <div className="panel">
        <h2>Recent events ({recent.length})</h2>
        <pre className="raw">{JSON.stringify(recent, null, 2)}</pre>
      </div>
    </>
  );
}

export default function App() {
  const data = useMonitor();
  const [tab, setTab] = useState<TabId>("graph");
  const run = data.state?.run;

  const counts: Partial<Record<TabId, number>> = {
    tools: data.tools?.tools?.length,
    events: data.feed.length,
  };

  return (
    <div className="app">
      <header className="app-header">
        <div className="app-brand">
          <span className="app-brand-title">Knowledge Manager Monitor</span>
          <span className="app-brand-sub">{data.machine?.name ?? "documentation-curator"}</span>
        </div>
        <nav className="app-nav">
          {TABS.map((t) => (
            <button
              key={t.id}
              className={`nav-tab${tab === t.id ? " nav-tab-active" : ""}`}
              onClick={() => setTab(t.id)}
            >
              {t.label}
              {counts[t.id] != null ? <span className="nav-tab-count">{counts[t.id]}</span> : null}
            </button>
          ))}
        </nav>
      </header>

      <main className="app-main">
        <StatusBanner status={data.status} error={data.error} />

        {tab === "graph" && (
          <>
            <div className="banner">
              <span className={statusBadgeClass(run?.status)}>{run?.status ?? "idle"}</span>
              <span className="mono">
                state={run?.state ?? "—"} · signal={run?.signal ?? "—"} · iteration={run?.iteration ?? "—"}
              </span>
            </div>
            <StateMachineGraph
              machine={data.machine}
              currentState={run?.state}
              lastTransition={data.lastTransition}
            />
          </>
        )}

        {tab === "run" && (
          <div className="panel">
            <h2>Run snapshot</h2>
            <RunPanel run={run} />
          </div>
        )}

        {tab === "tools" && (
          <div className="panel">
            <h2>Tools</h2>
            <ToolsPanel tools={data.tools} />
          </div>
        )}

        {tab === "events" && <EventsPanel feed={data.feed} events={data.events} />}

        {tab === "raw" && (
          <>
            <div className="panel">
              <h2>State (JSON)</h2>
              <pre className="raw">{JSON.stringify(data.state ?? {}, null, 2)}</pre>
            </div>
            <div className="panel">
              <h2>Machine (JSON)</h2>
              <pre className="raw">{JSON.stringify(data.machine ?? {}, null, 2)}</pre>
            </div>
          </>
        )}
      </main>
    </div>
  );
}

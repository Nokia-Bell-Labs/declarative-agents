import type { ReactNode } from "react";
import type { RunSnapshot } from "@declarative-agents/ui-kit";
import type { EventsSnapshot, FeedItem, MonitorData, ToolsSnapshot } from "./api";

// Curator-specific views of the monitor snapshot. The machine figure is the
// kit MachineView; these cover what the kit does not: the run card grid, the
// tools table, the live feed, and the raw JSON.

export function statusBadgeClass(status?: string): string {
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

export function StatusBanner({ status, error }: Pick<MonitorData, "status" | "error">) {
  const cls = status === "connected" ? "banner banner-connected" : status === "error" ? "banner banner-error" : "banner";
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

export function RunLine({ run }: { run?: RunSnapshot }) {
  return (
    <div className="banner">
      <span className={statusBadgeClass(run?.status)}>{run?.status ?? "idle"}</span>
      <span className="mono">
        state={run?.state ?? "—"} · signal={run?.signal ?? "—"} · iteration={run?.iteration ?? "—"}
      </span>
    </div>
  );
}

export function RunPanel({ run }: { run?: RunSnapshot }) {
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

export function ToolsPanel({ tools }: { tools?: ToolsSnapshot }) {
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

export function EventsPanel({ feed, events }: { feed: FeedItem[]; events?: EventsSnapshot }) {
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

export function RawPanel({ title, value }: { title: string; value: unknown }) {
  return (
    <div className="panel">
      <h2>{title}</h2>
      <pre className="raw">{JSON.stringify(value ?? {}, null, 2)}</pre>
    </div>
  );
}

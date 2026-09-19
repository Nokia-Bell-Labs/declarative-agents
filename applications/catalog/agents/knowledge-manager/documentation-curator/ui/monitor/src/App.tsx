import { useState } from "react";
import { MachineView } from "@declarative-agents/ui-kit";
import { useMonitor } from "./api";
import { EventsPanel, RawPanel, RunLine, RunPanel, StatusBanner, ToolsPanel } from "./panels";

// A single-page monitor, so no kit shell: tabs over one snapshot, the machine
// drawn by the kit MachineView and the rest by the curator panels.

type TabId = "graph" | "run" | "tools" | "events" | "raw";

const TABS: { id: TabId; label: string }[] = [
  { id: "graph", label: "Graph" },
  { id: "run", label: "Run" },
  { id: "tools", label: "Tools" },
  { id: "events", label: "Events" },
  { id: "raw", label: "Raw" },
];

export default function App() {
  const data = useMonitor();
  const [tab, setTab] = useState<TabId>("graph");
  const run = data.state?.run;
  const counts: Partial<Record<TabId, number>> = { tools: data.tools?.tools?.length, events: data.feed.length };

  return (
    <div className="app">
      <header className="app-header">
        <div className="app-brand">
          <span className="app-brand-title">Knowledge Manager Monitor</span>
          <span className="app-brand-sub">{data.machine?.name ?? "documentation-curator"}</span>
        </div>
        <nav className="app-nav">
          {TABS.map((t) => (
            <button key={t.id} className={`nav-tab${tab === t.id ? " nav-tab-active" : ""}`} onClick={() => setTab(t.id)}>
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
            <RunLine run={run} />
            <div className="panel">
              <MachineView spec={data.machine ?? {}} currentState={run?.state} activeEdge={data.lastTransition} />
            </div>
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
            <RawPanel title="State (JSON)" value={data.state} />
            <RawPanel title="Machine (JSON)" value={data.machine} />
          </>
        )}
      </main>
    </div>
  );
}

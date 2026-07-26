import { MONITORED_AGENTS, useAgentMonitor, type AgentMonitor, type MonitoredAgent } from "./monitorApi";
import TracePanel from "./TracePanel";
import { eventInWindow, useTurns } from "./turns";

function statusDotClass(status: AgentMonitor["status"]): string {
  if (status === "connected") return "dot dot-ok";
  if (status === "error") return "dot dot-err";
  return "dot dot-idle";
}

function AgentSubPanel({ agent }: { agent: MonitoredAgent }) {
  const monitor = useAgentMonitor(agent.name);
  const { turns, selectedId } = useTurns();
  const selectedTurn = turns.find((t) => t.id === selectedId);

  return (
    <div className="agent-panel">
      <div className="agent-head">
        <span className={statusDotClass(monitor.status)} />
        <span className="agent-name">{agent.label}</span>
        <span className="agent-state">{monitor.run?.state ?? "—"}</span>
      </div>
      <div className="agent-meta">
        <span>status: {monitor.run?.status ?? "—"}</span>
        <span>iter: {monitor.run?.iteration ?? "—"}</span>
        <span>metrics: {monitor.metricCount}</span>
      </div>
      <div className="agent-events">
        {monitor.runEvents.length === 0 ? (
          <div className="agent-empty">
            {monitor.status === "error" ? monitor.lastError ?? "unreachable" : "waiting for state transitions…"}
          </div>
        ) : (
          monitor.runEvents.map((e) => {
            const highlit = eventInWindow(selectedTurn, e.receivedAt);
            return (
              <div className={`event-row${highlit ? " event-row-hl" : ""}`} key={e.id}>
                <span className="event-kind">{e.commandName ?? "—"}</span>
                <span className="event-body">
                  {(e.fromState ?? "?")} → {(e.toState ?? "?")} <span className="event-signal">{e.signal}</span>
                </span>
              </div>
            );
          })
        )}
      </div>
    </div>
  );
}

function TurnSelector() {
  const { turns, selectedId, select } = useTurns();
  if (turns.length === 0) {
    return <div className="turn-bar turn-bar-empty">Send a chat turn to correlate its events across agents.</div>;
  }
  return (
    <div className="turn-bar">
      <span className="turn-bar-label">Correlate turn:</span>
      <button className={`turn-chip${selectedId === null ? " turn-chip-on" : ""}`} onClick={() => select(null)}>
        none
      </button>
      {turns.slice(0, 8).map((t) => (
        <button
          key={t.id}
          className={`turn-chip${selectedId === t.id ? " turn-chip-on" : ""}`}
          title={t.message}
          onClick={() => select(t.id)}
        >
          #{t.id + 1} {t.message.slice(0, 22)}
          {t.message.length > 22 ? "…" : ""}
        </button>
      ))}
    </div>
  );
}

function TraceSection() {
  const { turns, selectedId } = useTurns();
  const selectedTurn = turns.find((t) => t.id === selectedId);
  if (!selectedTurn) return null;
  return (
    <div className="trace-section">
      <div className="trace-head">Cross-agent trace — turn #{selectedTurn.id + 1}</div>
      <TracePanel traceId={selectedTurn.traceId} />
    </div>
  );
}

export default function ObservabilityPanel() {
  return (
    <div className="observability">
      <TurnSelector />
      <TraceSection />
      <div className="agent-grid">
        {MONITORED_AGENTS.map((agent) => (
          <AgentSubPanel key={agent.name} agent={agent} />
        ))}
      </div>
    </div>
  );
}

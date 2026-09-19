import { FleetPanel, TracePanel, type PanelProps } from "@declarative-agents/ui-kit";
import { useTurns } from "./turns";

// The observability panel composes two kit panels around the application's own
// turn store (applications srd004 R4): the kit FleetPanel shows one live
// sub-panel per ui.yaml monitored agent with the selected turn's window
// highlighted, and the kit TracePanel reads that turn's trace from the declared
// trace backend. Turns are chat state, so the selector stays here.

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

export default function ObservabilityPanel({ monitoredAgents, traceBackend }: PanelProps) {
  const { turns, selectedId } = useTurns();
  const selectedTurn = turns.find((t) => t.id === selectedId);
  return (
    <div className="observability">
      <TurnSelector />
      {selectedTurn && (
        <div className="dak-trace">
          <div className="trace-section">
            <div className="trace-head">Cross-agent trace — turn #{selectedTurn.id + 1}</div>
            <TracePanel
              traceId={selectedTurn.traceId}
              backend={traceBackend}
              unavailableHint="The per-agent monitor panels below stay live; enable the collector agent (Helm values) to see the cross-agent waterfall."
            />
          </div>
        </div>
      )}
      <FleetPanel agents={monitoredAgents} highlight={selectedTurn} />
    </div>
  );
}

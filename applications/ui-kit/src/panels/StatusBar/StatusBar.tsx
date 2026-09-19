import type { FleetSnapshot } from "../../hooks/useFleet";
import { useFleet } from "../../hooks/useFleet";
import "./statusBar.css";

// StatusBar summarizes the observer's fleet poll: connection, observer state,
// reachable agents, and the last poll time. Moved unchanged from the observer
// UIs, where it was byte-identical in all three repositories.
export function StatusBar({ snapshot }: { snapshot: FleetSnapshot }) {
  const reachable = snapshot.data.agents.filter((agent) => agent.reachable !== false).length;
  const connected = snapshot.observerState ? `Connected · observer ${snapshot.observerState}` : "Connected";
  const statusText =
    snapshot.status === "error"
      ? `Error: ${snapshot.error ?? "unknown"}`
      : snapshot.status === "connected"
        ? connected
        : snapshot.status === "polling"
          ? "Polling..."
          : "Connecting...";
  const dotClass = snapshot.status === "error" ? "dot-err" : snapshot.status === "connected" ? "dot-ok" : "dot-warn";

  return (
    <div className="dak-status-bar status-bar" aria-live="polite">
      <span>
        <span className={`dot ${dotClass}`} />
      </span>
      <span>{statusText}</span>
      <span>
        {reachable}/{snapshot.data.agents.length} reachable
      </span>
      <span className="last-poll">{snapshot.lastPoll ? `Last: ${snapshot.lastPoll.toLocaleTimeString()}` : ""}</span>
    </div>
  );
}

// StatusBarPanel is the mountable form: it polls the fleet itself.
export function StatusBarPanel() {
  return <StatusBar snapshot={useFleet()} />;
}

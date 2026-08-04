import type { FleetSnapshot } from "../useFleet";

export function StatusBar({ snapshot }: { snapshot: FleetSnapshot }) {
  const reachable = snapshot.data.agents.filter((agent) => agent.reachable !== false).length;
  const connected = snapshot.observerState
    ? `Connected · observer ${snapshot.observerState}`
    : "Connected";
  const statusText =
    snapshot.status === "error"
      ? `Error: ${snapshot.error ?? "unknown"}`
      : snapshot.status === "connected"
        ? connected
        : snapshot.status === "polling"
          ? "Polling..."
          : "Connecting...";
  const dotClass =
    snapshot.status === "error"
      ? "dot-err"
      : snapshot.status === "connected"
        ? "dot-ok"
        : "dot-warn";

  return (
    <div className="status-bar" aria-live="polite">
      <span><span className={`dot ${dotClass}`} /></span>
      <span>{statusText}</span>
      <span>{reachable}/{snapshot.data.agents.length} reachable</span>
      <span className="last-poll">
        {snapshot.lastPoll ? `Last: ${snapshot.lastPoll.toLocaleTimeString()}` : ""}
      </span>
    </div>
  );
}

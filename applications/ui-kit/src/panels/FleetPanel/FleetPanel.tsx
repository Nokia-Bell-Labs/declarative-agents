import type { MonitoredAgent } from "../../api/monitorApi";
import { useAgentMonitor, type AgentMonitor } from "../../hooks/useAgentMonitor";
import type { PanelProps } from "../manifest";
import "./fleetPanel.css";

// FleetPanel shows one live sub-panel per monitored agent, read through the
// monitor proxy: run state, iteration, metric count, and the recent run
// events. Base is agentic-wiki-mesh's FleetPanel, the same grid the
// chatbot-mesh and cohere-demo observability panels embed. An application
// passes the window of the turn it wants highlighted instead of the kit
// reading the application's turn store.

export interface HighlightWindow {
  startedAt: number;
  endedAt?: number;
}

export function eventInWindow(window: HighlightWindow | undefined, at: number, now: number = Date.now()): boolean {
  if (!window) return false;
  const end = window.endedAt ?? now;
  return at >= window.startedAt && at <= end + 500;
}

function statusDotClass(status: AgentMonitor["status"]): string {
  if (status === "connected") return "dot dot-ok";
  if (status === "error") return "dot dot-err";
  return "dot dot-idle";
}

function AgentSubPanel({ agent, highlight }: { agent: MonitoredAgent; highlight?: HighlightWindow }) {
  const monitor = useAgentMonitor(agent.name);

  // The declared list is a superset of the deployment; an agent the proxy
  // reports not deployed gets no panel at all (srd004 R2.2).
  if (monitor.status === "absent") return null;

  return (
    <div className="agent-panel" data-testid="agent-panel" data-agent={agent.name} data-status={monitor.status}>
      <div className="agent-head">
        <span className={statusDotClass(monitor.status)} />
        <span className="agent-name">{agent.label}</span>
        <span className="agent-state" data-testid="agent-state">
          {monitor.run?.state ?? "—"}
        </span>
      </div>
      <div className="agent-meta">
        <span>status: {monitor.run?.status ?? "—"}</span>
        <span>iter: {monitor.run?.iteration ?? "—"}</span>
        <span>metrics: {monitor.metricCount}</span>
      </div>
      <div className="agent-events">
        {monitor.runEvents.length === 0 ? (
          <div className="agent-empty">{monitor.status === "error" ? (monitor.lastError ?? "unreachable") : "waiting for state transitions…"}</div>
        ) : (
          monitor.runEvents.map((event) => (
            <div className={`event-row${eventInWindow(highlight, event.receivedAt) ? " event-row-hl" : ""}`} key={event.id}>
              <span className="event-kind">{event.commandName ?? "—"}</span>
              <span className="event-body">
                {event.fromState ?? "?"} → {event.toState ?? "?"} <span className="event-signal">{event.signal}</span>
              </span>
            </div>
          ))
        )}
      </div>
    </div>
  );
}

export function FleetPanel({ agents, highlight }: { agents: MonitoredAgent[]; highlight?: HighlightWindow }) {
  return (
    <div className="dak-fleet" data-testid="fleet">
      <div className="agent-grid">
        {agents.map((agent) => (
          <AgentSubPanel key={agent.name} agent={agent} highlight={highlight} />
        ))}
      </div>
    </div>
  );
}

// FleetPanelMount is the mountable form: the shell supplies the declared agents.
export function FleetPanelMount({ monitoredAgents }: PanelProps) {
  return <FleetPanel agents={monitoredAgents} />;
}

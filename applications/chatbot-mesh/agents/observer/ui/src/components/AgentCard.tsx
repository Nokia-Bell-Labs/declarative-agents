import type { FleetAgent, ResourceUsage } from "../fleet";

function stateClass(state: string): string {
  switch (state.toLowerCase()) {
    case "done":
    case "succeeded":
      return "state-done";
    case "failed":
      return "state-failed";
    case "idle":
    case "":
      return "state-idle";
    default:
      return "state-running";
  }
}

function shortTime(timestamp?: string): string {
  if (!timestamp) return "";
  const value = new Date(timestamp);
  return Number.isNaN(value.valueOf())
    ? timestamp
    : value.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit", second: "2-digit" });
}

function toolNames(tools: FleetAgent["tools"]): string[] {
  const values = Array.isArray(tools) ? tools : tools?.tools ?? [];
  return values.map((tool) => {
    if (typeof tool === "object" && tool !== null && "name" in tool) {
      return String((tool as { name: unknown }).name);
    }
    return String(tool);
  });
}

export function AgentCard({
  agent,
  resources,
}: {
  agent: FleetAgent;
  resources?: ResourceUsage;
}) {
  const stateValue = agent.state;
  const state = typeof stateValue === "object" ? stateValue.current_state ?? "" : stateValue ?? "";
  const name = agent.name ?? "unknown";
  const tools = toolNames(agent.tools);
  const events = agent.events ?? [];

  return (
    <article className="card">
      <div className="card-header">
        <span className="agent-name">{name}</span>
        {agent.reachable === false ? (
          <span className="agent-state state-failed">unreachable</span>
        ) : (
          <span className={`agent-state ${stateClass(state)}`}>{state || "unknown"}</span>
        )}
      </div>
      <div className="resources">
        {resources ? (
          <>
            <span>cpu {resources.cpu || "?"}</span>
            <span>mem {resources.memory || "?"}</span>
          </>
        ) : (
          <span className="res-unavailable">metrics unavailable</span>
        )}
      </div>
      {agent.machine?.purpose ? <p className="purpose">{agent.machine.purpose.split("\n")[0]}</p> : null}
      {agent.machine?.states ? (
        <>
          <div className="section-label">Machine</div>
          <dl>
            <div className="meta-row"><dt>States</dt><dd>{agent.machine.states.length}</dd></div>
            <div className="meta-row"><dt>Transitions</dt><dd>{agent.machine.transitions?.length ?? 0}</dd></div>
          </dl>
        </>
      ) : null}
      {tools.length ? (
        <>
          <div className="section-label">Tools</div>
          <div className="tool-list">
            {tools.map((tool) => <span className="tool-tag" key={tool}>{tool}</span>)}
          </div>
        </>
      ) : null}
      {events.length ? (
        <>
          <div className="section-label">Recent Events</div>
          <div className="events">
            {events.slice(-8).reverse().map((event, index) => (
              <div key={`${event.timestamp ?? event.time ?? ""}-${index}`}>
                <span className="ts">{shortTime(event.timestamp ?? event.time)}</span>{" "}
                <span className="signal">{event.signal ?? event.name ?? ""}</span>
              </div>
            ))}
          </div>
        </>
      ) : null}
    </article>
  );
}

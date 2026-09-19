import { useState, type ReactNode } from "react";
import type { FleetAgent, ResourceUsage } from "../../api/fleetApi";
import type { ServiceWalk } from "../../api/traceApi";
import { isFailureSignal, shortTime, toolRecords, yamlify } from "./cardModel";

// The sections of an agent card, one component each so the card itself stays
// a composition.

export type AgentMachine = NonNullable<FleetAgent["machine"]>;

// A machine renderer an application supplies (the kit MachineView, typically)
// in place of the card's own transition list.
export type RenderMachine = (machine: AgentMachine, currentState: string) => ReactNode;

export function ResourcesLine({ resources }: { resources?: ResourceUsage }) {
  return (
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
  );
}

function TransitionList({ machine, currentState }: { machine: AgentMachine; currentState: string }) {
  const transitions = machine.transitions ?? [];
  if (transitions.length === 0) return <div className="machine-empty">the monitor served no transitions</div>;
  return (
    <ol className="machine-transitions">
      {transitions.map((transition, index) => (
        <li key={`${transition.state ?? ""}-${transition.signal ?? ""}-${index}`} className={transition.state === currentState ? "transition-current" : undefined}>
          <span className="transition-state">{transition.state ?? "?"}</span>
          <span className="transition-signal">{transition.signal ?? "?"}</span>
          <span className="transition-state">{transition.next ?? "?"}</span>
          {transition.action ? <span className="transition-action">{transition.action}</span> : null}
        </li>
      ))}
    </ol>
  );
}

// MachineSection shows the supervisor the monitor serves: its counts (the
// wiki-mesh card's States / Transitions) as a toggle that opens the machine
// (cohere-demo GH-360), drawn by the application's renderer when given.
export function MachineSection({ machine, currentState, renderMachine }: { machine?: AgentMachine; currentState: string; renderMachine?: RenderMachine }) {
  const [open, setOpen] = useState(false);
  if (!machine?.states) return null;
  return (
    <>
      <div className="section-label">Supervisor Machine — declared</div>
      <button type="button" className="machine-toggle" aria-expanded={open} onClick={() => setOpen((was) => !was)}>
        {machine.states.length} states · {machine.transitions?.length ?? 0} transitions
      </button>
      {open ? <div className="machine-body">{renderMachine ? renderMachine(machine, currentState) : <TransitionList machine={machine} currentState={currentState} />}</div> : null}
    </>
  );
}

// ToolsSection lists the registered tools, colored by category; a click opens
// the monitor's record for that tool as YAML (cohere-demo GH-360).
export function ToolsSection({ tools }: { tools: FleetAgent["tools"] }) {
  const records = toolRecords(tools);
  const [openTool, setOpenTool] = useState<string>();
  if (records.length === 0) return null;
  const opened = records.find((tool) => tool.name === openTool);
  return (
    <>
      <div className="section-label">Tools</div>
      <div className="tool-list">
        {records.map((tool) => (
          <button
            type="button"
            className={`tool-tag tool-cat-${tool.category ?? "unstated"}`}
            key={tool.name}
            title={tool.category ? `category: ${tool.category}` : undefined}
            aria-pressed={openTool === tool.name}
            onClick={() => setOpenTool((was) => (was === tool.name ? undefined : tool.name))}
          >
            {tool.name}
          </button>
        ))}
      </div>
      {opened ? <pre className="tool-detail">{yamlify(opened.record)}</pre> : null}
    </>
  );
}

// WalkSection is the most recent request-scoped machine this agent ran, read
// from the collector's traces (cohere-demo GH-417). The supervisor above is
// near-identical across agents by design; this is the machine that did the
// work, and it exists only for the life of a request.
export function WalkSection({ walk }: { walk?: ServiceWalk }) {
  if (!walk || walk.steps.length === 0) return null;
  return (
    <>
      <div className="section-label">Last run · {walk.steps.length} steps</div>
      <ol className="walk-steps">
        {walk.steps.map((step) => (
          <li className="walk-step" key={step.iteration}>
            <span className="walk-iteration">{step.iteration}</span>
            <span className="walk-command">{step.command}</span>
            <span className={`walk-signal${isFailureSignal(step.signal) ? " walk-signal-failed" : ""}`}>{step.signal}</span>
          </li>
        ))}
      </ol>
    </>
  );
}

// EventsSection shows the last eight monitor events, newest first.
export function EventsSection({ events }: { events: FleetAgent["events"] }) {
  if (!events || events.length === 0) return null;
  return (
    <>
      <div className="section-label">Recent Events</div>
      <div className="events">
        {events
          .slice(-8)
          .reverse()
          .map((event, index) => (
            <div key={`${event.timestamp ?? event.time ?? ""}-${index}`}>
              <span className="ts">{shortTime(event.timestamp ?? event.time)}</span> <span className="signal">{event.signal ?? event.name ?? ""}</span>
            </div>
          ))}
      </div>
    </>
  );
}

import type { ReactNode } from "react";
import type { FleetAgent, ResourceUsage } from "../../api/fleetApi";
import type { ServiceWalk } from "../../api/traceApi";
import { agentState, agentStateClass } from "./cardModel";
import { EventsSection, MachineSection, ResourcesLine, ToolsSection, WalkSection, type RenderMachine } from "./CardSections";
import "./agentCard.css";

// One card per monitor-serving agent. Base is cohere-demo's card (short unit
// name, egress line, tool records, request walk) over the fleet-only card the
// agentic-wiki-mesh and chatbot-mesh observers share. cohere-demo's declared
// machine picker, stage line, and tool dialogs read /monitor/machines and
// /monitor/tools/declared, which are MachineView's endpoints, so an
// application slots that panel in through machineSlot instead.

export interface AgentCardProps {
  agent: FleetAgent;
  // The unit name every fleet surface leads with (cohere-demo GH-369); the
  // pod name stays as the card's detail line.
  shortName?: string;
  resources?: ResourceUsage;
  // The peers this agent was observed calling in recent traces (GH-376), the
  // same edges the topology figure draws.
  egress?: string[];
  // The richest recent request walk this agent ran (GH-417).
  walk?: ServiceWalk;
  // Draws the supervisor machine when its section opens; the default is a
  // transition list.
  renderMachine?: RenderMachine;
  // Replaces the supervisor machine section outright, for an application that
  // mounts the MachineView panel per agent.
  machineSlot?: ReactNode;
}

export function AgentCard({ agent, shortName, resources, egress, walk, renderMachine, machineSlot }: AgentCardProps) {
  const state = agentState(agent);
  const name = agent.name ?? "unknown";
  const unreachable = agent.reachable === false;
  return (
    <article className="dak-agent-card card" data-testid="agent-card" data-agent={name} data-reachable={unreachable ? "false" : "true"}>
      <div className="card-header">
        <span className="agent-name">{shortName || name}</span>
        {unreachable ? (
          <span className="agent-state state-failed">unreachable</span>
        ) : (
          <span className={`agent-state ${agentStateClass(state)}`}>{state || "unknown"}</span>
        )}
      </div>
      {shortName && shortName !== name ? <div className="agent-pod">{name}</div> : null}
      {egress && egress.length > 0 ? (
        <div className="agent-egress" title="Observed in recent traces — the same edges the topology figure draws">
          calls · {egress.join(" · ")}
        </div>
      ) : null}
      <ResourcesLine resources={resources} />
      {agent.machine?.purpose ? <p className="purpose">{agent.machine.purpose.split("\n")[0]}</p> : null}
      {machineSlot ?? <MachineSection machine={agent.machine} currentState={state} renderMachine={renderMachine} />}
      <ToolsSection tools={agent.tools} />
      <WalkSection walk={walk} />
      <EventsSection events={agent.events} />
    </article>
  );
}

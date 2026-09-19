import type { ReactNode } from "react";
import { metricsByPod, objectName, type FleetAgent, type FleetData } from "../../api/fleetApi";
import type { ServiceWalk } from "../../api/traceApi";
import { useFleet } from "../../hooks/useFleet";
import type { PanelProps } from "../manifest";
import { commonNamePrefix, shortUnitName } from "../Topology/names";
import { AgentCard } from "./AgentCard";
import { orderAgents } from "./cardModel";
import type { RenderMachine } from "./CardSections";

export const NO_AGENTS_TEXT = "No agents discovered yet. Waiting for mesh pods...";

export interface AgentCardsProps {
  data: FleetData;
  // Short unit names read first, in this order (cohere-demo GH-438); the rest
  // follow in discovery order. Empty keeps discovery order.
  order?: string[];
  // Observed egress and request walks keyed by short unit name, as
  // useTopology derives them; absent, the cards show fleet data only.
  egress?: Map<string, string[]>;
  walks?: Map<string, ServiceWalk>;
  renderMachine?: RenderMachine;
  machineSlot?: (agent: FleetAgent, shortName: string) => ReactNode;
  // Why the fleet is empty when it could not be read, instead of waiting.
  error?: string;
}

// AgentCards renders one card per fleet agent with its pod metrics.
export function AgentCards({ data, order, egress, walks, renderMachine, machineSlot, error }: AgentCardsProps) {
  if (data.agents.length === 0) {
    return (
      <div className="dak-agent-cards">
        <div className="empty" data-testid="agent-cards-empty">
          {error ? `Fleet unavailable: ${error}` : NO_AGENTS_TEXT}
        </div>
      </div>
    );
  }
  const metrics = metricsByPod(data.podMetrics);
  const prefix = commonNamePrefix(data.deployments.map((deployment) => objectName(deployment, "deployment")));
  return (
    <section className="dak-agent-cards grid" aria-label="Agents">
      {orderAgents(data.agents, prefix, order).map((agent, index) => {
        const short = shortUnitName(agent.name ?? "", prefix);
        return (
          <AgentCard
            key={`${agent.name ?? "unknown"}-${index}`}
            agent={agent}
            shortName={short}
            resources={metrics[agent.name ?? ""]}
            egress={egress?.get(short)}
            walk={walks?.get(short)}
            renderMachine={renderMachine}
            machineSlot={machineSlot?.(agent, short)}
          />
        );
      })}
    </section>
  );
}

function orderConfig(config: Record<string, unknown> | undefined): string[] | undefined {
  const value = config?.order;
  return Array.isArray(value) ? value.filter((entry): entry is string => typeof entry === "string") : undefined;
}

// AgentCardsMount is the mountable form: it polls the fleet and takes the
// reading order from the ui.yaml panel config.
export function AgentCardsMount({ config }: PanelProps) {
  const snapshot = useFleet();
  return <AgentCards data={snapshot.data} order={orderConfig(config)} error={snapshot.status === "error" ? snapshot.error : undefined} />;
}

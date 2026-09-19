import { metricsByPod, type FleetData } from "../../api/fleetApi";
import { useFleet } from "../../hooks/useFleet";
import type { PanelProps } from "../manifest";
import { ServiceList } from "./ServiceList";
import { TopologyGraphView } from "./TopologyGraphView";
import { useTopology, type TopologyRead } from "./topologyState";
import "./topology.css";

// The Topology section. Base is cohere-demo's figure derived from recent
// traces (GH-359, GH-376); when no trace can be read it degrades to the
// agentic-wiki-mesh / chatbot-mesh service list and says why, never to a
// broken panel.

export const TOPOLOGY_TRACE_BACKEND = "collector";

export interface TopologyProps {
  data: FleetData;
  read: TopologyRead;
  // The annotation key carrying each workload's declared role; no default,
  // since the key is the application's (cohere-demo: mesh.cohere-demo/role).
  roleAnnotation?: string;
  // Hover text for peers that are not deployed objects, keyed by node id.
  peerRoles?: Record<string, string>;
}

export function Topology({ data, read, roleAnnotation, peerRoles }: TopologyProps) {
  return (
    <section className="dak-topology topology" aria-labelledby="dak-topology-title" data-testid="topology">
      <h2 className="section-title" id="dak-topology-title">
        Topology
      </h2>
      {read.status === "ready" ? (
        <TopologyGraphView graph={read.state.graph} info={read.state.info} peerRoles={peerRoles} />
      ) : read.status === "deriving" ? (
        <div className="topo-graph-empty">deriving the graph from recent traces…</div>
      ) : (
        <>
          <div className="topo-graph-empty" data-testid="topology-reason">
            {read.reason} — showing the service list
          </div>
          <ServiceList data={data} metrics={metricsByPod(data.podMetrics)} roleAnnotation={roleAnnotation} />
        </>
      )}
    </section>
  );
}

function stringConfig(config: Record<string, unknown> | undefined, key: string): string | undefined {
  const value = config?.[key];
  return typeof value === "string" && value !== "" ? value : undefined;
}

function peerRolesConfig(config: Record<string, unknown> | undefined): Record<string, string> | undefined {
  const value = config?.peerRoles;
  if (typeof value !== "object" || value === null) return undefined;
  return Object.fromEntries(Object.entries(value).filter((entry): entry is [string, string] => typeof entry[1] === "string"));
}

// TopologyMount is the mountable form: it polls the fleet, reads recent traces
// from the declared trace backend, and takes roleAnnotation and peerRoles from
// the ui.yaml panel config.
export function TopologyMount({ config, traceBackend }: PanelProps) {
  const snapshot = useFleet();
  const roleAnnotation = stringConfig(config, "roleAnnotation");
  const read = useTopology(snapshot.data, { backend: traceBackend ?? TOPOLOGY_TRACE_BACKEND, roleAnnotation });
  return <Topology data={snapshot.data} read={read} roleAnnotation={roleAnnotation} peerRoles={peerRolesConfig(config)} />;
}

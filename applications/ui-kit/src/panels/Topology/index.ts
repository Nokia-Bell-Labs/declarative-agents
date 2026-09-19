import { definePanel } from "../manifest";
import { topologyManifest } from "./manifest";
import { TOPOLOGY_TRACE_BACKEND, Topology, TopologyMount, type TopologyProps } from "./Topology";

export { commonNamePrefix, roleOf, shortUnitName } from "./names";
export { ServiceList, serviceGroups, type TopologyServiceGroup } from "./ServiceList";
export {
  buildTopologyGraph,
  egressByAgent,
  OBSERVER_SERVICE,
  type TopologyEdge,
  type TopologyGraph,
  type TopologyNode,
  type TopologyNodeKind,
  type TopologySpan,
} from "./topologyGraph";
export { TopologyGraphView, type TopologyGraphViewProps } from "./TopologyGraphView";
export {
  buildTopologyState,
  NO_TRACES_REASON,
  useTopology,
  type TopologyNodeInfo,
  type TopologyRead,
  type TopologyState,
  type UseTopologyOptions,
} from "./topologyState";
export {
  fetchTopologyTraces,
  requestWalks,
  selectTopologyTraces,
  TOPOLOGY_MAX_SPANS,
  TOPOLOGY_TRACE_PAGE,
  type TopologyTraces,
} from "./traceReads";
export { TOPOLOGY_TRACE_BACKEND, Topology, TopologyMount, topologyManifest, type TopologyProps };
export const topologyPanel = definePanel(topologyManifest, TopologyMount);

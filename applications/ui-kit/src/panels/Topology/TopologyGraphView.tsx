import type { TopologyNodeInfo } from "./topologyState";
import type { TopologyGraph } from "./topologyGraph";

// The observed-topology figure (cohere-demo GH-359). The Docker host sits in
// the left column, agents next, the mesh's backing services after them, and
// external peers on the right, so "who talks to whom, and which agents reach
// out" reads left to right. Poll edges draw dashed and faint, egress edges
// amber, and each edge's title carries its observed call count. A deployed
// unit with no observed edge is drawn dimmed (GH-369).

const COL_X = [10, 240, 470, 700];
const COL_W = 190;
const ROW_H = 44;
const BOX_H = 28;
const KIND_COLUMN = { host: 0, agent: 1, service: 2, external: 3 } as const;

function nodeTitle(id: string, quiet: boolean, info?: TopologyNodeInfo, peerRoles?: Record<string, string>): string {
  const parts = [info?.role ?? peerRoles?.[id] ?? ""];
  if (info?.pod) parts.push(`pod ${info.pod}`);
  if (info?.resources) parts.push(info.resources);
  if (quiet) parts.push("deployed; no calls observed in the recent traces");
  return parts.filter(Boolean).join("\n");
}

function edgePath(from: { x: number; y: number }, to: { x: number; y: number }): string {
  const sameColumn = from.x === to.x;
  const x1 = from.x + (sameColumn ? COL_W / 2 : COL_W);
  const y1 = from.y + BOX_H / 2 + (sameColumn ? BOX_H / 2 : 0);
  const x2 = to.x + (sameColumn ? COL_W / 2 : -4);
  const y2 = to.y + BOX_H / 2 + (sameColumn ? -2 : 0);
  const bend = sameColumn ? 34 : (x2 - x1) / 3;
  return sameColumn
    ? `M ${x1} ${y1} C ${x1 + bend} ${y1 + 10}, ${x2 + bend} ${y2 - 10}, ${x2} ${y2}`
    : `M ${x1} ${y1} C ${x1 + bend} ${y1}, ${x2 - bend} ${y2}, ${x2} ${y2}`;
}

export interface TopologyGraphViewProps {
  graph: TopologyGraph;
  info?: Map<string, TopologyNodeInfo>;
  // Hover text for peers that are not deployed objects (an external API, the
  // Docker host), keyed by node id. Deployed units describe themselves
  // through their role annotation instead.
  peerRoles?: Record<string, string>;
}

export function TopologyGraphView({ graph, info, peerRoles }: TopologyGraphViewProps) {
  const rows = [0, 0, 0, 0];
  const position = new Map<string, { x: number; y: number }>();
  const ordered = [...graph.nodes].sort((a, b) => a.id.localeCompare(b.id));
  for (const node of ordered) {
    const column = KIND_COLUMN[node.kind];
    position.set(node.id, { x: COL_X[column], y: 10 + rows[column] * ROW_H });
    rows[column] += 1;
  }
  // Only columns that hold a node claim width.
  const usedColumns = [0, 1, 2, 3].filter((column) => rows[column] > 0);
  const height = Math.max(...rows) * ROW_H + 20;
  const width = (usedColumns.length > 0 ? COL_X[Math.max(...usedColumns)] : 0) + COL_W + 10;

  return (
    <div className="topo-graph" data-testid="topology-graph">
      <svg viewBox={`0 0 ${width} ${height}`} width={width} height={height} role="img" aria-label="Mesh topology derived from recent traces">
        <defs>
          <marker id="dak-topo-arrow" viewBox="0 0 8 8" refX="7" refY="4" markerWidth="6" markerHeight="6" orient="auto">
            <path className="topo-arrow" d="M0,0 L8,4 L0,8 z" />
          </marker>
        </defs>
        {graph.edges.map((edge) => {
          const from = position.get(edge.from);
          const to = position.get(edge.to);
          if (!from || !to) return null;
          return (
            <path key={`${edge.from} ${edge.to} ${edge.kind}`} d={edgePath(from, to)} className={`topo-edge topo-edge-${edge.kind}`} markerEnd="url(#dak-topo-arrow)">
              <title>{`${edge.from} → ${edge.to} · ${edge.count} call${edge.count === 1 ? "" : "s"} observed`}</title>
            </path>
          );
        })}
        {ordered.map((node) => {
          const at = position.get(node.id)!;
          const quiet = !graph.edges.some((edge) => edge.from === node.id || edge.to === node.id);
          return (
            <g key={node.id} className={`topo-node topo-node-${node.kind}${quiet ? " topo-node-quiet" : ""}`} data-node={node.id}>
              <title>{nodeTitle(node.id, quiet, info?.get(node.id), peerRoles)}</title>
              <rect x={at.x} y={at.y} width={COL_W} height={BOX_H} rx={node.kind === "external" ? 13 : 5} />
              <text x={at.x + COL_W / 2} y={at.y + BOX_H / 2 + 4} textAnchor="middle">
                {node.id}
              </text>
            </g>
          );
        })}
      </svg>
      <div className="topo-legend">
        <span className="topo-legend-host">docker host</span>
        <span className="topo-legend-agent">agent</span>
        <span className="topo-legend-service">backing service</span>
        <span className="topo-legend-external">external peer</span>
        <span className="topo-legend-egress">egress</span>
        <span className="topo-legend-poll">observer poll</span>
      </div>
    </div>
  );
}

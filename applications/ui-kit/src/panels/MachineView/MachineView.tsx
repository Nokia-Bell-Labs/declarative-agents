import { useId } from "react";
import type { MachineSpec } from "../../api/monitorApi";
import { layoutMachine } from "./machineLayout";
import "./machineView.css";

// The declared machine as a figure, with a turn's walk or the live run filled
// in. The boxes and arrows are the declaration; the fill is the join with what
// the agent did. Base is agentic-wiki-mesh's MachineView; cohere-demo's copy
// contributed the live current-state highlight, and the curator monitor's
// StateMachineGraph the highlighted last transition.

const SIBLING_W = 190;
const LEVEL_H = 64;
const BOX_W = 168;
const BOX_H = 30;
const PAD = 14;

export interface MachineEdgeRef {
  from: string;
  to: string;
}

export interface MachineViewProps {
  spec: MachineSpec;
  // Boxes every state of which the turn walked.
  visited?: Set<string>;
  // Boxes standing for several states, only some of which the turn walked.
  // A fold merges branches that a turn chooses between, so this is the
  // difference between "the turn was here" and "the turn was somewhere in
  // here".
  partly?: Set<string>;
  finalState?: string;
  // The state the live run is in now, from the agent's monitor.
  currentState?: string;
  // The transition the live run took last.
  activeEdge?: MachineEdgeRef;
}

export function MachineView({ spec, visited, partly, finalState, currentState, activeEdge }: MachineViewProps) {
  // One marker per figure: several figures on a page must not share an id.
  const arrowId = `dak-machine-arrow-${useId().replace(/[^a-zA-Z0-9_-]/g, "")}`;
  const layout = layoutMachine(spec);
  if (layout.states.length === 0) {
    return (
      <div className="dak-machine machine-empty" data-testid="machine-empty">
        The monitor served no machine states.
      </div>
    );
  }

  const perColumn = new Map<number, number>();
  for (const state of layout.states) {
    perColumn.set(state.column, (perColumn.get(state.column) ?? 0) + 1);
  }
  const widest = Math.max(...perColumn.values());
  const width = widest * SIBLING_W + PAD * 2;
  const height = layout.columns * LEVEL_H + PAD * 2;

  // Depth runs down the figure and siblings run across it, each column
  // centred, so a straight pipeline reads as a straight line.
  const centre = (state: { column: number; row: number }) => ({
    x: PAD + ((widest - (perColumn.get(state.column) ?? 1)) * SIBLING_W) / 2 + state.row * SIBLING_W + BOX_W / 2,
    y: PAD + state.column * LEVEL_H + BOX_H / 2,
  });
  const places = new Map(layout.states.map((state) => [state.name, centre(state)]));

  return (
    <div className="dak-machine machine-view" data-testid="machine-view">
      <svg viewBox={`0 0 ${width} ${height}`} width={width} height={height} role="img" aria-label={`State machine ${spec.name ?? ""}`}>
        <defs>
          <marker id={arrowId} markerWidth="9" markerHeight="7" refX="8" refY="3.5" orient="auto">
            <polygon className="machine-arrowhead" points="0 0, 9 3.5, 0 7" />
          </marker>
        </defs>
        {layout.edges.map((edge, index) => {
          const from = places.get(edge.from);
          const to = places.get(edge.to);
          if (!from || !to) return null;
          const dy = to.y - from.y;
          let path: string;
          if (edge.from === edge.to) {
            // A self-loop: the machine stayed where it was.
            path = `M ${from.x + BOX_W / 2} ${from.y - 6} C ${from.x + BOX_W / 2 + 28} ${from.y - 22}, ${from.x + BOX_W / 2 + 28} ${from.y + 22}, ${from.x + BOX_W / 2} ${from.y + 6}`;
          } else if (dy === 0) {
            const direction = to.x >= from.x ? 1 : -1;
            path = `M ${from.x + (direction * BOX_W) / 2} ${from.y} C ${from.x + direction * 44} ${from.y - 20}, ${to.x - direction * 44} ${to.y - 20}, ${to.x - (direction * BOX_W) / 2} ${to.y}`;
          } else {
            const startY = from.y + (dy >= 0 ? BOX_H / 2 : -BOX_H / 2);
            const endY = to.y - (dy >= 0 ? BOX_H / 2 + 4 : -(BOX_H / 2 + 4));
            const bend = dy / 4;
            path = `M ${from.x} ${startY} C ${from.x} ${startY + bend}, ${to.x} ${endY - bend}, ${to.x} ${endY}`;
          }
          const active = activeEdge !== undefined && activeEdge.from === edge.from && activeEdge.to === edge.to;
          return (
            <path
              className={active ? "machine-edge machine-edge-active" : "machine-edge"}
              key={`${edge.from}->${edge.to}:${index}`}
              d={path}
              markerEnd={`url(#${arrowId})`}
            >
              <title>{edge.label}</title>
            </path>
          );
        })}
        {layout.states.map((state) => {
          const place = places.get(state.name)!;
          const classes = [
            "machine-state",
            state.terminal ? "machine-state-terminal" : "",
            visited?.has(state.name) ? "machine-state-visited" : "",
            partly?.has(state.name) && !visited?.has(state.name) ? "machine-state-partial" : "",
            state.name === finalState ? "machine-state-final" : "",
            state.name === currentState ? "machine-state-current" : "",
          ]
            .filter(Boolean)
            .join(" ");
          return (
            <g key={state.name} className={classes} data-testid="machine-state" data-state={state.name}>
              <rect x={place.x - BOX_W / 2} y={place.y - BOX_H / 2} width={BOX_W} height={BOX_H} rx={state.terminal ? 14 : 5} />
              <text x={place.x} y={place.y + 4} textAnchor="middle">
                {state.name}
              </text>
            </g>
          );
        })}
      </svg>
    </div>
  );
}

export default MachineView;

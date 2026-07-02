import { useCallback, useEffect, useMemo, useState } from "react";
import dagre from "@dagrejs/dagre";
import {
  Background,
  Controls,
  Handle,
  MarkerType,
  MiniMap,
  Position,
  ReactFlow,
  type Edge,
  type Node,
  type NodeProps,
  type NodeTypes,
} from "@xyflow/react";
import "@xyflow/react/dist/style.css";
import type { MachineSnapshot, Transition } from "./api";

const NODE_WIDTH = 180;
const NODE_HEIGHT = 52;
const ACTIVE_MS = 2200;

interface StateNodeData extends Record<string, unknown> {
  label: string;
  sub?: string;
}

function StateNode({ data }: NodeProps<Node<StateNodeData>>) {
  return (
    <>
      <Handle type="target" position={Position.Top} />
      <div>
        {data.label}
        {data.sub ? <span className="sm-node-sub">{data.sub}</span> : null}
      </div>
      <Handle type="source" position={Position.Bottom} />
    </>
  );
}

const nodeTypes: NodeTypes = { smnode: StateNode };

function uniqueStates(machine: MachineSnapshot): string[] {
  const set = new Set<string>(machine.states ?? []);
  for (const t of machine.transitions ?? []) {
    if (t.state) set.add(t.state);
    if (t.next) set.add(t.next);
  }
  return [...set];
}

interface MergedEdge {
  source: string;
  target: string;
  signals: Set<string>;
  actions: Set<string>;
}

function mergeTransitions(transitions: Transition[]): MergedEdge[] {
  const byPair = new Map<string, MergedEdge>();
  for (const t of transitions) {
    if (!t.state || !t.next) continue;
    const key = `${t.state}\u0000${t.next}`;
    let edge = byPair.get(key);
    if (!edge) {
      edge = { source: t.state, target: t.next, signals: new Set(), actions: new Set() };
      byPair.set(key, edge);
    }
    if (t.signal) edge.signals.add(t.signal);
    if (t.action) edge.actions.add(t.action);
  }
  return [...byPair.values()];
}

function layout(machine: MachineSnapshot): { nodes: Node<StateNodeData>[]; edges: Edge[] } {
  const terminal = new Set(machine.terminal_states ?? []);
  const states = uniqueStates(machine);
  const merged = mergeTransitions(machine.transitions ?? []);

  const g = new dagre.graphlib.Graph();
  g.setDefaultEdgeLabel(() => ({}));
  g.setGraph({ rankdir: "TB", nodesep: 48, ranksep: 72, marginx: 16, marginy: 16 });
  for (const s of states) g.setNode(s, { width: NODE_WIDTH, height: NODE_HEIGHT });
  for (const e of merged) g.setEdge(e.source, e.target);
  dagre.layout(g);

  const nodes: Node<StateNodeData>[] = states.map((s) => {
    const pos = g.node(s);
    return {
      id: s,
      type: "smnode",
      position: { x: (pos?.x ?? 0) - NODE_WIDTH / 2, y: (pos?.y ?? 0) - NODE_HEIGHT / 2 },
      data: { label: s, sub: terminal.has(s) ? "terminal" : undefined },
      className: s === "Failed" ? "sm-failed" : terminal.has(s) ? "sm-terminal" : "",
    };
  });

  const edges: Edge[] = merged.map((e) => {
    const action = [...e.actions][0];
    const label = [...e.signals].join(" / ");
    return {
      id: `${e.source}->${e.target}`,
      source: e.source,
      target: e.target,
      label: action ? `${label}\n${action}` : label,
      labelBgPadding: [4, 2],
      labelBgStyle: { fill: "var(--bg-tertiary)" },
      markerEnd: { type: MarkerType.ArrowClosed, width: 16, height: 16 },
      className: e.target === "Failed" ? "sm-edge-fail" : "",
    };
  });

  return { nodes, edges };
}

interface Props {
  machine?: MachineSnapshot;
  currentState?: string;
  lastTransition?: { from: string; to: string; signal: string; at: number };
}

export default function StateMachineGraph({ machine, currentState, lastTransition }: Props) {
  const base = useMemo(() => (machine ? layout(machine) : { nodes: [], edges: [] }), [machine]);
  const [selected, setSelected] = useState<string | undefined>();
  const [activeAt, setActiveAt] = useState<number | undefined>(lastTransition?.at);

  useEffect(() => {
    if (!lastTransition?.at) return;
    setActiveAt(lastTransition.at);
    const timer = window.setTimeout(() => setActiveAt(undefined), ACTIVE_MS);
    return () => window.clearTimeout(timer);
  }, [lastTransition?.at]);

  const activeEdgeId =
    lastTransition && activeAt ? `${lastTransition.from}->${lastTransition.to}` : undefined;

  const nodes = useMemo<Node<StateNodeData>[]>(
    () =>
      base.nodes.map((n) => {
        const classes = [n.className ?? ""];
        if (n.id === currentState) classes.push("sm-current");
        if (n.id === selected) classes.push("sm-selected");
        return { ...n, className: classes.join(" ").trim(), selected: n.id === selected };
      }),
    [base.nodes, currentState, selected],
  );

  const edges = useMemo<Edge[]>(
    () =>
      base.edges.map((e) => {
        const active = e.id === activeEdgeId;
        return {
          ...e,
          animated: active,
          className: `${e.className ?? ""}${active ? " sm-edge-active" : ""}`.trim(),
        };
      }),
    [base.edges, activeEdgeId],
  );

  const onNodeClick = useCallback((_: unknown, node: Node) => {
    setSelected((prev) => (prev === node.id ? undefined : node.id));
  }, []);

  const outgoing = useMemo(
    () => (machine?.transitions ?? []).filter((t) => t.state === selected),
    [machine, selected],
  );

  return (
    <>
      <div className="graph-legend">
        <span className="graph-legend-item"><span className="graph-legend-swatch swatch-current" /> current state</span>
        <span className="graph-legend-item"><span className="graph-legend-swatch swatch-normal" /> state</span>
        <span className="graph-legend-item"><span className="graph-legend-swatch swatch-terminal" /> terminal</span>
        <span className="graph-legend-item"><span className="graph-legend-swatch swatch-failed" /> failure</span>
      </div>
      <div className="graph-panel">
        <ReactFlow
          nodes={nodes}
          edges={edges}
          nodeTypes={nodeTypes}
          onNodeClick={onNodeClick}
          nodesConnectable={false}
          edgesFocusable={false}
          fitView
          fitViewOptions={{ padding: 0.15 }}
          minZoom={0.3}
          proOptions={{ hideAttribution: true }}
        >
          <Background gap={20} color="var(--border)" />
          <Controls showInteractive={false} />
          <MiniMap pannable zoomable />
        </ReactFlow>
      </div>
      {selected ? (
        <p className="mono" style={{ marginTop: 12, color: "var(--text-secondary)" }}>
          {selected}: {outgoing.length
            ? outgoing.map((t) => `${t.signal} -> ${t.next}`).join("  |  ")
            : "no outgoing transitions"}
        </p>
      ) : (
        <p className="mono" style={{ marginTop: 12, color: "var(--text-tertiary)" }}>
          Drag to pan, scroll to zoom, click a state to inspect its transitions.
        </p>
      )}
    </>
  );
}

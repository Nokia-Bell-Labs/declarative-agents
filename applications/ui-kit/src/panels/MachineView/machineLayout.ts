import type { MachineSpec } from "../../api/monitorApi";

// Laying a declared machine out as a figure: states by how far they sit from
// the initial state, edges between them. The layout is hand-rolled rather than
// taken from a graph library, because an application SPA can ship in a
// ConfigMap and a layout engine would be most of the bundle. Base is
// agentic-wiki-mesh's machineLayout; cohere-demo's copy contributed the
// defensive string coercion of state names.

export interface LaidOutState {
  name: string;
  // Depth from the initial state: the row of the figure the state sits in.
  column: number;
  // Position among the states at that depth.
  row: number;
  terminal: boolean;
}

export interface LaidOutEdge {
  from: string;
  to: string;
  label: string;
}

export interface MachineLayout {
  states: LaidOutState[];
  edges: LaidOutEdge[];
  columns: number;
}

// layoutMachine assigns each state its shortest transition distance from the
// initial state, by relaxing every transition until nothing moves. A state no
// transition reaches -- an authored state the program cannot enter -- lands in
// a column of its own past the rest, where it reads as the loose end it is.
export function layoutMachine(spec: MachineSpec): MachineLayout {
  const names = (spec.states ?? []).map(String);
  if (names.length === 0) {
    return { states: [], edges: [], columns: 0 };
  }
  const transitions = spec.transitions ?? [];
  const terminal = new Set((spec.terminal_states ?? []).map(String));
  const known = new Set(names);
  const initial = spec.initial_state && known.has(spec.initial_state) ? spec.initial_state : names[0];

  const depth = new Map<string, number>([[initial, 0]]);
  for (let pass = 0; pass < names.length; pass++) {
    let changed = false;
    for (const transition of transitions) {
      const from = depth.get(String(transition.state ?? ""));
      if (from === undefined || !transition.next) continue;
      const proposed = from + 1;
      const existing = depth.get(String(transition.next));
      if (existing === undefined || proposed < existing) {
        depth.set(String(transition.next), proposed);
        changed = true;
      }
    }
    if (!changed) break;
  }

  const reached = [...depth.values()];
  const unreachableColumn = (reached.length === 0 ? 0 : Math.max(...reached)) + 1;
  const rows = new Map<number, number>();
  const states: LaidOutState[] = names.map((name) => {
    const column = depth.get(name) ?? unreachableColumn;
    const row = rows.get(column) ?? 0;
    rows.set(column, row + 1);
    return { name, column, row, terminal: terminal.has(name) };
  });

  const edges: LaidOutEdge[] = [];
  for (const transition of transitions) {
    const from = String(transition.state ?? "");
    const to = String(transition.next ?? "");
    if (!known.has(from) || !known.has(to)) continue;
    edges.push({ from, to, label: [transition.signal ?? "", transition.action ? `/ ${transition.action}` : ""].join(" ").trim() });
  }

  return { states, edges, columns: Math.max(...states.map((state) => state.column)) + 1 };
}

import type { MachineSpec, MachineTransition } from "../../api/monitorApi";

// A request machine of forty states drawn in full is a wall. Two reductions
// make it readable without inventing anything: drop the edges that only exist
// to carry a failure, and fold a run of states that can only be walked in one
// order into its head. Base is agentic-wiki-mesh's machineCollapse;
// cohere-demo's copy contributed the names of the hidden states.

// The signals a machine emits when something went wrong. Every one of these is
// an authored signal name from the declarations, not a guess: the failure
// edges of a request machine all carry one, and they triple the edge count of
// a figure whose subject is what the turn did.
const DEGRADATION_SIGNAL = /Error|Errored|Degraded|Failed|Refused|Denied|TimedOut|Dropped|Unavailable|Unauthorized|Throttled|Rejected/;

export interface HappyPath {
  spec: MachineSpec;
  hiddenEdges: number;
  hiddenStates: number;
  // The states the reduction hid, in declaration order, for a footnote that
  // names them rather than only counting them.
  hiddenStateNames: string[];
}

// happyPath keeps the edges a turn takes when nothing fails, and the states
// those edges reach. The initial state stays whatever happens, so the figure
// always has somewhere to start.
export function happyPath(spec: MachineSpec): HappyPath {
  const transitions = spec.transitions ?? [];
  const states = spec.states ?? [];
  const kept = transitions.filter((transition) => !DEGRADATION_SIGNAL.test(String(transition.signal ?? "")));
  const touched = new Set<string>();
  for (const transition of kept) {
    touched.add(String(transition.state ?? ""));
    touched.add(String(transition.next ?? ""));
  }
  const keptStates = states.filter((name, index) => index === 0 || touched.has(name));
  const hiddenStateNames = states.filter((name) => !keptStates.includes(name));
  return {
    spec: {
      ...spec,
      states: keptStates,
      terminal_states: (spec.terminal_states ?? []).filter((name) => keptStates.includes(name)),
      transitions: kept,
    },
    hiddenEdges: transitions.length - kept.length,
    hiddenStates: hiddenStateNames.length,
    hiddenStateNames,
  };
}

export interface CollapsedMachine {
  spec: MachineSpec;
  // The state a name is drawn as: itself, or the head of the chain that
  // absorbed it. An overlay of visited states maps through this, so a turn
  // that walked a folded state still marks the box the reader sees.
  drawnAs: (name: string) => string;
  // The declared states one drawn box stands for, head first. A box that
  // stands for four states and saw one of them walked has not been walked, and
  // an overlay that cannot tell the two apart overstates what the turn did.
  membersOf: (drawn: string) => string[];
  folded: number;
}

interface Degree {
  incoming: MachineTransition[];
  outgoing: MachineTransition[];
}

// collapseChains folds every maximal run of states that has one way in and one
// way out into the state that starts it, labelled with how many it swallowed.
// Nothing branches inside such a run, so the fold hides no choice the machine
// could have made.
export function collapseChains(spec: MachineSpec): CollapsedMachine {
  const states = spec.states ?? [];
  const transitions = spec.transitions ?? [];
  const terminal = new Set(spec.terminal_states ?? []);
  if (states.length === 0) {
    return { spec, drawnAs: (name) => name, membersOf: (box) => [box], folded: 0 };
  }

  const degrees = new Map<string, Degree>();
  for (const name of states) degrees.set(name, { incoming: [], outgoing: [] });
  for (const transition of transitions) {
    degrees.get(String(transition.state ?? ""))?.outgoing.push(transition);
    degrees.get(String(transition.next ?? ""))?.incoming.push(transition);
  }

  const absorbable = (name: string): boolean => {
    if (name === states[0] || terminal.has(name)) return false;
    const degree = degrees.get(name);
    if (!degree || degree.incoming.length !== 1 || degree.outgoing.length !== 1) return false;
    // A self-loop is a choice the machine makes, not a link in a chain.
    return degree.incoming[0].state !== name && degree.outgoing[0].next !== name;
  };

  const absorbedBy = new Map<string, string>();
  const swallowed = new Map<string, number>();
  const drawn: string[] = [];
  for (const name of states) {
    if (absorbedBy.has(name)) continue;
    drawn.push(name);
    let cursor = name;
    let count = 0;
    for (;;) {
      const next = degrees.get(cursor)?.outgoing[0]?.next;
      if (!next || !absorbable(String(next)) || absorbedBy.has(String(next))) break;
      absorbedBy.set(String(next), name);
      swallowed.set(name, ++count);
      cursor = String(next);
    }
  }

  const resolve = (name: string): string => absorbedBy.get(name) ?? name;
  const label = (name: string): string => {
    const count = swallowed.get(name) ?? 0;
    return count === 0 ? name : `${name} +${count}`;
  };

  const seen = new Set<string>();
  const rerouted: MachineTransition[] = [];
  for (const transition of transitions) {
    const from = resolve(String(transition.state ?? ""));
    const to = resolve(String(transition.next ?? ""));
    if (from === to && absorbedBy.has(String(transition.next ?? ""))) continue;
    const key = `${from}|${transition.signal ?? ""}|${to}`;
    if (seen.has(key)) continue;
    seen.add(key);
    rerouted.push({ ...transition, state: label(from), next: label(to) });
  }

  const members = new Map<string, string[]>();
  for (const name of states) {
    const head = label(resolve(name));
    members.set(head, [...(members.get(head) ?? []), name]);
  }

  return {
    spec: {
      ...spec,
      states: drawn.map(label),
      initial_state: spec.initial_state ? label(resolve(spec.initial_state)) : undefined,
      terminal_states: (spec.terminal_states ?? []).filter((name) => drawn.includes(name)).map(label),
      transitions: rerouted,
    },
    drawnAs: (name: string) => label(resolve(name)),
    membersOf: (box: string) => members.get(box) ?? [box],
    folded: absorbedBy.size,
  };
}

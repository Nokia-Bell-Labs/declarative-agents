import type { DeclaredMachine, MachineSpec } from "../../api/monitorApi";
import type { ServiceWalk } from "../../api/traceApi";
import type { CollapsedMachine } from "./machineCollapse";

// The joins between an agent's declared machines and what a turn did. The
// machines and words themselves are read through the kit client
// (fetchDeclaredMachines, fetchDeclaredTools in api/monitorApi); these are the
// pure functions the figure rests on. Base is agentic-wiki-mesh's machineViews;
// cohere-demo's declaredViews contributed the supervisor-last reading order.

export interface OrderOptions {
  // Move the machine the monitor serves first -- the agent's lifecycle
  // supervisor (srd033 R8) -- to the end, so the request workflows lead.
  supervisorLast?: boolean;
}

// orderedMachines puts one machine first, leaving the rest in the order the
// agent served them, so a picker opens on the machine the turn ran in.
export function orderedMachines(machines: DeclaredMachine[], preferred?: string, options: OrderOptions = {}): DeclaredMachine[] {
  let rest = machines;
  let tail: DeclaredMachine[] = [];
  if (options.supervisorLast && machines.length > 1 && machines[0].name !== preferred) {
    rest = machines.slice(1);
    tail = [machines[0]];
  }
  if (!preferred) return [...rest, ...tail];
  const first = rest.filter((machine) => machine.name === preferred);
  return [...first, ...rest.filter((machine) => machine.name !== preferred), ...tail];
}

// machineTools lists the words a machine's transitions dispatch, in first use
// order. A dispatch the runtime resolves at run time ($-prefixed) names no
// declaration to open.
export function machineTools(machine: MachineSpec): string[] {
  const out: string[] = [];
  for (const transition of machine.transitions ?? []) {
    const action = transition.action;
    if (!action || action.startsWith("$") || out.includes(action)) continue;
    out.push(action);
  }
  return out;
}

// visitedStates marks the states a walk actually ran in.
//
// The runtime reports, per executed command, the state the machine was in
// while it ran. So a transition whose action the walk ran marks that
// transition's own state, and only that one. Marking both ends -- as a figure
// that reasons about edges rather than commands would -- fills the state the
// command's signal moved to, which the machine reaches only if the next
// command also ran.
export function visitedStates(machine: MachineSpec, walk?: ServiceWalk): Set<string> {
  const visited = new Set<string>();
  if (!walk || walk.steps.length === 0) return visited;
  const ran = new Set(walk.steps.map((step) => step.command));
  for (const transition of machine.transitions ?? []) {
    if (transition.action && ran.has(transition.action) && transition.state) {
      visited.add(transition.state);
    }
  }
  // The machine was in its initial state to dispatch anything at all.
  const initial = machine.initial_state ?? machine.states?.[0];
  if (initial) visited.add(initial);
  return visited;
}

// finalState is where the walk left the machine: the state the last command's
// signal moved it to.
export function finalState(machine: MachineSpec, walk?: ServiceWalk): string | undefined {
  if (!walk || walk.steps.length === 0) return undefined;
  const last = walk.steps[walk.steps.length - 1];
  for (const transition of machine.transitions ?? []) {
    if (transition.action === last.command && transition.signal === last.signal) return transition.next;
  }
  for (const transition of machine.transitions ?? []) {
    if (transition.action === last.command) return transition.next;
  }
  return undefined;
}

// machineForWalk picks the machine a walk ran in: the one whose transitions
// dispatch the most of the words the walk recorded. An agent serves its
// lifecycle machine beside its request machines, and only the words say which
// of them a turn was actually in -- the request machine's name is not in the
// trace, and the server name a machine_request span carries names the door the
// request came through, not the program behind it.
export function machineForWalk(machines: DeclaredMachine[], walk?: ServiceWalk): DeclaredMachine | undefined {
  if (machines.length === 0) return undefined;
  if (!walk || walk.steps.length === 0) return machines[0];
  const ran = new Set(walk.steps.map((step) => step.command));
  let best = machines[0];
  let bestScore = -1;
  for (const machine of machines) {
    const dispatched = new Set((machine.transitions ?? []).map((transition) => transition.action).filter(Boolean) as string[]);
    let score = 0;
    for (const command of ran) if (dispatched.has(command)) score++;
    if (score > bestScore) {
      best = machine;
      bestScore = score;
    }
  }
  return best;
}

export interface WalkOverlay {
  // Boxes every state of which the turn walked.
  visited: Set<string>;
  // Boxes standing for several states, only some of which the turn walked.
  partly: Set<string>;
}

// walkOverlay maps the walked states through a fold. A folded box reads as
// walked only when the turn walked all of its members: a fold can merge two
// branches a turn chose between, and filling the box because one member ran
// would claim the turn took a path it did not.
export function walkOverlay(collapsed: CollapsedMachine, walked: Set<string>): WalkOverlay {
  const visited = new Set<string>();
  const partly = new Set<string>();
  for (const box of (collapsed.spec.states ?? []).map(String)) {
    const members = collapsed.membersOf(box);
    const ran = members.filter((member) => walked.has(member)).length;
    if (ran === members.length) visited.add(box);
    else if (ran > 0) partly.add(box);
  }
  return { visited, partly };
}

import type { KitClient } from "../../client/client";
import { MONITOR_MACHINES, toDeclaredMachine, type DeclaredMachine, type MachineSpec, type MachineTransition } from "../../api/monitorApi";
import type { ServiceWalk } from "../../api/traceApi";

// Stage views over a machine whose declaration tags its states. The monitor
// serves every machine as authored, view_tags and per-state tags included
// (srd033 R8); a machine authored at executor granularity reads as a handful
// of named stages when cut along those tags. Ported from cohere-demo's
// declaredViews (tagView, activeStage) and machineCollapse (sharedSink,
// withoutSink), re-expressed over the kit's DeclaredMachine. The kit's
// toDeclaredMachine reduces states to names, so the tags are kept beside it.

export interface ViewTag {
  tag: string;
  label: string;
}

export interface TaggedMachine extends DeclaredMachine {
  view_tags: ViewTag[];
  // The tags each state carries, keyed by state name.
  state_tags: Record<string, string[]>;
}

function strings(value: unknown): string[] {
  return Array.isArray(value) ? value.filter((entry): entry is string => typeof entry === "string") : [];
}

// toTaggedMachine normalizes one machine as the monitor serves it and keeps
// its stage tags. A machine served with plain state names has no tags, and
// every stage helper then treats it as one untagged figure.
export function toTaggedMachine(raw: unknown): TaggedMachine | undefined {
  const machine = toDeclaredMachine(raw);
  if (!machine) return undefined;
  const source = raw as { view_tags?: unknown; states?: unknown[] };
  const view_tags = (Array.isArray(source.view_tags) ? source.view_tags : [])
    .map((entry) => entry as Partial<ViewTag> | null)
    .filter((entry): entry is ViewTag => typeof entry?.tag === "string")
    .map((entry) => ({ tag: entry.tag, label: typeof entry.label === "string" ? entry.label : entry.tag }));
  const state_tags: Record<string, string[]> = {};
  for (const state of source.states ?? []) {
    if (state && typeof state === "object") {
      const { name, tags } = state as { name?: unknown; tags?: unknown };
      if (typeof name === "string") state_tags[name] = strings(tags);
    }
  }
  return { ...machine, view_tags, state_tags };
}

// fetchTaggedMachines reads one agent's machines with their stage tags through
// the kit client. An agent that is not deployed or does not answer yields none.
export async function fetchTaggedMachines(client: KitClient, agent: string): Promise<TaggedMachine[]> {
  try {
    const result = await client.getProxyJSON<unknown>(agent, MONITOR_MACHINES);
    if (!result.deployed || !Array.isArray(result.body)) return [];
    return result.body.map(toTaggedMachine).filter((machine): machine is TaggedMachine => machine !== undefined);
  } catch {
    return [];
  }
}

function tagsOf(machine: TaggedMachine, state: string): string[] {
  return machine.state_tags[state] ?? [];
}

// tagView cuts one named sub-machine out of the declaration: the states
// carrying the tag, and the transitions whose both ends carry it. A state no
// in-view transition leaves is drawn terminal -- within the view, it is where
// this part of the story ends.
export function tagView(machine: TaggedMachine, tag: string): MachineSpec {
  const states = machine.states.filter((state) => tagsOf(machine, state).includes(tag));
  const inView = new Set(states);
  const transitions: MachineTransition[] = (machine.transitions ?? []).filter(
    (transition) => inView.has(String(transition.state ?? "")) && inView.has(String(transition.next ?? "")),
  );
  const hasExit = new Set(transitions.map((transition) => String(transition.state)));
  return {
    name: `${machine.name} — ${tag}`,
    initial_state: states[0],
    states,
    terminal_states: states.filter((state) => !hasExit.has(state)),
    transitions,
  };
}

// sharedSink names the state present in every view tag, when the machine
// declares more than one tag. Tagging a state everywhere is the author saying
// "this belongs to no one stage" -- in practice the failure sink -- and
// drawing its edges into every stage view tangles all of them.
export function sharedSink(machine: TaggedMachine): string | undefined {
  if (machine.view_tags.length < 2) return undefined;
  const tags = machine.view_tags.map((view) => view.tag);
  const everywhere = machine.states.filter((state) => tags.every((tag) => tagsOf(machine, state).includes(tag)));
  return everywhere.length === 1 ? everywhere[0] : undefined;
}

export interface SinklessView {
  machine: TaggedMachine;
  sink?: string;
  // Transitions into or out of the sink that the stage figures no longer draw.
  sinkEdges: number;
}

// withoutSink removes the shared sink before the stage views cut the machine,
// and counts the edges that touched it.
export function withoutSink(machine: TaggedMachine): SinklessView {
  const sink = sharedSink(machine);
  if (!sink) return { machine, sinkEdges: 0 };
  const touches = (transition: MachineTransition) => transition.next === sink || transition.state === sink;
  const transitions = machine.transitions ?? [];
  return {
    machine: {
      ...machine,
      states: machine.states.filter((state) => state !== sink),
      terminal_states: machine.terminal_states.filter((state) => state !== sink),
      transitions: transitions.filter((transition) => !touches(transition)),
    },
    sink,
    sinkEdges: transitions.filter(touches).length,
  };
}

// activeStage names the view tag the walk last touched: replay the walk's
// commands over the declared transitions, take the last state reached, and
// return that state's first tag.
export function activeStage(machine: TaggedMachine, walk?: ServiceWalk): string | undefined {
  if (!walk || walk.steps.length === 0 || machine.view_tags.length === 0) return undefined;
  const byAction = new Map<string, MachineTransition>();
  for (const transition of machine.transitions ?? []) {
    if (transition.action) byAction.set(transition.action, transition);
  }
  let last: string | undefined;
  for (const step of walk.steps) {
    const transition = byAction.get(step.command);
    if (transition?.next) last = transition.next;
  }
  return last ? tagsOf(machine, last)[0] : undefined;
}

import { proxyPath, type KitClient, type ProxyResult } from "../client/client";

// Clients for an agent's monitor surface (agent-core srd033), read through the
// serving origin's monitor proxy (srd004 R2). Merged from the chatbot copies in
// chatbot-mesh, cohere-demo, and agentic-wiki-mesh plus agentic-wiki-mesh's
// declared-machine reader.

// MonitoredAgent names one agent a shell watches. The list comes from ui.yaml
// monitored_agents, never from a constant in the kit.
export interface MonitoredAgent {
  name: string;
  label: string;
}

export interface RunSnapshot {
  run_id?: string;
  status?: string;
  state?: string;
  signal?: string;
  iteration?: number;
  updated_at?: string;
}

export interface MonitorDiagnostic {
  stage?: string;
  message?: string;
  metric?: string;
  tool_name?: string;
  timestamp?: string;
}

export interface MonitorError {
  stage?: string;
  message?: string;
  command_name?: string;
  timestamp?: string;
}

// StateSnapshot is GET /monitor/state (srd033 R3.9).
export interface StateSnapshot {
  run?: RunSnapshot;
  diagnostics?: MonitorDiagnostic[];
  errors?: MonitorError[];
}

export interface MachineTransition {
  state?: string;
  signal?: string;
  next?: string;
  action?: string;
  metric_labels?: Record<string, string>;
}

export interface MachineSpec {
  name?: string;
  initial_state?: string;
  states?: string[];
  terminal_states?: string[];
  signals?: string[];
  transitions?: MachineTransition[];
}

// DeclaredMachine is one entry of GET /monitor/machines (srd033 R8) with its
// states reduced to names.
export interface DeclaredMachine extends MachineSpec {
  name: string;
  initial_state: string;
  states: string[];
  terminal_states: string[];
}

// DeclaredTool is one entry of GET /monitor/tools/declared (srd033 R9).
export interface DeclaredTool {
  name: string;
  [field: string]: unknown;
}

export interface MonitorEvent {
  id: number;
  kind: "run_event" | "metric_sample" | "notice";
  receivedAt: number;
  fromState?: string;
  toState?: string;
  signal?: string;
  commandName?: string;
  raw: string;
}

export type AgentStatus = "connecting" | "connected" | "error" | "absent";

export const MONITOR_STATE = "monitor/state";
export const MONITOR_MACHINES = "monitor/machines";
export const MONITOR_TOOLS_DECLARED = "monitor/tools/declared";
export const MONITOR_EVENTS_STREAM = "monitor/events/stream";

export const POLL_MS = 3000;
// An agent the release does not deploy answers 404 forever, so it is
// re-checked on this cadence instead: rarely enough to keep the console quiet,
// often enough that a unit added at run time appears without a reload.
export const ABSENT_POLL_MS = 60000;

// statusAfterStateFailure decides what a failed state poll means. HTTP 404
// identifies an agent the release does not deploy. Any other failure on a
// connected panel is a blip the next poll settles; before first contact it is
// an error, and that includes a failure after absent: an upstream that exists
// but refuses means the agent is deployed and down.
export function statusAfterStateFailure(prev: AgentStatus, httpStatus?: number): AgentStatus {
  if (httpStatus === 404) return "absent";
  return prev === "connected" ? "connected" : "error";
}

export function pollDelay(status: AgentStatus): number {
  return status === "absent" ? ABSENT_POLL_MS : POLL_MS;
}

// parseMonitorFrame shapes one SSE frame into a monitor event. A frame that is
// not JSON is kept raw.
export function parseMonitorFrame(id: number, kind: MonitorEvent["kind"], data: string, receivedAt: number): MonitorEvent {
  const event: MonitorEvent = { id, kind, receivedAt, raw: data };
  try {
    const parsed = JSON.parse(data) as Record<string, unknown>;
    if (typeof parsed.from_state === "string") event.fromState = parsed.from_state;
    if (typeof parsed.to_state === "string") event.toState = parsed.to_state;
    if (typeof parsed.signal === "string") event.signal = parsed.signal;
    if (typeof parsed.command_name === "string") event.commandName = parsed.command_name;
  } catch {
    // Kept raw.
  }
  return event;
}

export function fetchAgentState(client: KitClient, agent: string): Promise<ProxyResult<StateSnapshot>> {
  return client.getProxyJSON<StateSnapshot>(agent, MONITOR_STATE);
}

export function openAgentEventStream(client: KitClient, agent: string): EventSource {
  return client.openEventStream(proxyPath(agent, MONITOR_EVENTS_STREAM));
}

// toDeclaredMachine normalizes one machine as the monitor serves it. The
// declared-machines view serves each state as an object carrying its name and
// tags, while the single-machine view serves plain names; both reduce to names
// here so nothing downstream renders an object as a React child.
export function toDeclaredMachine(raw: unknown): DeclaredMachine | undefined {
  const machine = raw as (Omit<DeclaredMachine, "states"> & { states?: unknown }) | null;
  if (!machine || typeof machine.name !== "string" || !Array.isArray(machine.states) || !Array.isArray(machine.transitions)) {
    return undefined;
  }
  const states = machine.states
    .map((state) => (typeof state === "string" ? state : String((state as { name?: unknown })?.name ?? "")))
    .filter((name) => name !== "");
  return {
    ...machine,
    states,
    initial_state: machine.initial_state ?? states[0] ?? "",
    terminal_states: machine.terminal_states ?? [],
  };
}

// fetchDeclaredMachines reads every machine one agent declares. An agent that
// is not deployed or does not answer yields none, and the panel says so.
export async function fetchDeclaredMachines(client: KitClient, agent: string): Promise<DeclaredMachine[]> {
  try {
    const result = await client.getProxyJSON<unknown>(agent, MONITOR_MACHINES);
    if (!result.deployed || !Array.isArray(result.body)) return [];
    return result.body.map(toDeclaredMachine).filter((machine): machine is DeclaredMachine => machine !== undefined);
  } catch {
    return [];
  }
}

// fetchDeclaredTools reads the declarations behind an agent's words, keyed by
// name. It accepts the bare array and the older {tools: [...]} envelope.
export async function fetchDeclaredTools(client: KitClient, agent: string): Promise<Record<string, DeclaredTool>> {
  let body: unknown;
  try {
    const result = await client.getProxyJSON<unknown>(agent, MONITOR_TOOLS_DECLARED);
    if (!result.deployed) return {};
    body = result.body;
  } catch {
    return {};
  }
  const list = Array.isArray(body) ? body : Array.isArray((body as { tools?: unknown })?.tools) ? (body as { tools: unknown[] }).tools : [];
  const out: Record<string, DeclaredTool> = {};
  for (const entry of list) {
    const tool = entry as DeclaredTool | null;
    if (tool && typeof tool.name === "string") out[tool.name] = tool;
  }
  return out;
}

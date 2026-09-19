import type { FleetAgent } from "../../api/fleetApi";
import { shortUnitName } from "../Topology/names";

// Pure helpers behind the agent card, moved from the three observer UIs where
// stateClass and shortTime were identical and cohere-demo added the tool
// records and their YAML detail (GH-360).

// agentState reads the card's state string from either monitor shape: the
// flat string the fleet read produces, or the older { current_state } object.
export function agentState(agent: FleetAgent): string {
  const value = agent.state;
  return typeof value === "object" ? (value.current_state ?? "") : (value ?? "");
}

export function agentStateClass(state: string): string {
  switch (state.toLowerCase()) {
    case "done":
    case "succeeded":
      return "state-done";
    case "failed":
      return "state-failed";
    case "idle":
    case "":
      return "state-idle";
    default:
      return "state-running";
  }
}

export function shortTime(timestamp?: string): string {
  if (!timestamp) return "";
  const value = new Date(timestamp);
  return Number.isNaN(value.valueOf()) ? timestamp : value.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit", second: "2-digit" });
}

export interface ToolRecord {
  name: string;
  category?: string;
  record: unknown;
}

// toolRecords keeps the whole monitor record beside the name: the tag's color
// reads the category, and the click-open detail renders every field the
// monitor serves.
export function toolRecords(tools: FleetAgent["tools"]): ToolRecord[] {
  const values = Array.isArray(tools) ? tools : (tools?.tools ?? []);
  return values.map((tool) => {
    if (typeof tool === "object" && tool !== null && "name" in tool) {
      const record = tool as { name: unknown; category?: unknown };
      return { name: String(record.name), category: record.category ? String(record.category) : undefined, record: tool };
    }
    return { name: String(tool), record: tool };
  });
}

// yamlify renders a monitor tool record as YAML for the click-open detail: the
// declared contract the monitor serves, not the declaration file.
export function yamlify(value: unknown, indent = 0): string {
  const pad = "  ".repeat(indent);
  if (Array.isArray(value)) {
    if (value.length === 0) return `${pad}[]`;
    return value.map((entry) => (typeof entry === "object" && entry !== null ? `${pad}-\n${yamlify(entry, indent + 1)}` : `${pad}- ${String(entry)}`)).join("\n");
  }
  if (typeof value === "object" && value !== null) {
    const entries = Object.entries(value as Record<string, unknown>).filter(([, entry]) => entry !== null && entry !== undefined);
    if (entries.length === 0) return `${pad}{}`;
    return entries
      .map(([key, entry]) =>
        typeof entry === "object" && entry !== null && !(Array.isArray(entry) && entry.length === 0)
          ? `${pad}${key}:\n${yamlify(entry, indent + 1)}`
          : `${pad}${key}: ${Array.isArray(entry) ? "[]" : String(entry)}`,
      )
      .join("\n");
  }
  return `${pad}${String(value)}`;
}

// orderAgents puts the application's reading order on the fleet (cohere-demo
// GH-438): agents whose short name appears in order come first, in that
// order; the rest keep their discovery order after them.
export function orderAgents<T extends { name?: string }>(agents: T[], prefix: string, order: string[] = []): T[] {
  if (order.length === 0) return agents;
  const rank = (agent: T): number => {
    const index = order.indexOf(shortUnitName(agent.name ?? "", prefix));
    return index === -1 ? order.length : index;
  };
  return [...agents].sort((a, b) => rank(a) - rank(b));
}

// Signals a walk step reads as a failure.
export function isFailureSignal(signal: string): boolean {
  return /Failed|Error|Refused|Denied/.test(signal);
}

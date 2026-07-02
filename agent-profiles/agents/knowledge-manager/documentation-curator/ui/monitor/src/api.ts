import { useEffect, useRef, useState } from "react";

export const paths = {
  state: "/monitor/state",
  machine: "/monitor/machine",
  tools: "/monitor/tools",
  events: "/monitor/events",
  stream: "/monitor/events/stream",
} as const;

export interface Transition {
  state: string;
  signal: string;
  next: string;
  action?: string;
  metric_labels?: Record<string, string>;
}

export interface MachineSnapshot {
  name?: string;
  signals?: string[];
  states?: string[];
  terminal_states?: string[];
  transitions?: Transition[];
  metric_labels?: Record<string, string>;
}

export interface RunSnapshot {
  run_id?: string;
  status?: string;
  state?: string;
  signal?: string;
  iteration?: number;
  updated_at?: string;
}

export interface StateSnapshot {
  run?: RunSnapshot;
  diagnostics?: unknown[];
  errors?: unknown[];
}

export interface ToolEntry {
  name?: string;
  category?: string;
  visibility?: string;
  emits?: string[];
}

export interface ToolsSnapshot {
  tools?: ToolEntry[];
}

export interface EventsSnapshot {
  recent_events?: Array<Record<string, unknown>>;
}

export interface RunEvent {
  from_state?: string;
  to_state?: string;
  signal?: string;
  command_name?: string;
  iteration?: number;
  duration_ms?: number;
  timestamp?: string;
}

export interface FeedItem {
  id: number;
  kind: string;
  receivedAt: string;
  data: string;
}

export interface MonitorData {
  status: "loading" | "connected" | "error";
  error?: string;
  state?: StateSnapshot;
  machine?: MachineSnapshot;
  tools?: ToolsSnapshot;
  events?: EventsSnapshot;
  feed: FeedItem[];
  lastTransition?: { from: string; to: string; signal: string; at: number };
}

async function fetchJSON<T>(url: string): Promise<T> {
  const res = await fetch(url);
  if (!res.ok) throw new Error(`${url} -> HTTP ${res.status}`);
  return (await res.json()) as T;
}

const POLL_MS = 4000;
const MAX_FEED = 200;

export function useMonitor(): MonitorData {
  const [data, setData] = useState<MonitorData>({ status: "loading", feed: [] });
  const feedId = useRef(0);

  useEffect(() => {
    let active = true;

    async function refresh() {
      try {
        const [state, machine, tools, events] = await Promise.all([
          fetchJSON<StateSnapshot>(paths.state),
          fetchJSON<MachineSnapshot>(paths.machine),
          fetchJSON<ToolsSnapshot>(paths.tools),
          fetchJSON<EventsSnapshot>(paths.events),
        ]);
        if (!active) return;
        setData((prev) => ({ ...prev, status: "connected", error: undefined, state, machine, tools, events }));
      } catch (err) {
        if (!active) return;
        const message = err instanceof Error ? err.message : String(err);
        setData((prev) => ({ ...prev, status: "error", error: message }));
      }
    }

    void refresh();
    const timer = window.setInterval(() => void refresh(), POLL_MS);

    const es = new EventSource(paths.stream);
    const push = (kind: string) => (ev: MessageEvent) => {
      const item: FeedItem = {
        id: feedId.current++,
        kind,
        receivedAt: new Date().toISOString(),
        data: ev.data,
      };
      setData((prev) => {
        const next: MonitorData = { ...prev, feed: [item, ...prev.feed].slice(0, MAX_FEED) };
        if (kind === "run_event") {
          try {
            const re = JSON.parse(ev.data) as RunEvent;
            if (re.from_state && re.to_state) {
              next.lastTransition = {
                from: re.from_state,
                to: re.to_state,
                signal: re.signal ?? "",
                at: Date.now(),
              };
            }
          } catch {
            /* ignore malformed event payloads */
          }
        }
        return next;
      });
    };
    es.addEventListener("run_event", push("run_event"));
    es.addEventListener("metric_sample", push("metric_sample"));
    es.onerror = () => {
      setData((prev) => ({
        ...prev,
        feed: [
          { id: feedId.current++, kind: "eventsource", receivedAt: new Date().toISOString(), data: "connection error (retrying)" },
          ...prev.feed,
        ].slice(0, MAX_FEED),
      }));
    };

    return () => {
      active = false;
      window.clearInterval(timer);
      es.close();
    };
  }, []);

  return data;
}

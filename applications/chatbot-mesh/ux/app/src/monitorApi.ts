import { useEffect, useRef, useState } from "react";

// The monitored agents. This list mirrors ui/ux.yaml monitored_agents and the
// rest.yaml monitor_proxy upstreams; keep the three in sync. The panel reads each
// agent's monitor through the chatbot's same-origin proxy, so no agent binds a
// cross-origin request from the browser.
export interface MonitoredAgent {
  name: string;
  label: string;
}

export const MONITORED_AGENTS: MonitoredAgent[] = [
  { name: "chatbot", label: "Chatbot" },
  { name: "rag0", label: "RAG server 0" },
  { name: "rag1", label: "RAG server 1" },
];

export function monitorPath(agent: string, suffix: string): string {
  return `/monitor-proxy/${agent}/${suffix}`;
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

export interface AgentMonitor {
  status: "connecting" | "connected" | "error";
  run?: RunSnapshot;
  // Run events (state transitions) are the panel's focus; they are kept in their
  // own capped buffer so the far more frequent metric samples cannot evict them.
  runEvents: MonitorEvent[];
  metricCount: number;
  lastError?: string;
}

const POLL_MS = 3000;
const MAX_EVENTS = 200;

async function fetchJSON<T>(url: string): Promise<T> {
  const res = await fetch(url);
  if (!res.ok) throw new Error(`${url} -> HTTP ${res.status}`);
  return (await res.json()) as T;
}

// useAgentMonitor subscribes to one agent's monitor: a periodic state poll plus a
// live SSE feed of run events and metric samples, all through the same-origin
// monitor proxy. The monitor stream returns one frame per request, so the
// EventSource reconnects on its own between events.
export function useAgentMonitor(agent: string): AgentMonitor {
  const [data, setData] = useState<AgentMonitor>({ status: "connecting", runEvents: [], metricCount: 0 });
  const eventId = useRef(0);

  useEffect(() => {
    let active = true;

    async function refresh() {
      try {
        const state = await fetchJSON<StateSnapshot>(monitorPath(agent, "monitor/state"));
        if (!active) return;
        setData((prev) => ({ ...prev, status: "connected", lastError: undefined, run: state.run }));
      } catch (err) {
        if (!active) return;
        const message = err instanceof Error ? err.message : String(err);
        setData((prev) => ({ ...prev, status: prev.status === "connected" ? "connected" : "error", lastError: message }));
      }
    }

    void refresh();
    const timer = window.setInterval(() => void refresh(), POLL_MS);

    const es = new EventSource(monitorPath(agent, "monitor/events/stream"));
    const push = (kind: MonitorEvent["kind"]) => (ev: MessageEvent) => {
      let parsed: Record<string, unknown> = {};
      try {
        parsed = JSON.parse(ev.data) as Record<string, unknown>;
      } catch {
        /* keep raw only */
      }
      if (kind === "metric_sample") {
        setData((prev) => ({ ...prev, status: "connected", metricCount: prev.metricCount + 1 }));
        return;
      }
      const item: MonitorEvent = {
        id: eventId.current++,
        kind,
        receivedAt: Date.now(),
        fromState: parsed.from_state as string | undefined,
        toState: parsed.to_state as string | undefined,
        signal: parsed.signal as string | undefined,
        commandName: parsed.command_name as string | undefined,
        raw: ev.data,
      };
      setData((prev) => ({ ...prev, status: "connected", runEvents: [item, ...prev.runEvents].slice(0, MAX_EVENTS) }));
    };
    es.addEventListener("run_event", push("run_event"));
    es.addEventListener("metric_sample", push("metric_sample"));
    es.onerror = () => {
      // The monitor stream closes after each frame; the browser reconnects. Only
      // surface a lasting error if we never connected.
      setData((prev) => (prev.status === "connected" ? prev : { ...prev, status: "error" }));
    };

    return () => {
      active = false;
      window.clearInterval(timer);
      es.close();
    };
  }, [agent]);

  return data;
}

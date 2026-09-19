import { useEffect, useRef, useState } from "react";
import {
  parseMonitorFrame,
  toDeclaredMachine,
  useKitClient,
  type MachineEdgeRef,
  type MachineSpec,
  type StateSnapshot,
} from "@declarative-agents/ui-kit";

// The curator serves this monitor from its own monitor listener, so every read
// is same-origin through the kit client (srd004 R6.1). /monitor/machine and
// /monitor/tools sit outside the presentation contract; a domain UI may read
// them directly (srd004 R1.3).
export const paths = {
  state: "/monitor/state",
  machine: "/monitor/machine",
  tools: "/monitor/tools",
  events: "/monitor/events",
  stream: "/monitor/events/stream",
} as const;

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
  machine?: MachineSpec;
  tools?: ToolsSnapshot;
  events?: EventsSnapshot;
  feed: FeedItem[];
  lastTransition?: MachineEdgeRef;
}

const POLL_MS = 4000;
const MAX_FEED = 200;

export function useMonitor(): MonitorData {
  const client = useKitClient();
  const [data, setData] = useState<MonitorData>({ status: "loading", feed: [] });
  const feedId = useRef(0);

  useEffect(() => {
    let active = true;

    async function refresh() {
      try {
        const [state, machine, tools, events] = await Promise.all([
          client.getJSON<StateSnapshot>(paths.state),
          client.getJSON<unknown>(paths.machine),
          client.getJSON<ToolsSnapshot>(paths.tools),
          client.getJSON<EventsSnapshot>(paths.events),
        ]);
        if (!active) return;
        setData((prev) => ({ ...prev, status: "connected", error: undefined, state, machine: toDeclaredMachine(machine), tools, events }));
      } catch (err) {
        if (!active) return;
        const message = err instanceof Error ? err.message : String(err);
        setData((prev) => ({ ...prev, status: "error", error: message }));
      }
    }

    const append = (prev: MonitorData, kind: string, text: string): MonitorData => {
      const item: FeedItem = { id: feedId.current++, kind, receivedAt: new Date().toISOString(), data: text };
      return { ...prev, feed: [item, ...prev.feed].slice(0, MAX_FEED) };
    };

    void refresh();
    const timer = window.setInterval(() => void refresh(), POLL_MS);

    const es = client.openEventStream(paths.stream);
    const push = (kind: "run_event" | "metric_sample") => (ev: MessageEvent<string>) => {
      setData((prev) => {
        const next = append(prev, kind, ev.data);
        if (kind !== "run_event") return next;
        const frame = parseMonitorFrame(0, kind, ev.data, Date.now());
        if (frame.fromState && frame.toState) next.lastTransition = { from: frame.fromState, to: frame.toState };
        return next;
      });
    };
    es.addEventListener("run_event", push("run_event"));
    es.addEventListener("metric_sample", push("metric_sample"));
    es.onerror = () => setData((prev) => append(prev, "eventsource", "connection error (retrying)"));

    return () => {
      active = false;
      window.clearInterval(timer);
      es.close();
    };
  }, [client]);

  return data;
}

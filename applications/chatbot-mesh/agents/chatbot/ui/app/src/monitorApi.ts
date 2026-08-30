import { useEffect, useRef, useState } from "react";

// The monitored agents. This list mirrors ui.yaml monitored_agents and the
// rest.yaml monitor_proxy upstreams; keep the three in sync. The panel reads each
// agent's monitor through the chatbot's same-origin proxy, so no agent binds a
// cross-origin request from the browser. The list is a superset of what any one
// release deploys: the chart declares proxy upstreams only for its deployed rag
// units, the proxy answers HTTP 404 for the rest, and the panel reads that 404
// as "not deployed" rather than an error.
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
  status: "connecting" | "connected" | "error" | "absent";
  run?: RunSnapshot;
  // Run events (state transitions) are the panel's focus; they are kept in their
  // own capped buffer so the far more frequent metric samples cannot evict them.
  runEvents: MonitorEvent[];
  metricCount: number;
  lastError?: string;
}

const POLL_MS = 3000;
// An agent the release does not deploy answers 404 forever, so it is re-checked
// on this cadence instead — rarely enough to keep the console quiet, often
// enough that a unit added at run time appears without a reload.
const ABSENT_POLL_MS = 60000;
const MAX_EVENTS = 200;

export class HTTPError extends Error {
  constructor(
    url: string,
    readonly status: number,
  ) {
    super(`${url} -> HTTP ${status}`);
  }
}

async function fetchJSON<T>(url: string): Promise<T> {
  const res = await fetch(url);
  if (!res.ok) throw new HTTPError(url, res.status);
  return (await res.json()) as T;
}

// statusAfterStateFailure decides what a failed state poll means. The monitor
// proxy declares one upstream per deployed agent, so HTTP 404 identifies an
// agent the release does not deploy. Any other failure on a connected panel is
// a blip the next poll settles; before first contact it is an error, and that
// includes a failure after absent — an upstream that exists but refuses means
// the agent is deployed and down, which must read red.
export function statusAfterStateFailure(
  prev: AgentMonitor["status"],
  httpStatus?: number,
): AgentMonitor["status"] {
  if (httpStatus === 404) return "absent";
  return prev === "connected" ? "connected" : "error";
}

// pollDelay picks the cadence for the next state poll.
//
// An absent agent is still re-checked, because the provisioning flow can add a
// RAG unit at run time and the panel should pick it up without a reload. What
// it must not do is ask on the live cadence: the answer is a 404, the browser
// logs every one of them, and twenty wasted requests a minute per absent agent
// is the difference between a quiet console and a scrolling one.
export function pollDelay(status: AgentMonitor["status"]): number {
  return status === "absent" ? ABSENT_POLL_MS : POLL_MS;
}

// useAgentMonitor subscribes to one agent's monitor: a periodic state poll plus a
// live SSE feed of run events and metric samples, all through the same-origin
// monitor proxy. The monitor stream returns one frame per request, so the
// EventSource reconnects on its own between events.
export function useAgentMonitor(agent: string): AgentMonitor {
  const [data, setData] = useState<AgentMonitor>({ status: "connecting", runEvents: [], metricCount: 0 });
  const eventId = useRef(0);
  // The scheduler reads the status through a ref because it lives inside an
  // effect that must not re-run when the status changes.
  const statusRef = useRef<AgentMonitor["status"]>("connecting");
  statusRef.current = data.status;

  useEffect(() => {
    let active = true;
    let es: EventSource | null = null;

    async function refresh() {
      try {
        const state = await fetchJSON<StateSnapshot>(monitorPath(agent, "monitor/state"));
        if (!active) return;
        setData((prev) => ({ ...prev, status: "connected", lastError: undefined, run: state.run }));
        openStream();
      } catch (err) {
        if (!active) return;
        const message = err instanceof Error ? err.message : String(err);
        const httpStatus = err instanceof HTTPError ? err.status : undefined;
        setData((prev) => {
          const status = statusAfterStateFailure(prev.status, httpStatus);
          return { ...prev, status, lastError: status === "absent" ? undefined : message };
        });
        if (httpStatus === 404) closeStream();
      }
    }

    function closeStream() {
      es?.close();
      es = null;
    }

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

    // The stream opens on first contact and reopens if a state poll succeeds
    // after an absence, so an agent provisioned mid-session gets its live feed
    // back without a reload. An absent agent has no stream to hold open.
    function openStream() {
      if (es) return;
      es = new EventSource(monitorPath(agent, "monitor/events/stream"));
      es.addEventListener("run_event", push("run_event"));
      es.addEventListener("metric_sample", push("metric_sample"));
      es.onerror = () => {
        // The monitor stream closes after each frame; the browser reconnects. Only
        // surface a lasting error if we never connected, and never over an absence.
        setData((prev) => (prev.status === "connected" || prev.status === "absent" ? prev : { ...prev, status: "error" }));
      };
    }

    // A self-scheduling timeout rather than a fixed interval, so the cadence can
    // change when the status does. statusRef carries the current status into the
    // scheduler without making the effect depend on it, which would tear the
    // stream down and rebuild it on every poll.
    let timer: number | undefined;
    function scheduleRefresh() {
      timer = window.setTimeout(async () => {
        await refresh();
        if (active) scheduleRefresh();
      }, pollDelay(statusRef.current));
    }

    void refresh();
    scheduleRefresh();
    openStream();

    return () => {
      active = false;
      if (timer !== undefined) window.clearTimeout(timer);
      closeStream();
    };
  }, [agent]);

  return data;
}

import { useEffect, useRef, useState } from "react";
import { HTTPError } from "../client/client";
import { useKitClient } from "../client/context";
import {
  fetchAgentState,
  openAgentEventStream,
  parseMonitorFrame,
  pollDelay,
  statusAfterStateFailure,
  type AgentStatus,
  type MonitorEvent,
  type RunSnapshot,
} from "../api/monitorApi";

export interface AgentMonitor {
  status: AgentStatus;
  run?: RunSnapshot;
  // Run events (state transitions) are kept in their own capped buffer so the
  // far more frequent metric samples cannot evict them.
  runEvents: MonitorEvent[];
  metricCount: number;
  lastError?: string;
}

export const MAX_MONITOR_EVENTS = 200;

// useAgentMonitor subscribes to one agent's monitor: a periodic state poll plus
// a live SSE feed of run events and metric samples, all through the monitor
// proxy. The stream opens after first contact and closes while the agent is
// not deployed.
export function useAgentMonitor(agent: string): AgentMonitor {
  const client = useKitClient();
  const [data, setData] = useState<AgentMonitor>({ status: "connecting", runEvents: [], metricCount: 0 });
  const eventId = useRef(0);
  const statusRef = useRef<AgentStatus>("connecting");
  statusRef.current = data.status;

  useEffect(() => {
    let active = true;
    let stream: EventSource | null = null;
    let timer: ReturnType<typeof setTimeout> | undefined;

    const openStream = () => {
      if (stream || !active) return;
      stream = openAgentEventStream(client, agent);
      const push = (kind: MonitorEvent["kind"]) => (message: MessageEvent) => {
        if (!active) return;
        const event = parseMonitorFrame(eventId.current++, kind, String(message.data), Date.now());
        setData((prev) =>
          kind === "metric_sample"
            ? { ...prev, metricCount: prev.metricCount + 1 }
            : { ...prev, runEvents: [event, ...prev.runEvents].slice(0, MAX_MONITOR_EVENTS) },
        );
      };
      stream.addEventListener("run_event", push("run_event") as EventListener);
      stream.addEventListener("metric_sample", push("metric_sample") as EventListener);
      // The stream returns one frame per request; the browser reconnects.
      stream.onerror = () => undefined;
    };

    const closeStream = () => {
      stream?.close();
      stream = null;
    };

    const setStatus = (status: AgentStatus, lastError?: string) => {
      statusRef.current = status;
      setData((prev) => ({ ...prev, status, lastError }));
    };

    const poll = async () => {
      if (!active) return;
      try {
        const result = await fetchAgentState(client, agent);
        if (!active) return;
        if (!result.deployed) {
          setStatus("absent", `${agent} is not deployed`);
          closeStream();
        } else {
          statusRef.current = "connected";
          setData((prev) => ({ ...prev, status: "connected", run: result.body.run, lastError: undefined }));
          openStream();
        }
      } catch (err) {
        if (!active) return;
        const status = statusAfterStateFailure(statusRef.current, err instanceof HTTPError ? err.status : undefined);
        setStatus(status, err instanceof Error ? err.message : String(err));
      }
      timer = setTimeout(poll, pollDelay(statusRef.current));
    };
    void poll();

    return () => {
      active = false;
      if (timer) clearTimeout(timer);
      closeStream();
    };
  }, [agent, client]);

  return data;
}

import { useEffect, useState } from "react";
import { useKitClient } from "../client/context";
import { FLEET_POLL_MS, fetchFleet, fetchObserverState, type FleetData } from "../api/fleetApi";

export type FleetSnapshot = {
  status: "connecting" | "polling" | "connected" | "error";
  data: FleetData;
  observerState: string;
  error?: string;
  lastPoll?: Date;
};

export const EMPTY_FLEET: FleetData = { agents: [], pods: [], deployments: [], services: [], podMetrics: [] };

// useFleet polls the observer's fleet view and its own run state.
export function useFleet(pollMs: number = FLEET_POLL_MS): FleetSnapshot {
  const client = useKitClient();
  const [snapshot, setSnapshot] = useState<FleetSnapshot>({ status: "connecting", data: EMPTY_FLEET, observerState: "" });

  useEffect(() => {
    let active = true;
    async function poll() {
      setSnapshot((current) => ({ ...current, status: "polling", error: undefined }));
      try {
        const data = await fetchFleet(client);
        const observerState = await fetchObserverState(client);
        if (active) setSnapshot({ status: "connected", data, observerState, lastPoll: new Date() });
      } catch (error) {
        if (active) {
          setSnapshot((current) => ({
            ...current,
            status: "error",
            error: error instanceof Error ? error.message : String(error),
            lastPoll: new Date(),
          }));
        }
      }
    }
    void poll();
    const timer = setInterval(() => void poll(), pollMs);
    return () => {
      active = false;
      clearInterval(timer);
    };
  }, [client, pollMs]);

  return snapshot;
}

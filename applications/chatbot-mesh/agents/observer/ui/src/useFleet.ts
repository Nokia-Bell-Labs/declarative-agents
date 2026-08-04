import { useEffect, useState } from "react";
import { fetchFleet, fetchObserverState, POLL_MS } from "./api";
import type { FleetData } from "./fleet";

export type FleetSnapshot = {
  status: "connecting" | "polling" | "connected" | "error";
  data: FleetData;
  observerState: string;
  error?: string;
  lastPoll?: Date;
};

const emptyFleet: FleetData = {
  agents: [],
  pods: [],
  deployments: [],
  services: [],
  podMetrics: [],
};

export function useFleet(): FleetSnapshot {
  const [snapshot, setSnapshot] = useState<FleetSnapshot>({
    status: "connecting",
    data: emptyFleet,
    observerState: "",
  });

  useEffect(() => {
    let active = true;

    async function poll() {
      setSnapshot((current) => ({ ...current, status: "polling", error: undefined }));
      try {
        const data = await fetchFleet();
        const observerState = await fetchObserverState();
        if (active) {
          setSnapshot({
            status: "connected",
            data,
            observerState,
            lastPoll: new Date(),
          });
        }
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
    const timer = window.setInterval(() => void poll(), POLL_MS);
    return () => {
      active = false;
      window.clearInterval(timer);
    };
  }, []);

  return snapshot;
}

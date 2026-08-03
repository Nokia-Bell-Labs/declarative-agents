import { routes, type FleetData, type FleetResponse, fleetData } from "./fleet";

export const POLL_MS = 10_000;

async function fetchJSON<T>(path: string): Promise<T> {
  const response = await fetch(path);
  if (!response.ok) throw new Error(`HTTP ${response.status}`);
  return (await response.json()) as T;
}

export async function fetchFleet(): Promise<FleetData> {
  return fleetData(await fetchJSON<FleetResponse>(routes.fleet));
}

export async function fetchObserverState(): Promise<string> {
  try {
    const body = await fetchJSON<{ run?: { state?: string } }>(routes.state);
    return body.run?.state ?? "";
  } catch {
    // The observer state is status-bar enrichment and never fails a fleet poll.
    return "";
  }
}

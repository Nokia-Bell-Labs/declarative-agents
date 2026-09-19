import { proxyPath, type KitClient } from "../client/client";
import { MONITOR_EVENTS_STREAM } from "./monitorApi";

// Highlights from a turn's own telemetry. While a turn composes, the serving
// agent's monitor streams its request machine's transitions through the proxy;
// this module turns that stream into the few lines a reader wants while
// waiting. The milestone map names words, not states, and belongs to the
// application: everything unmapped is silence, which keeps the feed readable.

export interface RunEvent {
  command_name?: string;
  from_state?: string;
  to_state?: string;
  signal?: string;
  timestamp?: string;
}

export interface Highlight {
  label: string;
  at?: string;
}

// Milestones maps a declared word to the line shown while it runs.
export type Milestones = Record<string, string>;

export function highlightFor(event: RunEvent, milestones: Milestones): Highlight | undefined {
  const label = milestones[event.command_name ?? ""];
  if (!label) return undefined;
  return { label, at: event.timestamp };
}

// appendHighlight adds a line unless it repeats the previous one, so a
// for_each fan-out does not write the same sentence per unit.
export function appendHighlight(feed: Highlight[], next: Highlight): Highlight[] {
  if (feed.length > 0 && feed[feed.length - 1].label === next.label) return feed;
  return [...feed, next];
}

// openActivityStream subscribes to one agent's monitor stream and calls back
// with each highlight. It returns a stop function; a stream that cannot
// connect never calls back, so the caller keeps its plain pending state.
export function openActivityStream(
  client: KitClient,
  agent: string,
  milestones: Milestones,
  onHighlight: (highlight: Highlight) => void,
): () => void {
  let source: EventSource | undefined;
  try {
    source = client.openEventStream(proxyPath(agent, MONITOR_EVENTS_STREAM));
  } catch {
    return () => undefined;
  }
  const handle = (event: MessageEvent) => {
    try {
      const highlight = highlightFor(JSON.parse(String(event.data)) as RunEvent, milestones);
      if (highlight) onHighlight(highlight);
    } catch {
      // A malformed frame is one lost highlight, never a broken feed.
    }
  };
  source.addEventListener("run_event", handle as EventListener);
  return () => source?.close();
}

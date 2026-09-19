import { useEffect, useState } from "react";
import { useKitClient } from "../client/context";
import { appendHighlight, openActivityStream, type Highlight, type Milestones } from "../api/turnActivity";

// useTurnActivity collects milestone highlights from an agent's monitor stream
// while active is true, and clears them when a new activity starts.
export function useTurnActivity(agent: string, milestones: Milestones, active: boolean): Highlight[] {
  const client = useKitClient();
  const [feed, setFeed] = useState<Highlight[]>([]);
  useEffect(() => {
    if (!active) return;
    setFeed([]);
    return openActivityStream(client, agent, milestones, (highlight) => setFeed((prev) => appendHighlight(prev, highlight)));
  }, [client, agent, milestones, active]);
  return feed;
}

import { useEffect, useState } from "react";
import { fetchAgentState, pollDelay } from "../api/monitorApi";
import { useKitClient } from "../client/context";

// unknown covers both "not probed yet" and a probe that failed for a reason
// other than a proxy 404; neither hides a panel.
export type AgentPresence = "deployed" | "absent" | "unknown";

// useAgentPresence probes each agent's monitor state once through the monitor
// proxy. A proxy 404 marks the agent absent (srd004 R2.2); an absent agent is
// probed again on the monitor's absent cadence so a later deployment shows up.
export function useAgentPresence(agents: string[]): Record<string, AgentPresence> {
  const client = useKitClient();
  const key = [...new Set(agents)].sort().join("\n");
  const [presence, setPresence] = useState<Record<string, AgentPresence>>({});

  useEffect(() => {
    let active = true;
    const timers = new Set<ReturnType<typeof setTimeout>>();

    const probe = async (agent: string) => {
      let state: AgentPresence;
      try {
        state = (await fetchAgentState(client, agent)).deployed ? "deployed" : "absent";
      } catch {
        state = "unknown";
      }
      if (!active) return;
      setPresence((prev) => (prev[agent] === state ? prev : { ...prev, [agent]: state }));
      if (state === "absent") {
        const timer = setTimeout(() => {
          timers.delete(timer);
          void probe(agent);
        }, pollDelay("absent"));
        timers.add(timer);
      }
    };
    for (const agent of key === "" ? [] : key.split("\n")) void probe(agent);

    return () => {
      active = false;
      timers.forEach(clearTimeout);
    };
  }, [client, key]);

  return presence;
}

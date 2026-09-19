import { useEffect, useState } from "react";
import { fetchDeclaredMachines, fetchDeclaredTools, type DeclaredMachine, type DeclaredTool } from "../../api/monitorApi";
import { useKitClient } from "../../client/context";

export interface DeclaredState {
  reading: boolean;
  machines: DeclaredMachine[];
  tools: Record<string, DeclaredTool>;
}

// useDeclared reads one agent's declared machines and words through the kit
// client. They are read live rather than baked into the bundle because the
// figure must draw the program the agent is running, which a live profile
// change can alter under a running mesh. An agent the proxy reports not
// deployed yields no machines.
export function useDeclared(agent: string | undefined): DeclaredState {
  const client = useKitClient();
  const [state, setState] = useState<DeclaredState>({ reading: agent !== undefined, machines: [], tools: {} });
  useEffect(() => {
    if (!agent) {
      setState({ reading: false, machines: [], tools: {} });
      return;
    }
    let active = true;
    setState({ reading: true, machines: [], tools: {} });
    void Promise.all([fetchDeclaredMachines(client, agent), fetchDeclaredTools(client, agent)]).then(([machines, tools]) => {
      if (active) setState({ reading: false, machines, tools });
    });
    return () => {
      active = false;
    };
  }, [client, agent]);
  return state;
}

import { useEffect, useState } from "react";
import {
  AgentCards,
  MachineViewMount,
  StatusBar,
  Topology,
  TOPOLOGY_TRACE_BACKEND,
  staticHref,
  useFleet,
  useKitClient,
  useTopology,
  type MonitoredAgent,
} from "@declarative-agents/ui-kit";

// The observer the REST tool serves from the embedded bundle (applications
// srd004 R9). It is composition only: the declaration's config, served as
// ui-config.json beside the bundle, names the title, monitored agents, trace
// backend, and role annotation; the kit panels do the rest.

export interface ObserverConfig {
  title?: string;
  monitored_agents?: MonitoredAgent[];
  trace_backend?: string;
  role_annotation?: string;
  agent_order?: string[];
}

const NO_ROUTES = { routes: [], defaultPanel: "" };

function useObserverConfig(): ObserverConfig | undefined {
  const client = useKitClient();
  const [config, setConfig] = useState<ObserverConfig>();
  useEffect(() => {
    const path = staticHref(window.location.pathname, NO_ROUTES, "ui-config.json");
    client.getJSON<ObserverConfig>(path).then(setConfig, () => setConfig({}));
  }, [client]);
  return config;
}

export function Observer() {
  const config = useObserverConfig();
  const snapshot = useFleet();
  const read = useTopology(snapshot.data, {
    backend: config?.trace_backend ?? TOPOLOGY_TRACE_BACKEND,
    roleAnnotation: config?.role_annotation,
  });
  const title = config?.title ?? "Observer";
  useEffect(() => {
    document.title = title;
  }, [title]);
  const derived = read.status === "ready" ? read.state : undefined;

  return (
    <div className="observer">
      <h1>{title}</h1>
      <StatusBar snapshot={snapshot} />
      <Topology data={snapshot.data} read={read} roleAnnotation={config?.role_annotation} />
      <AgentCards
        data={snapshot.data}
        order={config?.agent_order}
        egress={derived?.egress}
        walks={derived?.walks}
        error={snapshot.status === "error" ? snapshot.error : undefined}
      />
      {(config?.monitored_agents?.length ?? 0) > 0 && (
        <MachineViewMount monitoredAgents={config?.monitored_agents ?? []} traceBackend={config?.trace_backend} />
      )}
    </div>
  );
}

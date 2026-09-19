import { useMemo, useState, type ComponentType } from "react";
import { createKitClient, type KitClient } from "../client/client";
import { KitClientProvider } from "../client/context";
import type { KitPanel, PanelProps } from "../panels/manifest";
import { kitPanelRegistry } from "../panels/registry";
import { PanelFrame } from "./PanelFrame";
import { Sidebar } from "./Sidebar";
import { ShellRoutingProvider } from "./subPath";
import { routingFromConfig, shellTitle, KIT_PACKAGE, type UIConfig, type UIPanel } from "./uiConfig";
import { useAgentPresence } from "./useAgentPresence";
import { usePanelLocation } from "./usePanelLocation";

// A registry entry is a mountable component or a whole kit panel, so an
// application may write {...kitPanelRegistry, ...domainPanels}.
export type PanelRegistry = Record<string, ComponentType<PanelProps> | KitPanel>;

export interface AppShellProps {
  config: UIConfig;
  registry?: PanelRegistry;
  // Defaults to a same-origin client (srd004 R6.1).
  client?: KitClient;
}

// AppShell is the generic application shell (srd004 R5): sidebar and routes
// come from ui.yaml alone, panels mount from the application registry or, for
// kit panels, from the kit registry by export, and the URL scheme is the
// shared splitPanelPath one (R5.3).
export function AppShell({ config, registry = {}, client }: AppShellProps) {
  const [fallback] = useState(() => createKitClient());
  return (
    <KitClientProvider client={client ?? fallback}>
      <ShellBody config={config} registry={registry} />
    </KitClientProvider>
  );
}

function ShellBody({ config, registry }: { config: UIConfig; registry: PanelRegistry }) {
  const routing = useMemo(() => routingFromConfig(config), [config]);
  const location = usePanelLocation(routing);
  const panels = useMemo(() => new Map((config.panels ?? []).map((panel) => [panel.id, panel])), [config]);

  // srd004 R2.2: a kit panel whose monitored agents are all not deployed has
  // no sidebar entry. It stays reachable by URL.
  const watched = useMemo(() => [...panels.values()].map((panel) => [panel.id, monitoredBy(panel, config)] as const), [panels, config]);
  const presence = useAgentPresence(watched.flatMap(([, agents]) => agents));
  const absentPanels = new Set(watched.filter(([, agents]) => agents.length > 0 && agents.every((agent) => presence[agent] === "absent")).map(([id]) => id));
  const sidebarRoutes = routing.routes.map((route) => (absentPanels.has(route.id) ? { ...route, hidden: true } : route));

  const active = location.active;
  const Mount = mountFor(active, panels.get(active), registry);
  return (
    <ShellRoutingProvider routing={routing}>
      <PanelFrame
        sidebar={
          <Sidebar
            title={shellTitle(config)}
            routes={sidebarRoutes}
            groups={routing.groups}
            active={active}
            href={location.href}
            onNavigate={location.navigate}
          />
        }
      >
        {Mount ? (
          <Mount
            key={active}
            config={panels.get(active)?.config}
            monitoredAgents={config.monitored_agents ?? []}
            traceBackend={config.trace_backend?.name}
          />
        ) : (
          <div className="dak-shell-placeholder" role="status" data-testid="panel-placeholder">
            No component is registered for panel {JSON.stringify(active)}.
          </div>
        )}
      </PanelFrame>
    </ShellRoutingProvider>
  );
}

const isKitPanel = (entry: ComponentType<PanelProps> | KitPanel): entry is KitPanel =>
  typeof entry === "object" && entry !== null && "manifest" in entry && "component" in entry;

// mountFor resolves the application registry first, then a kit panel by its
// export. A version 1 route mounts only from the registry.
function mountFor(id: string, panel: UIPanel | undefined, registry: PanelRegistry): ComponentType<PanelProps> | undefined {
  const entry = Object.prototype.hasOwnProperty.call(registry, id) ? registry[id] : undefined;
  if (entry) return isKitPanel(entry) ? entry.component : entry;
  if (panel?.package === KIT_PACKAGE && panel.export) return kitPanelRegistry[panel.export]?.component;
  return undefined;
}

// monitoredBy names the agents a kit panel's manifest monitors: the ui.yaml
// monitored_agents for "declared", otherwise the manifest's own list.
function monitoredBy(panel: UIPanel, config: UIConfig): string[] {
  if (panel.package !== KIT_PACKAGE || !panel.export) return [];
  const manifest = kitPanelRegistry[panel.export]?.manifest;
  if (!manifest) return [];
  return manifest.monitored_agents === "declared" ? (config.monitored_agents ?? []).map((agent) => agent.name) : manifest.monitored_agents;
}

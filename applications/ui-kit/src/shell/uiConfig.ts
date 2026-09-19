import { kitPanelManifestById } from "../panels/manifests";
import type { PanelRoute, PanelRouting } from "./paths";
import type { SidebarGroup } from "./Sidebar";

// A parsed ui.yaml, the composition input of the kit shell (srd004 R7). The
// types mirror the Go Document in magefiles/uiyaml; unknown top-level keys are
// kept so application extensions stay valid.

// KIT_PACKAGE is the panels package whose exports the shell mounts without an
// application registry entry (srd004 R7.1).
export const KIT_PACKAGE = "@declarative-agents/ui-kit";

export interface UIRoute {
  id: string;
  path: string;
  label?: string;
  action?: string;
  resource?: string;
}

export interface UISidebarGroup {
  label: string;
  order?: number;
}

export interface UISidebar {
  title?: string;
  groups?: Record<string, UISidebarGroup>;
}

export interface UIPanel {
  id: string;
  package: string;
  export?: string;
  route: string;
  label?: string;
  sidebar_group?: string;
  hidden?: boolean;
  config?: Record<string, unknown>;
}

export interface UIMonitoredAgent {
  name: string;
  label: string;
}

export interface UITraceBackend {
  name: string;
  query_path?: string;
}

export interface UIBranding {
  title?: string;
  logo?: string;
  accent?: string;
}

export interface UIConfig {
  version?: number;
  id: string;
  title?: string;
  source_owner?: string;
  routes?: UIRoute[];
  sidebar?: UISidebar;
  actions?: Record<string, unknown>;
  panels?: UIPanel[];
  monitored_agents?: UIMonitoredAgent[];
  trace_backend?: UITraceBackend;
  branding?: UIBranding;
  presentation?: Record<string, unknown>;
  deployment_api?: Record<string, unknown>;
  [extension: string]: unknown;
}

// TRACE_QUERY_SUFFIX ends every trace_backend.query_path; the prefix before it
// is the same-origin trace backend (srd004 R2.4, R7.1).
export const TRACE_QUERY_SUFFIX = "/query/traces/{trace_id}";

// One lower-case segment: the shell's URL scheme treats the last segment as
// the panel slot (srd004 R5.3).
const ROUTE_PATTERN = /^\/[a-z0-9][a-z0-9-]*$/;

const PANEL_KEYS = new Set(["id", "package", "export", "route", "label", "sidebar_group", "hidden", "config"]);
const BRANDING_KEYS = new Set(["title", "logo", "accent"]);

type Mapping = Record<string, unknown>;
const isMapping = (value: unknown): value is Mapping => typeof value === "object" && value !== null && !Array.isArray(value);
const absent = (value: unknown) => value === undefined || value === null;
const text = (value: unknown) => (typeof value === "string" ? value : "");
const entries = (value: unknown): Mapping[] => (Array.isArray(value) ? value.filter(isMapping) : []);
const q = (value: string) => JSON.stringify(value);
const has = (map: object, key: string) => Object.prototype.hasOwnProperty.call(map, key);

type Kind = "string" | "integer" | "boolean" | "mapping" | "list";
const KIND_CHECKS: Record<Kind, (value: unknown) => boolean> = {
  string: (value) => typeof value === "string",
  integer: (value) => Number.isInteger(value),
  boolean: (value) => typeof value === "boolean",
  mapping: isMapping,
  list: Array.isArray,
};

// structuralProblems is the ui.v2.schema.json layer: field types, required
// keys the semantic rules do not already report, and the closed key sets of
// panels and branding.
function structuralProblems(doc: Mapping): string[] {
  const problems: string[] = [];
  const expect = (where: string, value: unknown, kind: Kind) => {
    if (!absent(value) && !KIND_CHECKS[kind](value)) problems.push(`${where} must be a ${kind}`);
  };
  const fields = (where: string, value: Mapping, kinds: Record<string, Kind>) => {
    for (const [key, kind] of Object.entries(kinds)) expect(`${where}.${key}`, value[key], kind);
  };

  fields("ui.yaml", doc, {
    id: "string",
    title: "string",
    source_owner: "string",
    routes: "list",
    sidebar: "mapping",
    actions: "mapping",
    panels: "list",
    monitored_agents: "list",
    trace_backend: "mapping",
    branding: "mapping",
    presentation: "mapping",
    deployment_api: "mapping",
  });
  const listed = (key: string) => (Array.isArray(doc[key]) ? (doc[key] as unknown[]) : []);
  listed("routes").forEach((route, i) => {
    if (!isMapping(route)) return problems.push(`routes[${i}] must be a mapping`);
    fields(`routes[${i}]`, route, { id: "string", path: "string", label: "string", action: "string", resource: "string" });
  });
  listed("panels").forEach((panel, i) => {
    if (!isMapping(panel)) return problems.push(`panels[${i}] must be a mapping`);
    fields(`panels[${i}]`, panel, {
      id: "string",
      package: "string",
      export: "string",
      route: "string",
      label: "string",
      sidebar_group: "string",
      hidden: "boolean",
      config: "mapping",
    });
    for (const key of Object.keys(panel)) if (!PANEL_KEYS.has(key)) problems.push(`panels[${i}] has unknown key ${q(key)}`);
  });
  listed("monitored_agents").forEach((agent, i) => {
    if (!isMapping(agent)) return problems.push(`monitored_agents[${i}] must be a mapping`);
    fields(`monitored_agents[${i}]`, agent, { name: "string", label: "string" });
    if (absent(agent.label)) problems.push(`monitored_agents[${i}] has no label`);
  });
  if (isMapping(doc.sidebar)) {
    fields("sidebar", doc.sidebar, { title: "string", groups: "mapping" });
    if (isMapping(doc.sidebar.groups)) {
      for (const [id, group] of Object.entries(doc.sidebar.groups)) {
        if (!isMapping(group)) {
          problems.push(`sidebar.groups.${id} must be a mapping`);
        } else {
          fields(`sidebar.groups.${id}`, group, { label: "string", order: "integer" });
          if (absent(group.label)) problems.push(`sidebar.groups.${id} has no label`);
        }
      }
    }
  }
  if (isMapping(doc.trace_backend)) fields("trace_backend", doc.trace_backend, { name: "string", query_path: "string" });
  if (isMapping(doc.branding)) {
    fields("branding", doc.branding, { title: "string", logo: "string", accent: "string" });
    for (const key of Object.keys(doc.branding)) if (!BRANDING_KEYS.has(key)) problems.push(`branding has unknown key ${q(key)}`);
  }
  return problems;
}

// validateUIConfig returns every violation of a parsed ui.yaml, sorted, and an
// empty list for a valid file. The semantic rules and their messages mirror
// Document.Validate in magefiles/uiyaml (srd004 R7.1, R7.2).
export function validateUIConfig(doc: unknown): string[] {
  if (!isMapping(doc)) return ["ui.yaml must be a mapping"];
  const problems = structuralProblems(doc);
  const add = (problem: string) => problems.push(problem);

  const panels = entries(doc.panels);
  const version = doc.version;
  if (absent(version) || version === 1) {
    if (panels.length > 0) add("panels require version: 2");
  } else if (version !== 2) {
    add(`version ${typeof version === "number" ? version : q(String(version))} is not supported (1 or 2)`);
  }
  if (text(doc.id).trim() === "") add("id is required");

  const ids = new Map<string, string>();
  const paths = new Map<string, string>();
  const claim = (kind: string, id: string, path: string) => {
    if (id === "") add(`${kind} has no id`);
    else if (ids.has(id)) add(`duplicate id ${q(id)} (${ids.get(id)} and ${kind})`);
    else ids.set(id, kind);
    if (!ROUTE_PATTERN.test(path)) add(`${kind} ${q(id)} path ${q(path)} must be one lower-case segment such as /traces`);
    else if (paths.has(path)) add(`route ${path} collides: ${paths.get(path)} and ${kind} ${q(id)}`);
    else paths.set(path, `${kind} ${q(id)}`);
  };
  for (const route of entries(doc.routes)) claim("route", text(route.id), text(route.path));

  const groups = isMapping(doc.sidebar) && isMapping(doc.sidebar.groups) ? doc.sidebar.groups : {};
  for (const panel of panels) {
    const id = text(panel.id);
    const pkg = text(panel.package);
    const group = text(panel.sidebar_group);
    claim("panel", id, text(panel.route));
    if (pkg.trim() === "") add(`panel ${q(id)} has no package`);
    if (pkg === KIT_PACKAGE && text(panel.export).trim() === "") add(`panel ${q(id)} from ${KIT_PACKAGE} must name the kit panel in export`);
    if (group !== "" && !has(groups, group)) add(`panel ${q(id)} sidebar_group ${q(group)} is not declared under sidebar.groups`);
  }

  const agents = new Set<string>();
  for (const agent of entries(doc.monitored_agents)) {
    const name = text(agent.name);
    if (name === "") add("monitored_agents entry has no name");
    else if (agents.has(name)) add(`monitored agent ${q(name)} is listed twice`);
    agents.add(name);
  }
  if (isMapping(doc.trace_backend)) {
    if (text(doc.trace_backend.name).trim() === "") add("trace_backend has no name");
    const queryPath = text(doc.trace_backend.query_path);
    if (queryPath !== "" && !(queryPath.startsWith("/") && queryPath.endsWith(TRACE_QUERY_SUFFIX))) {
      add(`trace_backend query_path ${q(queryPath)} must be an absolute path ending in ${TRACE_QUERY_SUFFIX}`);
    }
  }

  return problems.sort();
}

// ShellRouting is the panel routing plus the ordered sidebar groups, both
// derived from ui.yaml alone (srd004 R5.2).
export interface ShellRouting extends PanelRouting {
  groups: SidebarGroup[];
}

// routingFromConfig lists routes[] then panels[] as shell routes. A version 1
// route renders under the sidebar group keyed by its own id when one is
// declared, as the chatbot-mesh shell groups today; a panel renders under its
// sidebar_group. A kit panel without a label takes its manifest title. The
// default panel is the first non-hidden entry in sidebar order.
export function routingFromConfig(config: UIConfig): ShellRouting {
  const declared = config.sidebar?.groups ?? {};
  const groups: SidebarGroup[] = Object.entries(declared).map(([id, group]) => ({ id, label: group.label, order: group.order }));

  const routes: PanelRoute[] = [
    ...(config.routes ?? []).map((route) => ({
      id: route.id,
      path: route.path,
      label: route.label ?? route.id,
      group: has(declared, route.id) ? route.id : undefined,
    })),
    ...(config.panels ?? []).map((panel) => ({
      id: panel.id,
      path: panel.route,
      label: panel.label ?? kitManifestTitle(panel) ?? panel.id,
      hidden: panel.hidden || undefined,
      group: panel.sidebar_group,
    })),
  ];

  return { routes, groups, defaultPanel: sidebarOrder(routes, groups).find((route) => !route.hidden)?.id ?? routes[0]?.id ?? "" };
}

// traceBackendFromConfig is the trace backend the shell passes panels as
// PanelProps.traceBackend (srd004 R2.4): the same-origin prefix of
// trace_backend.query_path ("/" when the prefix is empty), otherwise
// trace_backend.name.
export function traceBackendFromConfig(config: UIConfig): string | undefined {
  const backend = config.trace_backend;
  if (!backend) return undefined;
  const queryPath = backend.query_path ?? "";
  if (queryPath.endsWith(TRACE_QUERY_SUFFIX)) return queryPath.slice(0, -TRACE_QUERY_SUFFIX.length) || "/";
  return backend.name || undefined;
}

// shellTitle is the sidebar title: branding first, then the sidebar's own
// title, then the UI title.
export function shellTitle(config: UIConfig): string {
  return config.branding?.title ?? config.sidebar?.title ?? config.title ?? config.id;
}

function kitManifestTitle(panel: UIPanel): string | undefined {
  return panel.package === KIT_PACKAGE && panel.export ? kitPanelManifestById[panel.export]?.title : undefined;
}

// sidebarOrder is the order Sidebar renders entries: groups by order, members
// in declaration order, then entries without a known group.
function sidebarOrder(routes: PanelRoute[], groups: SidebarGroup[]): PanelRoute[] {
  const ordered = [...groups].sort((a, b) => (a.order ?? 0) - (b.order ?? 0));
  const known = new Set(ordered.map((group) => group.id));
  return [
    ...ordered.flatMap((group) => routes.filter((route) => route.group === group.id)),
    ...routes.filter((route) => !route.group || !known.has(route.group)),
  ];
}

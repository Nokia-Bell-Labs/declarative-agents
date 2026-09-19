// Panel URL scheme shared by every shell (srd004 R5.3). The app is served under
// a mount such as /ui/ by static_assets and at / by the Vite dev server, so the
// base is derived from the URL: the last segment is the panel slot and
// everything before it is the base. Extracted unchanged from the chatbot copies
// in chatbot-mesh, cohere-demo, and agentic-wiki-mesh, with the route table
// passed in instead of compiled in.

export interface PanelRoute {
  id: string;
  path: string;
  label: string;
  // A hidden route is reachable by URL but has no sidebar entry.
  hidden?: boolean;
  // The sidebar group the entry renders under, when the shell groups entries.
  group?: string;
}

export interface PanelRouting {
  routes: PanelRoute[];
  defaultPanel: string;
}

export function splitPanelPath(pathname: string, routing: PanelRouting): { base: string; panel: string } {
  const segments = pathname.split("/");
  const last = segments[segments.length - 1];
  if (last === "") {
    // A trailing slash means no panel segment: the whole path is the base.
    return { base: pathname, panel: routing.defaultPanel };
  }
  const base = segments.slice(0, -1).join("/") + "/";
  const match = routing.routes.find((route) => route.path === `/${last}`);
  // An unknown segment falls back to the default and is still the panel slot,
  // so navigating from it replaces the segment instead of nesting under it.
  return { base, panel: match ? match.id : routing.defaultPanel };
}

export function panelForPath(pathname: string, routing: PanelRouting): string {
  return splitPanelPath(pathname, routing).panel;
}

// panelHref builds a panel's URL from the current location, preserving the
// base the app is mounted at.
export function panelHref(pathname: string, routing: PanelRouting, panel: string): string {
  const { base } = splitPanelPath(pathname, routing);
  const route = routing.routes.find((entry) => entry.id === panel);
  return base + (route ? route.path.slice(1) : "");
}

// staticHref resolves a file served beside the app, under the same base.
export function staticHref(pathname: string, routing: PanelRouting, file: string): string {
  return splitPanelPath(pathname, routing).base + file;
}

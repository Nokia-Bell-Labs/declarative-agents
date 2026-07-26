// Panel routing for the three-panel shell. The paths here are the ones
// ux/ux.yaml declares (srd002 R5, srd003); a Go test cross-checks the two so a
// route added to the config and not to the app fails the build rather than
// silently rendering the chat panel (GH-723).

export type PanelId = "chat" | "observability" | "provisioning";

export interface PanelRoute {
  id: PanelId;
  path: string;
  label: string;
}

export const PANEL_ROUTES: PanelRoute[] = [
  { id: "chat", path: "/chat", label: "Chat" },
  { id: "observability", path: "/observability", label: "Observability" },
  { id: "provisioning", path: "/provisioning", label: "Provisioning" },
];

export const DEFAULT_PANEL: PanelId = "chat";

// splitPanelPath separates the mount point from the panel segment. The app is
// served under /ui/ by the chatbot's static_assets and at / by the vite dev
// server, so the base is derived from the URL rather than hard-coded: the last
// segment is the panel slot, and everything before it is the base.
export function splitPanelPath(pathname: string): { base: string; panel: PanelId } {
  const segments = pathname.split("/");
  const last = segments[segments.length - 1];
  if (last === "") {
    // A trailing slash means no panel segment: the whole path is the base.
    return { base: pathname, panel: DEFAULT_PANEL };
  }
  const base = segments.slice(0, -1).join("/") + "/";
  const match = PANEL_ROUTES.find((route) => route.path === `/${last}`);
  // An unknown segment falls back to chat and is still treated as the panel
  // slot, so navigating from it replaces the segment instead of nesting under
  // it (GH-723 R4).
  return { base, panel: match ? match.id : DEFAULT_PANEL };
}

export function panelForPath(pathname: string): PanelId {
  return splitPanelPath(pathname).panel;
}

// panelHref builds the URL for a panel from the current location, preserving
// whatever base path the app is mounted at.
export function panelHref(pathname: string, panel: PanelId): string {
  const { base } = splitPanelPath(pathname);
  const route = PANEL_ROUTES.find((entry) => entry.id === panel);
  return base + (route ? route.path.slice(1) : "");
}

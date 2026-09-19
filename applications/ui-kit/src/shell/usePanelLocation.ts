import { useEffect, useState, type MouseEvent } from "react";
import { panelForPath, panelHref, type PanelRouting } from "./paths";

export interface PanelLocation {
  path: string;
  active: string;
  href(panel: string): string;
  navigate(event: MouseEvent<HTMLAnchorElement>, panel: string): void;
}

// usePanelLocation derives the active panel from the URL rather than component
// state, so every declared panel is linkable and survives a reload.
export function usePanelLocation(routing: PanelRouting): PanelLocation {
  const [path, setPath] = useState(() => window.location.pathname);

  // Back and forward move between panels because navigation pushes history
  // entries; without this listener the URL would change and the panel stay.
  useEffect(() => {
    const onPopState = () => setPath(window.location.pathname);
    window.addEventListener("popstate", onPopState);
    return () => window.removeEventListener("popstate", onPopState);
  }, []);

  return {
    path,
    active: panelForPath(path, routing),
    href: (panel) => panelHref(path, routing, panel),
    navigate(event, panel) {
      // Plain left-clicks route in-app. Modified clicks keep the browser's own
      // behaviour, which is why entries carry a real href.
      if (event.defaultPrevented || event.button !== 0) return;
      if (event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return;
      event.preventDefault();
      window.history.pushState(null, "", panelHref(window.location.pathname, routing, panel));
      setPath(window.location.pathname);
    },
  };
}

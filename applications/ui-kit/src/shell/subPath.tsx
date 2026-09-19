import { createContext, useContext, useEffect, useState, type MouseEvent, type ReactNode } from "react";
import type { PanelRouting } from "./paths";

// Sub-paths below a panel (srd004 R5.3). The shell selects a panel by the last
// URL segment, which cannot express a drill-down such as /traces/{trace_id} or
// /sessions/{suite}/{ts}. A panel that owns such paths reads the full location
// through usePanelPath and moves with navigateTo, which fires popstate so the
// shell re-derives its active panel. Extracted from the catalog bench and
// collector UIs, which carried identical copies.

const ShellRoutingContext = createContext<PanelRouting>({ routes: [], defaultPanel: "" });

export function ShellRoutingProvider({ routing, children }: { routing: PanelRouting; children: ReactNode }) {
  return <ShellRoutingContext.Provider value={routing}>{children}</ShellRoutingContext.Provider>;
}

export function navigateTo(path: string, replace = false) {
  if (replace) window.history.replaceState(null, "", path);
  else window.history.pushState(null, "", path);
  window.dispatchEvent(new PopStateEvent("popstate"));
}

// canonicalPanelPath folds a sidebar link taken from a sub-path back to the
// panel: from /traces/{id} the sidebar links /traces/explore, and that path
// names the explore panel itself.
export function canonicalPanelPath(pathname: string, routing: PanelRouting): string {
  const last = pathname.slice(pathname.lastIndexOf("/"));
  return last !== pathname && routing.routes.some((route) => route.path === last) ? last : pathname;
}

// usePanelPath is the current location, folded to its canonical form and
// re-read on every history move.
export function usePanelPath(): string {
  const routing = useContext(ShellRoutingContext);
  const [, rerender] = useState(0);
  useEffect(() => {
    const onPop = () => rerender((n) => n + 1);
    window.addEventListener("popstate", onPop);
    return () => window.removeEventListener("popstate", onPop);
  }, []);
  const path = window.location.pathname;
  const clean = canonicalPanelPath(path, routing);
  useEffect(() => {
    if (clean !== path) navigateTo(clean, true);
  }, [clean, path]);
  return clean;
}

// PanelLink routes plain left-clicks in-app and leaves modified clicks to the
// browser, which is why it renders a real href.
export function PanelLink({ to, className, children }: { to: string; className?: string; children: ReactNode }) {
  const onClick = (event: MouseEvent<HTMLAnchorElement>) => {
    if (event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return;
    event.preventDefault();
    navigateTo(to);
  };
  return (
    <a href={to} className={className} onClick={onClick}>
      {children}
    </a>
  );
}

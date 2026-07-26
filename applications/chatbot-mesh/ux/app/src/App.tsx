import { useEffect, useState } from "react";
import ChatPanel from "./ChatPanel";
import ObservabilityPanel from "./ObservabilityPanel";
import ProvisioningPanel from "./ProvisioningPanel";
import { TurnProvider } from "./turns";
import { PANEL_ROUTES, panelForPath, panelHref, type PanelId } from "./routes";

// Three-panel shell (srd002 R5): chat, observability, and provisioning are built.
// The active panel is derived from the URL rather than held in component state,
// so every panel ux.yaml declares is linkable and survives a reload (GH-723).

function panelFor(active: PanelId): React.ReactNode {
  switch (active) {
    case "chat":
      return <ChatPanel />;
    case "observability":
      return <ObservabilityPanel />;
    default:
      return <ProvisioningPanel />;
  }
}

export default function App() {
  const [path, setPath] = useState(() => window.location.pathname);
  const active = panelForPath(path);

  // Back and forward move between panels because the nav pushes history
  // entries. Without this listener they would change the URL and leave the
  // rendered panel behind.
  useEffect(() => {
    const onPopState = () => setPath(window.location.pathname);
    window.addEventListener("popstate", onPopState);
    return () => window.removeEventListener("popstate", onPopState);
  }, []);

  function navigate(event: React.MouseEvent<HTMLAnchorElement>, panel: PanelId) {
    // Plain left-clicks route in-app. Modified clicks keep the browser's own
    // behaviour, which is why the nav items carry a real href: open-in-new-tab
    // and copy-link-address have to keep working.
    if (event.defaultPrevented || event.button !== 0) return;
    if (event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return;
    event.preventDefault();
    window.history.pushState(null, "", panelHref(window.location.pathname, panel));
    setPath(window.location.pathname);
  }

  return (
    <TurnProvider>
      <div className="shell">
        <nav className="sidebar">
          <div className="sidebar-title">Chatbot</div>
          {PANEL_ROUTES.map((item) => (
            <a
              key={item.id}
              href={panelHref(path, item.id)}
              className={`nav-item${active === item.id ? " nav-item-active" : ""}`}
              onClick={(event) => navigate(event, item.id)}
            >
              <span>{item.label}</span>
            </a>
          ))}
        </nav>
        <main className="content">{panelFor(active)}</main>
      </div>
    </TurnProvider>
  );
}

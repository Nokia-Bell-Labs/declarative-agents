import { useState } from "react";
import ChatPanel from "./ChatPanel";
import ObservabilityPanel from "./ObservabilityPanel";
import ProvisioningPanel from "./ProvisioningPanel";
import { TurnProvider } from "./turns";

type PanelId = "chat" | "observability" | "provisioning";

interface NavItem {
  id: PanelId;
  label: string;
  planned?: boolean;
}

// Three-panel shell (srd014 R5): chat, observability, and provisioning are built.
const NAV: NavItem[] = [
  { id: "chat", label: "Chat" },
  { id: "observability", label: "Observability" },
  { id: "provisioning", label: "Provisioning" },
];

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
  const [active, setActive] = useState<PanelId>("chat");

  return (
    <TurnProvider>
      <div className="shell">
        <nav className="sidebar">
          <div className="sidebar-title">Chatbot</div>
          {NAV.map((item) => (
            <button
              key={item.id}
              className={`nav-item${active === item.id ? " nav-item-active" : ""}`}
              onClick={() => setActive(item.id)}
            >
              <span>{item.label}</span>
              {item.planned && <span className="nav-badge">planned</span>}
            </button>
          ))}
        </nav>
        <main className="content">{panelFor(active)}</main>
      </div>
    </TurnProvider>
  );
}

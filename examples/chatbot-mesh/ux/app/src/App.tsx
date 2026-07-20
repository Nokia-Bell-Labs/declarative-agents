import { useState } from "react";
import ChatPanel from "./ChatPanel";
import ObservabilityPanel from "./ObservabilityPanel";
import { TurnProvider } from "./turns";

type PanelId = "chat" | "observability";

interface NavItem {
  id: PanelId;
  label: string;
  planned?: boolean;
}

// Two-panel shell (srd002 R5): chat and observability. The provisioning panel is
// part of the deferred control-plane addition and is not in this data-plane cut.
const NAV: NavItem[] = [
  { id: "chat", label: "Chat" },
  { id: "observability", label: "Observability" },
];

function panelFor(active: PanelId): React.ReactNode {
  switch (active) {
    case "chat":
      return <ChatPanel />;
    default:
      return <ObservabilityPanel />;
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

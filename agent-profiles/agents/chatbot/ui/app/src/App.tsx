import { useState } from "react";
import ChatPanel from "./ChatPanel";

type PanelId = "chat" | "observability" | "provisioning";

interface NavItem {
  id: PanelId;
  label: string;
  planned?: boolean;
}

// Three-panel shell (srd014 R5). Chat is built; observability and provisioning
// are placeholders that later epics fill in — the shell reserves their slots so
// they drop in without a navigation rework.
const NAV: NavItem[] = [
  { id: "chat", label: "Chat" },
  { id: "observability", label: "Observability", planned: true },
  { id: "provisioning", label: "Provisioning", planned: true },
];

function Placeholder({ label }: { label: string }) {
  return (
    <div className="placeholder">
      <h2>{label}</h2>
      <p>This panel is planned. Its content ships in a later epic.</p>
    </div>
  );
}

export default function App() {
  const [active, setActive] = useState<PanelId>("chat");

  return (
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
      <main className="content">
        {active === "chat" ? <ChatPanel /> : <Placeholder label={NAV.find((n) => n.id === active)!.label} />}
      </main>
    </div>
  );
}

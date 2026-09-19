import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import config from "virtual:ui-config";
import { AppShell } from "@declarative-agents/ui-kit";
import "@declarative-agents/ui-kit/tokens.css";

// Sidebar, routes, and both panels come from ui.yaml; the registry is empty
// because kit panels mount by their export. An application types
// virtual:ui-config with /// <reference types="@declarative-agents/ui-kit/ui-config" />;
// the kit's tsconfig includes that file directly.
createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <AppShell config={config} registry={{}} />
  </StrictMode>,
);

import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { createKitClient, KitClientProvider } from "@declarative-agents/ui-kit";
import "@declarative-agents/ui-kit/tokens.css";
import "@declarative-agents/ui-kit/styles.css";
import "./observer.css";
import { Observer } from "./Observer";

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <KitClientProvider client={createKitClient()}>
      <Observer />
    </KitClientProvider>
  </StrictMode>,
);

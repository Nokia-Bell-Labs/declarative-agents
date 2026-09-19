import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { createKitClient, KitClientProvider } from "@declarative-agents/ui-kit";
import "@declarative-agents/ui-kit/styles.css";
import App from "./App";
import "./App.css";

// The curator's monitor listener serves this bundle, so the kit client reads
// same-origin.
const client = createKitClient();

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <KitClientProvider client={client}>
      <App />
    </KitClientProvider>
  </StrictMode>,
);

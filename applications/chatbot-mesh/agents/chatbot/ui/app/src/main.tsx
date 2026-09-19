import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import App from "./App";
import "@declarative-agents/ui-kit/styles.css";
import "./App.css";

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <App />
  </StrictMode>,
);

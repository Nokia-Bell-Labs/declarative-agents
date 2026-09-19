import uiConfig from "virtual:ui-config";
import { AppShell } from "@declarative-agents/ui-kit";
import { registry } from "./panels";
import { TurnProvider } from "./turns";

// The shell reads its sidebar and routes from ui.yaml, bundled at build time by
// the kit's Vite plugin (applications srd004 R5). The active panel follows the
// URL, so every declared panel is linkable and survives a reload (GH-723).
export default function App() {
  return (
    <TurnProvider>
      <AppShell config={uiConfig} registry={registry} />
    </TurnProvider>
  );
}

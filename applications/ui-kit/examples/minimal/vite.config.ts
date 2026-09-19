import { resolve } from "node:path";
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
// An application imports the plugin from "@declarative-agents/ui-kit/vite";
// the example builds against the kit sources so it needs no built dist.
import uiYaml from "../../src/shell/vite-plugin-ui-yaml";

const kit = resolve(__dirname, "../../src");

export default defineConfig({
  root: __dirname,
  base: "./",
  plugins: [react(), uiYaml({ path: "ui.yaml", registryIds: [] })],
  resolve: {
    alias: [
      { find: /^@declarative-agents\/ui-kit$/, replacement: `${kit}/index.ts` },
      { find: /^@declarative-agents\/ui-kit\/tokens\.css$/, replacement: `${kit}/tokens.css` },
    ],
    dedupe: ["react", "react-dom"],
  },
  build: { outDir: "dist", emptyOutDir: true },
});

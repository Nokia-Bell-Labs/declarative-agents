import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import { uiYaml } from "@declarative-agents/ui-kit/vite";

export default defineConfig({
  base: "./",
  plugins: [react(), uiYaml({ path: "../ui.yaml" })],
  resolve: { dedupe: ["react", "react-dom"] },
  build: {
    outDir: "dist",
    emptyOutDir: true,
    sourcemap: false,
  },
  server: {
    port: 5176,
    proxy: {
      "/api": "http://localhost:18080",
      "/monitor": "http://localhost:18082",
    },
  },
});

import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// The observer bundle is mounted wherever a rest.yaml static_assets binding
// selects it, so asset URLs are relative (base "./").
export default defineConfig({
  base: "./",
  plugins: [react()],
  resolve: { dedupe: ["react", "react-dom"] },
  build: { outDir: "dist", emptyOutDir: true, sourcemap: false },
  test: { environment: "jsdom", include: ["test/**/*.test.tsx"] },
});

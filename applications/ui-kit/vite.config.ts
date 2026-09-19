import { resolve } from "node:path";
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// Library build: one ES module with React left to the consuming application,
// so an application bundle holds exactly one React instance (srd004 R8.1).
export default defineConfig({
  plugins: [react()],
  build: {
    outDir: "dist",
    emptyOutDir: true,
    sourcemap: false,
    lib: {
      entry: {
        "ui-kit": resolve(__dirname, "src/index.ts"),
        fixtures: resolve(__dirname, "src/fixtures/index.ts"),
        // The ui.yaml plugin runs in Node at the consumer's build time.
        vite: resolve(__dirname, "src/shell/vite-plugin-ui-yaml.ts"),
      },
      formats: ["es"],
      cssFileName: "ui-kit",
    },
    rollupOptions: {
      external: ["react", "react-dom", "react/jsx-runtime", "vite", "yaml", /^node:/],
    },
  },
  test: {
    include: ["test/**/*.test.ts", "test/**/*.test.tsx"],
  },
});

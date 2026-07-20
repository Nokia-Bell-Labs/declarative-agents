import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

export default defineConfig({
  base: "./",
  plugins: [react()],
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

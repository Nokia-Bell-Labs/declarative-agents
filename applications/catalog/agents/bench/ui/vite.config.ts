import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import uiYaml from '@declarative-agents/ui-kit/vite'

// The bench serves this bundle at its origin's root (rest.yaml static_assets
// /{path...}), so assets keep absolute paths and resolve from deep links such
// as /sessions/{suite}/{ts}. ui.yaml sits beside this file; registryIds are
// the keys of src/panels.ts.
export default defineConfig({
  plugins: [react(), uiYaml({ path: 'ui.yaml', registryIds: ['experiments', 'launch'] })],
  resolve: { dedupe: ['react', 'react-dom'] },
  build: {
    outDir: 'dist',
  },
  server: {
    port: 5173,
    proxy: {
      '/api': 'http://localhost:8080',
    },
  },
})

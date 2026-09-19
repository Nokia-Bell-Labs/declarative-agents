import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import uiYaml from '@declarative-agents/ui-kit/vite'

// The collector serves this bundle at its query origin's root (rest.yaml
// static_assets /{path...}), so assets keep absolute paths and resolve from
// deep links such as /traces/{trace_id}. ui.yaml sits beside this file;
// registryIds are the keys of src/panels.ts.
export default defineConfig({
  plugins: [react(), uiYaml({ path: 'ui.yaml', registryIds: ['traces', 'explore'] })],
  resolve: { dedupe: ['react', 'react-dom'] },
  build: {
    outDir: 'dist',
  },
  server: {
    port: 5174,
    proxy: {
      '/query': 'http://localhost:18193',
    },
  },
})

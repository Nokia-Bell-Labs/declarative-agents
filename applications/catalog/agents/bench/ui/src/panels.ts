import type { PanelRegistry } from '@declarative-agents/ui-kit'
import Experiments from './pages/Experiments'
import Launcher from './pages/Launcher'

// The application's panels, keyed by the ui.yaml panel id. ui.yaml is the only
// route table; the Vite plugin fails the build when a declared local panel has
// no entry here (srd004 R5.2, R7.4).
export const registry: PanelRegistry = {
  experiments: Experiments,
  launch: Launcher,
}

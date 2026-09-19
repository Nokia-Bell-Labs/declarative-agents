import type { PanelRegistry } from '@declarative-agents/ui-kit'
import Explore from './pages/Explore'
import Traces from './pages/Traces'

// The application's panels, keyed by the ui.yaml panel id. ui.yaml is the only
// route table; the Vite plugin fails the build when a declared local panel has
// no entry here (srd004 R5.2, R7.4).
export const registry: PanelRegistry = {
  traces: Traces,
  explore: Explore,
}

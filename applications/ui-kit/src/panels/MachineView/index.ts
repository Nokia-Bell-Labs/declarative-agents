import { definePanel } from "../manifest";
import { MachineViewMount } from "./MachineViewMount";
import { machineViewManifest } from "./manifest";

export { AgentPanel, type AgentPanelProps } from "./AgentPanel";
export { MachineDetail, ToolWindow, type MachineDetailProps } from "./MachineDetail";
export { MachineView, type MachineEdgeRef, type MachineViewProps } from "./MachineView";
export { machineViewConfig, MachineViewMount, type MachineViewConfig } from "./MachineViewMount";
export { collapseChains, happyPath, type CollapsedMachine, type HappyPath } from "./machineCollapse";
export { layoutMachine, type LaidOutEdge, type LaidOutState, type MachineLayout } from "./machineLayout";
export {
  activeStage,
  fetchTaggedMachines,
  sharedSink,
  tagView,
  toTaggedMachine,
  withoutSink,
  type SinklessView,
  type TaggedMachine,
  type ViewTag,
} from "./machineTags";
export {
  finalState,
  machineForWalk,
  machineTools,
  orderedMachines,
  visitedStates,
  walkOverlay,
  type OrderOptions,
  type WalkOverlay,
} from "./machineViews";
export { dialogView, resolveEnvDefault, type DialogParameter, type DialogPrompt, type DialogView } from "./toolDialog";
export { useDeclared, type DeclaredState } from "./useDeclared";
export { machineViewManifest };

export const machineViewPanel = definePanel(machineViewManifest, MachineViewMount);

import { definePanel } from "../manifest";
import { tracePanelManifest } from "./manifest";
import { TracePanelMount } from "./TracePanel";
import "./tracePanel.css";

export { tracePanelManifest };
export { MachineWalks, TracePanel, TracePanelMount, TraceView, type TracePanelProps, type TraceViewProps } from "./TracePanel";
export { DEFAULT_TRACE_PAGE_SIZE, relativeTime, TraceList, type TraceListProps } from "./TraceList";
export { TraceStoryOverlay, type TraceChatTurns, type TraceStoryOverlayProps } from "./StoryOverlay";
export { SpanTreeToggle, Waterfall } from "./Waterfall";
export { Timeline, type TimelineBody } from "./Timeline";
export { parseMessages, SpanDetail, type SpanMessage } from "./SpanDetail";
export { makeTraceReader, TraceOptionsProvider, useTraceOptions, type TraceReader, type TraceViewOptions } from "./options";
export {
  CATEGORY_COLORS,
  DEFAULT_CATEGORY_RULES,
  declaredWord,
  FAILURE_SIGNAL,
  filterTree,
  groupRootsByService,
  SERVICE_COLORS,
  serviceAgents,
  serviceColor,
  spanAnswer,
  spanCategory,
  spanDescription,
  spanState,
  spanStates,
  spanTree,
  timelineLanes,
  timelineSequence,
  traceRootService,
  type CategoryOptions,
  type CategoryRules,
  type DeclaredSurface,
  type DeclaredWord,
  type SequenceRow,
  type SpanCategory,
  type SpanNode,
  type TimelineLane,
} from "./traceLayout";
export { agentGroups, continuations, stepGroups, windowSpans, type Continuation, type ServiceGroup, type StepGroup } from "./traceGroups";

export const tracePanel = definePanel(tracePanelManifest, TracePanelMount);

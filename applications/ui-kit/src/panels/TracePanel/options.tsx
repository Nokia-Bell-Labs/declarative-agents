import { createContext, useContext, useMemo, type ReactNode } from "react";
import type { TraceModel, TraceSpan } from "../../api/traceApi";
import {
  serviceAgents,
  spanCategory,
  spanDescription,
  spanState,
  traceRootService,
  type CategoryRules,
  type DeclaredSurface,
  type SpanCategory,
} from "./traceLayout";

// TraceViewOptions is what an application may tell the trace views about its
// own program. Every field is optional; without them the views read the trace
// alone. They replace the files each application used to compile in
// (cohere-demo declaredDescriptions.json, agentic-wiki-mesh declaredWords.json,
// the forced-tool name table).
export interface TraceViewOptions {
  // The one-line description a declared word carries, by command name.
  describe?: (command: string) => string | undefined;
  // The generated declared-word surface; when present it classifies spans and
  // labels the machine state each word ran in.
  surface?: DeclaredSurface;
  // The declared tool a forced-call command invokes (select_sources_via_tool ->
  // select_sources). Unlisted commands show their name without _via_tool.
  toolNames?: Record<string, string>;
  // Overrides for the host heuristic used when no surface is supplied.
  categoryRules?: Partial<CategoryRules>;
}

const TraceOptionsContext = createContext<TraceViewOptions>({});

export function TraceOptionsProvider({ options, children }: { options: TraceViewOptions; children: ReactNode }) {
  const { describe, surface, toolNames, categoryRules } = options;
  const value = useMemo(() => ({ describe, surface, toolNames, categoryRules }), [describe, surface, toolNames, categoryRules]);
  return <TraceOptionsContext.Provider value={value}>{children}</TraceOptionsContext.Provider>;
}

export function useTraceOptions(): TraceViewOptions {
  return useContext(TraceOptionsContext);
}

// TraceReader resolves the per-span labels a view shows, once per trace.
export interface TraceReader {
  rootService: string;
  category(span: TraceSpan): SpanCategory;
  description(span: TraceSpan): string | undefined;
  state(span: TraceSpan): string | undefined;
  toolName(command: string): string;
}

export function makeTraceReader(trace: TraceModel, options: TraceViewOptions, services: Set<string> = new Set(trace.services)): TraceReader {
  const { surface } = options;
  const agents = surface ? serviceAgents(trace.spans, surface) : new Map<string, string>();
  const rootService = traceRootService(trace);
  return {
    rootService,
    category: (span) => spanCategory(span, services, rootService, { rules: options.categoryRules, surface, agents }),
    description: (span) => (surface ? spanDescription(span, surface, agents) : undefined) ?? (span.command ? options.describe?.(span.command) : undefined),
    state: (span) => (surface ? spanState(span, surface, agents) : undefined),
    toolName: (command) => options.toolNames?.[command] ?? command.replace(/_via_tool$/, ""),
  };
}

export function useTraceReader(trace: TraceModel, services?: Set<string>): TraceReader {
  const options = useTraceOptions();
  return useMemo(() => makeTraceReader(trace, options, services), [trace, options, services]);
}

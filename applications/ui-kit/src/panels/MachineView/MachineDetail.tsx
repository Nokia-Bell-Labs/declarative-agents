import { useState } from "react";
import type { DeclaredMachine, DeclaredTool } from "../../api/monitorApi";
import type { ServiceWalk } from "../../api/traceApi";
import { collapseChains, happyPath } from "./machineCollapse";
import { finalState, machineTools, visitedStates, walkOverlay } from "./machineViews";
import { MachineView, type MachineEdgeRef } from "./MachineView";
import { dialogView } from "./toolDialog";

// One declared machine as the panels show it: the figure reduced so it can be
// read, the walk or live run overlaid through the reduction, and the machine's
// words as tags that open what they declare. Shared by AgentPanel (a trace's
// walk) and the mountable panel (the live run). Split out of agentic-wiki-mesh's
// AgentPanel.

export function ToolWindow({ name, declaration, onClose }: { name: string; declaration?: DeclaredTool; onClose: () => void }) {
  if (!declaration) {
    return (
      <div className="tool-window" role="dialog" aria-label={`Declaration of ${name}`} data-testid="tool-window">
        <div className="tool-window-head">
          <span className="tool-window-title">{name}</span>
          <button type="button" className="machine-close" onClick={onClose}>
            ✕
          </button>
        </div>
        <div className="machine-notice">This agent's monitor served no declaration for {name}.</div>
      </div>
    );
  }
  const view = dialogView(declaration);
  return (
    <div className="tool-window" role="dialog" aria-label={`Declaration of ${name}`} data-testid="tool-window" data-tool={name}>
      <div className="tool-window-head">
        <span className={`kind-badge kind-${view.kindClass}`} data-testid="tool-kind">
          {view.kind}
        </span>
        <span className="tool-window-title">{name}</span>
        {view.model && (
          <span className="dialog-model" data-testid="tool-model">
            {view.model}
          </span>
        )}
        <button type="button" className="machine-close" onClick={onClose}>
          ✕
        </button>
      </div>
      {view.calls.length > 0 && (
        <div className="dialog-calls" data-testid="tool-calls">
          calls {view.calls.join(", ")}
        </div>
      )}
      {view.description && <div className="dialog-description">{view.description}</div>}
      {view.problem && <div className="dialog-problem">{view.problem}</div>}
      {view.goals.length > 0 && (
        <ul className="dialog-goals">
          {view.goals.map((goal) => (
            <li key={goal}>{goal}</li>
          ))}
        </ul>
      )}
      {view.parameters.length > 0 && (
        <table className="dialog-params">
          <tbody>
            {view.parameters.map((parameter) => (
              <tr key={parameter.name}>
                <td>{parameter.name}</td>
                <td>{parameter.type}</td>
                <td>{parameter.required ? "required" : "optional"}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
      {view.inputs.length > 0 && <div className="dialog-inputs">reads {view.inputs.join(", ")}</div>}
      {view.emits.length > 0 && <div className="dialog-emits">emits {view.emits.join(", ")}</div>}
      {/* A prompt is the longest thing a declaration carries, so it opens on
          demand rather than pushing everything else off the panel. */}
      {view.prompts.map((prompt) => (
        <details className="dialog-prompt" key={prompt.label}>
          <summary>{prompt.label}</summary>
          <pre>{prompt.text}</pre>
        </details>
      ))}
      <details className="dialog-prompt dialog-raw" data-testid="tool-raw">
        <summary>declaration as served</summary>
        <pre>{view.raw}</pre>
      </details>
    </div>
  );
}

export interface MachineDetailProps {
  machine: DeclaredMachine;
  tools: Record<string, DeclaredTool>;
  // A trace's walk through this machine's agent.
  walk?: ServiceWalk;
  // The live run's state and last transition, in declared state names.
  currentState?: string;
  activeEdge?: MachineEdgeRef;
}

export function MachineDetail({ machine, tools, walk, currentState, activeEdge }: MachineDetailProps) {
  const [openTool, setOpenTool] = useState<string | undefined>(undefined);
  const [showAllStates, setShowAllStates] = useState(false);

  // The figure is the declared machine reduced twice so it can be read: the
  // failure edges hidden, and the runs that can only be walked one way folded
  // into their heads. The walk and the live state are mapped through the fold,
  // so a state inside a folded run still marks the box it is drawn in.
  const reduced = showAllStates ? { spec: machine, hiddenStates: 0 } : happyPath(machine);
  const collapsed = collapseChains(reduced.spec);
  const overlay = walkOverlay(collapsed, visitedStates(machine, walk));
  const ends = finalState(machine, walk);
  const words = machineTools(machine);
  const drawn = new Set((collapsed.spec.states ?? []).map(String));
  const current = currentState ? collapsed.drawnAs(currentState) : undefined;
  const currentHidden = current !== undefined && !drawn.has(current);
  const edge = activeEdge ? { from: collapsed.drawnAs(activeEdge.from), to: collapsed.drawnAs(activeEdge.to) } : undefined;

  return (
    <>
      <div className="machine-counts">
        {machine.states.length} states · {words.length} words
        {walk && <> · {overlay.visited.size} walked</>}
        {overlay.partly.size > 0 && <span className="machine-hidden"> · {overlay.partly.size} partly</span>}
        {currentState && (
          <span className="machine-now" data-testid="machine-current">
            {" "}
            · now {currentState}
            {currentHidden ? " (hidden)" : ""}
          </span>
        )}
        {reduced.hiddenStates > 0 && <span className="machine-hidden"> · {reduced.hiddenStates} failure states hidden</span>}
        {collapsed.folded > 0 && <span className="machine-hidden"> · {collapsed.folded} folded</span>}
        <button type="button" className="machine-toggle" data-testid="machine-toggle" onClick={() => setShowAllStates((was) => !was)}>
          {showAllStates ? "hide the failure paths" : "show every state"}
        </button>
      </div>
      <MachineView
        spec={collapsed.spec}
        visited={overlay.visited}
        partly={overlay.partly}
        finalState={ends ? collapsed.drawnAs(ends) : undefined}
        currentState={current}
        activeEdge={edge}
      />
      <div className="tool-list" data-testid="tool-list">
        {words.map((tool) => (
          <button
            type="button"
            className={`tool-tag${openTool === tool ? " tool-tag-on" : ""}`}
            data-testid="tool-tag"
            key={tool}
            title={String(tools[tool]?.description ?? "")}
            onClick={() => setOpenTool(openTool === tool ? undefined : tool)}
          >
            {tool}
          </button>
        ))}
      </div>
      {openTool && <ToolWindow name={openTool} declaration={tools[openTool]} onClose={() => setOpenTool(undefined)} />}
    </>
  );
}

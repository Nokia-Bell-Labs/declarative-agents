// @vitest-environment jsdom
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { toDeclaredMachine, type MachineSpec } from "../../src/api/monitorApi";
import { toModel } from "../../src/api/traceApi";
import { fixtures } from "../../src/fixtures";
import { AgentPanel, collapseChains, finalState, happyPath, MachineView, visitedStates, walkOverlay } from "../../src/panels/MachineView";

// Ported from agentic-wiki-mesh machineView.test.tsx: the figure drawn over the
// chatbot's real request machine and the walk the recorded turn took, as
// static markup.
const machine = toDeclaredMachine(fixtures["/monitor/machines"][0])! as MachineSpec;
const trace = toModel(fixtures["/query/traces/{trace_id}"]);
const walk = trace.walks.find((candidate) => candidate.service === "chatbot")!;

function drawnTurn() {
  const collapsed = collapseChains(happyPath(machine).spec);
  const { visited, partly } = walkOverlay(collapsed, visitedStates(machine, walk));
  const ends = finalState(machine, walk);
  return {
    collapsed,
    visible: visited,
    partly,
    html: renderToStaticMarkup(<MachineView spec={collapsed.spec} visited={visited} partly={partly} finalState={ends ? collapsed.drawnAs(ends) : undefined} />),
  };
}

function stateMarkup(html: string, state: string): string {
  const marker = `data-state="${state}"`;
  const at = html.indexOf(marker);
  expect(at, `no box drawn for ${state}`).toBeGreaterThan(-1);
  return html.slice(html.lastIndexOf("<g", at), html.indexOf("</g>", at));
}

describe("drawing a machine", () => {
  const toy: MachineSpec = {
    name: "toy",
    initial_state: "A",
    states: ["A", "B"],
    terminal_states: ["B"],
    transitions: [{ state: "A", signal: "Seed", next: "B", action: "one" }],
  };

  it("draws one box per state and an edge per transition", () => {
    const html = renderToStaticMarkup(<MachineView spec={toy} />);
    expect(html).toContain('data-testid="machine-view"');
    expect(html.match(/data-testid="machine-state"/g)).toHaveLength(2);
    expect(html).toContain("machine-state-terminal");
    // The edge carries the signal and the word it dispatches, as its title.
    expect(html).toContain("<title>Seed / one</title>");
  });

  it("marks the live state and the transition it arrived by", () => {
    const html = renderToStaticMarkup(<MachineView spec={toy} currentState="B" activeEdge={{ from: "A", to: "B" }} />);
    expect(stateMarkup(html, "B")).toContain("machine-state-current");
    expect(stateMarkup(html, "A")).not.toContain("machine-state-current");
    expect(html).toContain("machine-edge machine-edge-active");
  });

  it("gives each figure its own arrow marker", () => {
    const html = renderToStaticMarkup(
      <>
        <MachineView spec={toy} />
        <MachineView spec={toy} />
      </>,
    );
    const ids = [...html.matchAll(/<marker id="([^"]+)"/g)].map((match) => match[1]);
    expect(ids).toHaveLength(2);
    expect(new Set(ids).size).toBe(2);
  });

  it("says so when the monitor served no states", () => {
    expect(renderToStaticMarkup(<MachineView spec={{ name: "empty", states: [] }} />)).toContain('data-testid="machine-empty"');
  });
});

describe("overlaying the recorded turn", () => {
  const { html, collapsed, visible, partly } = drawnTurn();

  it("fills the states this turn was in", () => {
    expect(stateMarkup(html, collapsed.drawnAs("AwaitingRequest"))).toContain("machine-state-visited");
    expect(stateMarkup(html, collapsed.drawnAs("ComposingGrounded"))).toContain("machine-state-visited");
    expect(stateMarkup(html, collapsed.drawnAs("Reranking"))).toContain("machine-state-visited");
  });

  it("keeps a folded state's fill on the box that swallowed it", () => {
    // Reranking is drawn inside a fold on this machine; this fails if the
    // overlay is applied before the collapse rather than mapped through it.
    const drawnAs = collapsed.drawnAs("Reranking");
    expect(drawnAs).not.toBe("Reranking");
    expect(stateMarkup(html, drawnAs)).toContain("machine-state-visited");
  });

  it("marks where the turn left the machine", () => {
    expect(html).toContain("machine-state-final");
    expect(stateMarkup(html, collapsed.drawnAs("Finalizing"))).toContain("machine-state-final");
  });

  it("does not claim a turn walked a branch it skipped", () => {
    // The rerank applied, so the skipped outcome never ran; the fold puts
    // DeclaringRerankSkipped in a box the turn partly walked.
    const box = collapsed.drawnAs("DeclaringRerankSkipped");
    expect(visible.has(box)).toBe(false);
    expect(stateMarkup(html, box)).not.toContain("machine-state-visited");
    expect(partly.has(box)).toBe(true);
    expect(stateMarkup(html, box)).toContain("machine-state-partial");
  });

  it("draws no box the machine does not declare", () => {
    const declared = new Set(machine.states);
    const boxes = [...html.matchAll(/data-state="([^"]+)"/g)].map((match) => match[1]);
    expect(boxes.length).toBeGreaterThan(5);
    expect(boxes.every((name) => declared.has(name.replace(/ \+\d+$/, "")))).toBe(true);
  });
});

// The panel around the figure. Static markup runs no effects, so this pins the
// shell an operator sees while the monitor is being read, and that the agents
// offered are the ones this trace carries.
describe("the agent panel", () => {
  it("offers the services of the open trace and labels each with its program", () => {
    const html = renderToStaticMarkup(<AgentPanel trace={trace} serviceLabels={new Map([["knowledge0", "knowledge-manager"]])} />);
    expect(html).toContain('data-testid="agent-machine"');
    expect(html).toContain('data-testid="agent-picker"');
    expect(html).toContain("<option");
    expect(html).toContain("knowledge0 (knowledge-manager)");
    expect(html).toContain("Reading chatbot&#x27;s declared machines");
  });

  it("labels nothing without a label map", () => {
    const html = renderToStaticMarkup(<AgentPanel trace={trace} />);
    expect(html).not.toContain("knowledge0 (");
  });
});

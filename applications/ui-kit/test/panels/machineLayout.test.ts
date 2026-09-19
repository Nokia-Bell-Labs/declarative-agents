import { describe, expect, it } from "vitest";

import { toDeclaredMachine, type MachineSpec } from "../../src/api/monitorApi";
import { fixtures } from "../../src/fixtures";
import { collapseChains, happyPath, layoutMachine } from "../../src/panels/MachineView";

// Ported from agentic-wiki-mesh machineLayout.test.ts; the real machine is the
// kit's live recording of the chatbot request machine.
const machine = toDeclaredMachine(fixtures["/monitor/machines"][0])! as MachineSpec;

describe("laying a machine out", () => {
  const spec: MachineSpec = {
    name: "toy",
    initial_state: "A",
    states: ["A", "B", "C", "D", "Orphan"],
    terminal_states: ["D"],
    transitions: [
      { state: "A", signal: "Seed", next: "B", action: "one" },
      { state: "B", signal: "Did", next: "C", action: "two" },
      { state: "A", signal: "Skipped", next: "C", action: "three" },
      { state: "C", signal: "Done", next: "D" },
    ],
  };

  it("places each state at its shortest distance from the initial state", () => {
    const layout = layoutMachine(spec);
    const column = (name: string) => layout.states.find((state) => state.name === name)!.column;
    expect(column("A")).toBe(0);
    expect(column("B")).toBe(1);
    // Two ways to C -- through B, or straight from A. The shorter one wins.
    expect(column("C")).toBe(1);
    expect(column("D")).toBe(2);
  });

  it("puts a state no transition reaches past the rest", () => {
    const layout = layoutMachine(spec);
    const orphan = layout.states.find((state) => state.name === "Orphan")!;
    expect(orphan.column).toBe(3);
    expect(layout.columns).toBe(4);
  });

  it("labels an edge with the signal that fires it and the word it dispatches", () => {
    const edge = layoutMachine(spec).edges.find((e) => e.from === "A" && e.to === "B")!;
    expect(edge.label).toBe("Seed / one");
    expect(layoutMachine(spec).edges.find((e) => e.from === "C")!.label).toBe("Done");
  });

  it("marks the terminal states and survives an empty machine", () => {
    expect(layoutMachine(spec).states.find((s) => s.name === "D")!.terminal).toBe(true);
    expect(layoutMachine({})).toEqual({ states: [], edges: [], columns: 0 });
  });

  it("starts from the declared initial state even when it is not listed first", () => {
    const layout = layoutMachine({ initial_state: "B", states: ["A", "B"], transitions: [{ state: "B", signal: "Go", next: "A" }] });
    expect(layout.states.find((state) => state.name === "B")!.column).toBe(0);
    expect(layout.states.find((state) => state.name === "A")!.column).toBe(1);
  });

  it("lays out the chatbot's real request machine", () => {
    const layout = layoutMachine(machine);
    expect(layout.states).toHaveLength(machine.states!.length);
    expect(layout.states.find((state) => state.name === "AwaitingRequest")!.column).toBe(0);
    // Every edge of the figure joins two states the machine declares.
    const known = new Set(machine.states);
    expect(layout.edges.every((edge) => known.has(edge.from) && known.has(edge.to))).toBe(true);
  });
});

describe("reducing a machine to what a reader can follow", () => {
  const spec: MachineSpec = {
    name: "toy",
    initial_state: "A",
    states: ["A", "B", "C", "D", "Failed"],
    terminal_states: ["D", "Failed"],
    transitions: [
      { state: "A", signal: "Seed", next: "B", action: "one" },
      { state: "B", signal: "Did", next: "C", action: "two" },
      { state: "C", signal: "Done", next: "D", action: "three" },
      { state: "A", signal: "CommandError", next: "Failed" },
      { state: "B", signal: "CommandError", next: "Failed" },
    ],
  };

  it("drops the failure edges and the states only they reach", () => {
    const reduced = happyPath(spec);
    expect(reduced.spec.states).toEqual(["A", "B", "C", "D"]);
    expect(reduced.spec.terminal_states).toEqual(["D"]);
    expect(reduced.hiddenEdges).toBe(2);
    expect(reduced.hiddenStates).toBe(1);
    // Ported from cohere-demo: the footnote can name what it hid.
    expect(reduced.hiddenStateNames).toEqual(["Failed"]);
  });

  it("folds a run with one way in and one way out into its head", () => {
    const collapsed = collapseChains(happyPath(spec).spec);
    // B and C can only be walked in that order, so they fold into A.
    expect(collapsed.spec.states).toEqual(["A +2", "D"]);
    expect(collapsed.folded).toBe(2);
    // The chain's internal edges go with it; what is left is the one edge out
    // of the fold, re-rooted on the head and keeping the signal that fires it.
    expect(collapsed.spec.transitions).toEqual([{ state: "A +2", signal: "Done", next: "D", action: "three" }]);
  });

  it("keeps an overlay of a folded state pointing at the box that swallowed it", () => {
    const collapsed = collapseChains(happyPath(spec).spec);
    // This is what makes the walk overlay survive the fold: a turn that ran a
    // word in C marks the box drawn as "A +2", rather than marking nothing.
    expect(collapsed.drawnAs("C")).toBe("A +2");
    expect(collapsed.drawnAs("A")).toBe("A +2");
    expect(collapsed.drawnAs("D")).toBe("D");
  });

  it("says which declared states a drawn box stands for", () => {
    const collapsed = collapseChains(happyPath(spec).spec);
    // The box is one drawing of three states, and an overlay that cannot see
    // that would fill it for a turn that walked only one of them.
    expect(collapsed.membersOf("A +2")).toEqual(["A", "B", "C"]);
    expect(collapsed.membersOf("D")).toEqual(["D"]);
    // A name nothing folded stands for itself.
    expect(collapsed.membersOf("Unknown")).toEqual(["Unknown"]);
  });

  it("folds nothing when every state branches", () => {
    const branching: MachineSpec = {
      states: ["A", "B", "C"],
      initial_state: "A",
      terminal_states: [],
      transitions: [
        { state: "A", signal: "Left", next: "B" },
        { state: "A", signal: "Right", next: "C" },
      ],
    };
    const collapsed = collapseChains(branching);
    expect(collapsed.folded).toBe(0);
    expect(collapsed.spec.states).toEqual(["A", "B", "C"]);
  });

  it("makes the chatbot's request machine small enough to read", () => {
    const full = machine.states!.length;
    const drawn = collapseChains(happyPath(machine).spec).spec.states ?? [];
    // 57 states of declared program, most of them a single line of a pipeline.
    expect(drawn.length).toBeLessThan(full / 2);
    // And nothing invented: every drawn box is a declared state, or a declared
    // state with the count it swallowed.
    const declared = new Set(machine.states);
    expect(drawn.every((name) => declared.has(name.replace(/ \+\d+$/, "")))).toBe(true);
  });
});

import { describe, expect, it } from "vitest";
import { fetchDeclaredMachines, fetchDeclaredTools, toDeclaredMachine, type DeclaredMachine } from "../../src/api/monitorApi";
import { toModel } from "../../src/api/traceApi";
import { createKitClient } from "../../src/client/client";
import { fixtureFetch, fixtures } from "../../src/fixtures";
import { collapseChains, finalState, machineForWalk, machineTools, orderedMachines, visitedStates, walkOverlay } from "../../src/panels/MachineView";

// Ported from agentic-wiki-mesh machineViews.test.ts. The machine is the kit's
// live recording of the payload the chatbot's monitor served, read through the
// normalizer the panel uses, and the walk is the recorded trace's. The reads
// now go through the kit client instead of a stubbed global fetch.
const machine = toDeclaredMachine(fixtures["/monitor/machines"][0])!;
const trace = toModel(fixtures["/query/traces/{trace_id}"]);
const chatbotWalk = trace.walks.find((walk) => walk.service === "chatbot")!;

function clientServing(machines?: unknown, tools?: unknown, absentAgents: string[] = []) {
  const overrides: Record<string, unknown> = {};
  if (machines !== undefined) overrides["/monitor/machines"] = machines;
  if (tools !== undefined) overrides["/monitor/tools/declared"] = tools;
  return createKitClient({ fetch: fixtureFetch({ overrides, absentAgents }) });
}

describe("reading an agent's declared machines and words", () => {
  it("reads both views through the same-origin monitor proxy", async () => {
    const seen: string[] = [];
    const inner = fixtureFetch();
    const client = createKitClient({
      fetch: (input, init) => {
        seen.push(String(input));
        return inner(input, init);
      },
    });
    await fetchDeclaredMachines(client, "knowledge0");
    await fetchDeclaredTools(client, "knowledge0");
    expect(seen).toEqual(["/monitor-proxy/knowledge0/monitor/machines", "/monitor-proxy/knowledge0/monitor/tools/declared"]);
  });

  it("keeps the machines that carry states and transitions, and fills the initial state", async () => {
    const client = clientServing([
      { name: "chatbot", states: ["Idle", "Done"], transitions: [{ state: "Idle", signal: "Seed", next: "Done" }] },
      { name: "half-built", states: ["Idle"] },
      null,
    ]);
    const machines = await fetchDeclaredMachines(client, "chatbot");
    expect(machines.map((m) => m.name)).toEqual(["chatbot"]);
    expect(machines[0].initial_state).toBe("Idle");
    expect(machines[0].terminal_states).toEqual([]);
  });

  it("reads a state whichever way the monitor writes it", async () => {
    const client = clientServing([
      {
        name: "chatbot-turn",
        initial_state: "AwaitingRequest",
        states: [{ name: "AwaitingRequest", meaning: "Seeded by the endpoint." }, { name: "Failed" }],
        terminal_states: ["Failed"],
        transitions: [{ state: "AwaitingRequest", signal: "Seed", next: "Failed", action: "capture_request" }],
      },
    ]);
    const [read] = await fetchDeclaredMachines(client, "chatbot");
    expect(read.states).toEqual(["AwaitingRequest", "Failed"]);
    // And the fixture the other tests read is that shape, from the wire.
    expect(machine.states.every((state) => typeof state === "string")).toBe(true);
    expect(machine.states).toContain("ComposingGrounded");
  });

  it("degrades to nothing when an agent does not serve the view", async () => {
    const absent = clientServing(undefined, undefined, ["page-writer"]);
    expect(await fetchDeclaredMachines(absent, "page-writer")).toEqual([]);
    expect(await fetchDeclaredTools(absent, "page-writer")).toEqual({});
    const refused = createKitClient({
      fetch: async () => {
        throw new Error("connection refused");
      },
    });
    expect(await fetchDeclaredMachines(refused, "page-writer")).toEqual([]);
  });

  it("keys the declarations by the word they declare", async () => {
    const tools = await fetchDeclaredTools(clientServing(undefined, [{ name: "invoke_cohere_command", init: "rest_client_invoke" }, { init: "compose" }]), "chatbot");
    expect(Object.keys(tools)).toEqual(["invoke_cohere_command"]);
    expect(tools.invoke_cohere_command.init).toBe("rest_client_invoke");
  });

  it("puts the machine the turn ran in first", () => {
    const machines = [{ name: "chatbot" }, { name: "chatbot-turn" }] as DeclaredMachine[];
    expect(orderedMachines(machines, "chatbot-turn").map((m) => m.name)).toEqual(["chatbot-turn", "chatbot"]);
    expect(orderedMachines(machines, undefined).map((m) => m.name)).toEqual(["chatbot", "chatbot-turn"]);
  });

  it("can demote the lifecycle supervisor behind the request workflows (cohere-demo)", () => {
    const machines = [{ name: "chatbot" }, { name: "chatbot-export-relay" }, { name: "chatbot-turn" }] as DeclaredMachine[];
    const names = (list: DeclaredMachine[]) => list.map((m) => m.name);
    expect(names(orderedMachines(machines, "chatbot-turn", { supervisorLast: true }))).toEqual(["chatbot-turn", "chatbot-export-relay", "chatbot"]);
    expect(names(orderedMachines(machines, undefined, { supervisorLast: true }))).toEqual(["chatbot-export-relay", "chatbot-turn", "chatbot"]);
    // Asking for the supervisor itself keeps it first.
    expect(names(orderedMachines(machines, "chatbot", { supervisorLast: true }))[0]).toBe("chatbot");
    expect(names(orderedMachines([machines[0]], "chatbot-turn", { supervisorLast: true }))).toEqual(["chatbot"]);
  });

  it("lists the words a machine dispatches, skipping the ones resolved at run time", () => {
    const tools = machineTools({
      states: ["A", "B"],
      transitions: [
        { state: "A", action: "capture_request" },
        { state: "A", action: "capture_request" },
        { state: "B", action: "$dynamic" },
        { state: "B" },
      ],
    });
    expect(tools).toEqual(["capture_request"]);
  });
});

// The join the figure rests on, over the chatbot's real request machine and
// the walk the recorded turn took.
describe("overlaying a walk on the machine it walked", () => {
  const visited = visitedStates(machine, chatbotWalk);

  it("marks the states the turn ran its words in", () => {
    for (const state of ["AwaitingRequest", "DefaultingCitations", "ComposingSourceSelection", "CheckingView", "Reranking", "ComposingGrounded", "DeclaringPageNone"]) {
      expect(visited.has(state), `${state} should be marked`).toBe(true);
    }
  });

  it("leaves the branches this turn did not take unmarked", () => {
    for (const state of ["DeclaringRerankSkipped", "DeclaringRerankDegraded", "ComposingTaggedView", "RenderingTagClauses", "ComposingWindowView"]) {
      expect(visited.has(state), `${state} should not be marked`).toBe(false);
    }
  });

  it("marks no state at all without a walk", () => {
    expect(visitedStates(machine, undefined).size).toBe(0);
    expect(visitedStates(machine, { service: "chatbot", steps: [] }).size).toBe(0);
  });

  it("marks only the state a word ran in, not the one its signal moved to", () => {
    const oneWord = { service: "chatbot", steps: [{ iteration: 34, command: "invoke_cohere_command", signal: "AnswerComposed" }] };
    const marked = visitedStates(machine, oneWord);
    expect(marked.has("ComposingGrounded")).toBe(true);
    expect(marked.has("PartitioningAnswerBlocks")).toBe(false);
  });

  it("names where the walk left the machine", () => {
    expect(finalState(machine, chatbotWalk)).toBe("Finalizing");
  });

  it("picks the machine whose words the walk ran", () => {
    const lifecycle = { name: "chatbot", initial_state: "Idle", states: ["Idle"], terminal_states: [], transitions: [{ state: "Idle", action: "launch" }] };
    expect(machineForWalk([lifecycle, machine], chatbotWalk)?.name).toBe(machine.name);
    expect(machineForWalk([lifecycle, machine], undefined)?.name).toBe("chatbot");
    expect(machineForWalk([], chatbotWalk)).toBeUndefined();
  });

  it("fills a folded box only when the turn walked every state in it", () => {
    const collapsed = collapseChains({
      initial_state: "A",
      states: ["A", "B", "C", "D"],
      terminal_states: ["D"],
      transitions: [
        { state: "A", signal: "Go", next: "B" },
        { state: "B", signal: "Go", next: "C" },
        { state: "C", signal: "Go", next: "D" },
      ],
    });
    expect(collapsed.spec.states).toEqual(["A +2", "D"]);
    expect(walkOverlay(collapsed, new Set(["A", "B", "C"]))).toEqual({ visited: new Set(["A +2"]), partly: new Set() });
    expect(walkOverlay(collapsed, new Set(["A"]))).toEqual({ visited: new Set(), partly: new Set(["A +2"]) });
  });
});

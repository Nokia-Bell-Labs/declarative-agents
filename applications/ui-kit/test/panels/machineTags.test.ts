import { describe, expect, it } from "vitest";
import { createKitClient } from "../../src/client/client";
import { fixtureFetch, fixtures } from "../../src/fixtures";
import { activeStage, fetchTaggedMachines, sharedSink, tagView, toTaggedMachine, withoutSink } from "../../src/panels/MachineView";

// Ported from cohere-demo declaredViews.test.ts and machineCollapse.test.ts:
// stage views over a machine whose declaration tags its states.
const wire = [
  {
    name: "chatbot",
    initial_state: "Idle",
    states: [{ name: "Idle" }, { name: "Launching" }],
    transitions: [{ state: "Idle", signal: "Seed", next: "Launching", action: "launch_chat_requests" }],
  },
  {
    name: "chatbot-turn",
    initial_state: "AwaitingRequest",
    view_tags: [
      { tag: "intake", label: "Intake & clearance" },
      { tag: "compose", label: "Composition & tier" },
    ],
    states: [
      { name: "AwaitingRequest", tags: ["intake"] },
      { name: "Embedding", tags: ["intake"] },
      { name: "Answering", tags: ["compose"] },
      { name: "Failed", tags: ["intake", "compose"] },
    ],
    transitions: [
      { state: "AwaitingRequest", signal: "Seed", next: "Embedding", action: "embed_query_cohere" },
      { state: "Embedding", signal: "CommandError", next: "Failed" },
      { state: "Embedding", signal: "QueryEmbedded", next: "Answering", action: "compose_prompt" },
      { state: "Answering", signal: "ToolDone", next: "Answering", action: "$tool" },
      { state: "Answering", signal: "Retry", next: "Embedding", action: "embed_query_cohere" },
    ],
  },
];

const turn = toTaggedMachine(wire[1])!;

describe("stage views over a tagged machine", () => {
  it("reads the machines with their tags through the kit client", async () => {
    const client = createKitClient({ fetch: fixtureFetch({ overrides: { "/monitor/machines": wire }, absentAgents: ["rag1"] }) });
    const machines = await fetchTaggedMachines(client, "chatbot");
    expect(machines.map((m) => m.name)).toEqual(["chatbot", "chatbot-turn"]);
    expect(machines[1].view_tags.map((view) => view.tag)).toEqual(["intake", "compose"]);
    expect(machines[1].state_tags.Failed).toEqual(["intake", "compose"]);
    expect(machines[1].states).toEqual(["AwaitingRequest", "Embedding", "Answering", "Failed"]);
    expect(await fetchTaggedMachines(client, "rag1")).toEqual([]);
  });

  it("treats an untagged machine as one figure", () => {
    const plain = toTaggedMachine(fixtures["/monitor/machines"][0])!;
    expect(plain.view_tags).toEqual([]);
    expect(sharedSink(plain)).toBeUndefined();
    expect(activeStage(plain, { service: "chatbot", steps: [{ iteration: 1, command: "capture_request", signal: "RequestCaptured" }] })).toBeUndefined();
  });

  it("cuts the states carrying a tag and the transitions between them", () => {
    const view = tagView(turn, "intake");
    expect(view.states).toEqual(["AwaitingRequest", "Embedding", "Failed"]);
    expect(view.transitions?.map((t) => `${t.state}>${t.next}`)).toEqual(["AwaitingRequest>Embedding", "Embedding>Failed"]);
    expect(view.terminal_states).toEqual(["Failed"]);
  });

  it("takes the state carrying every tag out as the shared sink, with a count", () => {
    expect(sharedSink(turn)).toBe("Failed");
    const view = withoutSink(turn);
    expect(view.sink).toBe("Failed");
    expect(view.sinkEdges).toBe(1);
    expect(view.machine.states).toEqual(["AwaitingRequest", "Embedding", "Answering"]);
    expect(view.machine.transitions).toHaveLength(4);
    const single = { ...turn, view_tags: [{ tag: "intake", label: "Intake" }] };
    expect(sharedSink(single)).toBeUndefined();
    expect(withoutSink(single).machine).toBe(single);
  });

  it("names the stage the walk last reached", () => {
    const stage = activeStage(turn, {
      service: "chatbot",
      steps: [
        { iteration: 1, command: "embed_query_cohere", signal: "QueryEmbedded" },
        { iteration: 2, command: "compose_prompt", signal: "Composed" },
      ],
    });
    expect(stage).toBe("compose");
    expect(activeStage(turn, { service: "chatbot", steps: [] })).toBeUndefined();
  });
});

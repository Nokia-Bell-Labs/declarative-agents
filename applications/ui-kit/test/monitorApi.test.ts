import { describe, expect, it } from "vitest";
import { createKitClient } from "../src/client/client";
import { fetchAgentState, fetchDeclaredMachines, fetchDeclaredTools, parseMonitorFrame, pollDelay, statusAfterStateFailure, toDeclaredMachine } from "../src/api/monitorApi";
import { fixtureFetch, fixtures } from "../src/fixtures";

const client = createKitClient({ fetch: fixtureFetch({ absentAgents: ["rag1"] }) });

describe("monitor api", () => {
  it("reads a 404 as not deployed and other failures as blips or errors", () => {
    expect(statusAfterStateFailure("connecting", 404)).toBe("absent");
    expect(statusAfterStateFailure("connected", 500)).toBe("connected");
    expect(statusAfterStateFailure("connecting", 500)).toBe("error");
    expect(statusAfterStateFailure("absent", 502)).toBe("error");
    expect(pollDelay("absent")).toBeGreaterThan(pollDelay("connected"));
  });

  it("shapes a run-event frame and keeps a non-JSON frame raw", () => {
    const event = parseMonitorFrame(1, "run_event", fixtures["/monitor/events/stream"][0].data, 5);
    expect(event).toMatchObject({ id: 1, fromState: "Embedding", toState: "DeclaringModel", signal: "QueryEmbedded", commandName: "embed_query", receivedAt: 5 });
    expect(parseMonitorFrame(2, "notice", "not json", 6).raw).toBe("not json");
  });

  it("reads state through the proxy and reports an absent agent", async () => {
    const state = await fetchAgentState(client, "chatbot");
    expect(state).toEqual({ deployed: true, body: fixtures["/monitor/state"] });
    expect(await fetchAgentState(client, "rag1")).toEqual({ deployed: false });
  });

  it("reduces declared machine states to names", async () => {
    const machines = await fetchDeclaredMachines(client, "chatbot");
    expect(machines).toHaveLength(1);
    expect(machines[0].states.every((state) => typeof state === "string")).toBe(true);
    expect(machines[0].initial_state).toBe(machines[0].states[0]);
    expect(await fetchDeclaredMachines(client, "rag1")).toEqual([]);
    expect(toDeclaredMachine({ name: "m" })).toBeUndefined();
  });

  it("keys declared tools by name and accepts the tools envelope", async () => {
    expect(Object.keys(await fetchDeclaredTools(client, "chatbot"))).toEqual(["capture_request", "invoke_llm_fast"]);
    const enveloped = createKitClient({ fetch: fixtureFetch({ overrides: { "/monitor/tools/declared": { tools: [{ name: "x" }] } } }) });
    expect(Object.keys(await fetchDeclaredTools(enveloped, "chatbot"))).toEqual(["x"]);
    expect(await fetchDeclaredTools(client, "rag1")).toEqual({});
  });
});

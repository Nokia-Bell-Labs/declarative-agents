// @vitest-environment jsdom
import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { toDeclaredMachine } from "../../src/api/monitorApi";
import { toModel } from "../../src/api/traceApi";
import { fixtures } from "../../src/fixtures";
import { definePanel } from "../../src/panels/manifest";
import { AgentPanel, collapseChains, happyPath, machineViewConfig, machineViewManifest, machineViewPanel } from "../../src/panels/MachineView";
import { FakeEventSource, FixtureProvider } from "./support";

// The mountable panel and AgentPanel rendered against the recorded contract
// fixtures (srd004 R3.2).
const agents = [
  { name: "chatbot", label: "Chatbot" },
  { name: "rag1", label: "RAG server 1" },
];

const flat = {
  name: "flat",
  initial_state: "Idle",
  states: [{ name: "Idle" }, { name: "Working" }, { name: "Done" }],
  terminal_states: ["Done"],
  transitions: [
    { state: "Idle", signal: "Seed", next: "Working", action: "capture_request" },
    { state: "Working", signal: "Worked", next: "Idle", action: "invoke_llm_fast" },
    { state: "Working", signal: "Finished", next: "Done", action: "invoke_llm_fast" },
  ],
};

const Mount = machineViewPanel.component;

function boxes(): string[] {
  return screen.getAllByTestId("machine-state").map((node) => node.getAttribute("data-state") ?? "");
}

function box(name: string): Element {
  return screen.getAllByTestId("machine-state").find((node) => node.getAttribute("data-state") === name)!;
}

afterEach(() => cleanup());

describe("machine view panel", () => {
  it("publishes its manifest through definePanel", () => {
    expect(machineViewPanel.manifest).toBe(machineViewManifest);
    expect(machineViewManifest).toMatchObject({ id: "machine-view", route: "/machines", monitored_agents: "declared" });
    expect(() => definePanel(machineViewManifest, Mount)).not.toThrow();
  });

  it("draws a flat machine as declared, with its words", async () => {
    render(
      <FixtureProvider overrides={{ "/monitor/machines": [flat] }}>
        <Mount monitoredAgents={agents} />
      </FixtureProvider>,
    );
    await waitFor(() => expect(screen.getByTestId("machine-live").getAttribute("data-machine")).toBe("flat"));
    expect(boxes()).toEqual(["Idle", "Working", "Done"]);
    expect(screen.queryByText(/folded/)).toBeNull();
    expect(screen.getAllByTestId("tool-tag").map((node) => node.textContent)).toEqual(["capture_request", "invoke_llm_fast"]);
    fireEvent.click(screen.getAllByTestId("tool-tag")[1]);
    expect(screen.getByTestId("tool-window").getAttribute("data-tool")).toBe("invoke_llm_fast");
    expect(screen.getByText("emits LLMResponded, LLMFailed")).toBeTruthy();
  });

  it("folds the recorded request machine and unfolds it on demand", async () => {
    render(
      <FixtureProvider>
        <Mount monitoredAgents={agents} />
      </FixtureProvider>,
    );
    await waitFor(() => expect(screen.getByTestId("machine-live").getAttribute("data-machine")).toBe("chatbot-turn"));
    const machine = toDeclaredMachine(fixtures["/monitor/machines"][0])!;
    const collapsed = collapseChains(happyPath(machine).spec);
    expect(boxes()).toEqual(collapsed.spec.states);
    expect(boxes().length).toBeLessThan(machine.states.length / 2);
    expect(screen.getByText(/folded/)).toBeTruthy();
    fireEvent.click(screen.getByTestId("machine-toggle"));
    expect(boxes().length).toBeGreaterThan(collapsed.spec.states!.length);
    expect(screen.getByTestId("machine-toggle").textContent).toBe("hide the failure paths");
  });

  it("marks the live state and the transition the run took last", async () => {
    render(
      <FixtureProvider overrides={{ "/monitor/machines": [flat], "/monitor/state": { run: { state: "Working", status: "running" } } }}>
        <Mount monitoredAgents={agents} />
      </FixtureProvider>,
    );
    await waitFor(() => expect(box("Working").getAttribute("class")).toContain("machine-state-current"));
    expect(screen.getByTestId("machine-current").textContent).toContain("now Working");
    expect(box("Idle").getAttribute("class")).not.toContain("machine-state-current");
    act(() => FakeEventSource.last!.emit("run_event", JSON.stringify({ from_state: "Idle", to_state: "Working", signal: "Seed" })));
    const active = document.querySelectorAll(".machine-edge-active");
    expect(active).toHaveLength(1);
    expect(active[0].textContent).toBe("Seed / capture_request");
  });

  it("marks the recorded run's live state through the fold", async () => {
    render(
      <FixtureProvider>
        <Mount monitoredAgents={agents} />
      </FixtureProvider>,
    );
    const machine = toDeclaredMachine(fixtures["/monitor/machines"][0])!;
    const drawn = collapseChains(happyPath(machine).spec).drawnAs(fixtures["/monitor/state"].run!.state!);
    await waitFor(() => expect(box(drawn).getAttribute("class")).toContain("machine-state-current"));
  });

  it("reports an agent the proxy says is not deployed instead of failing", async () => {
    render(
      <FixtureProvider absentAgents={["rag1"]}>
        <Mount monitoredAgents={[agents[1], agents[0]]} />
      </FixtureProvider>,
    );
    await waitFor(() => expect(screen.getByTestId("machine-not-deployed").getAttribute("data-hidden")).toBe("true"));
    expect(screen.queryByTestId("machine-view")).toBeNull();
    expect(screen.queryByTestId("machine-unavailable")).toBeNull();
    // Picking a deployed agent draws its machine.
    fireEvent.change(screen.getByTestId("agent-picker"), { target: { value: "chatbot" } });
    await waitFor(() => expect(screen.getByTestId("machine-live")).toBeTruthy());
  });

  it("says so when no monitored agents are declared", () => {
    render(
      <FixtureProvider>
        <Mount monitoredAgents={[]} />
      </FixtureProvider>,
    );
    expect(screen.getByTestId("machine-panel").textContent).toContain("No monitored agents");
  });

  it("reads its ui.yaml config defensively", () => {
    expect(machineViewConfig({ preferred_machines: { chatbot: "chatbot-turn", bad: 3 }, supervisor_last: true })).toEqual({
      preferred_machines: { chatbot: "chatbot-turn" },
      supervisor_last: true,
    });
    expect(machineViewConfig({ preferred_machines: ["x"], supervisor_last: "yes" })).toEqual({});
    expect(machineViewConfig(undefined)).toEqual({});
  });
});

describe("agent panel", () => {
  const trace = toModel(fixtures["/query/traces/{trace_id}"]);

  it("draws the machine the trace's walk ran in, with the walk filled", async () => {
    render(
      <FixtureProvider>
        <AgentPanel trace={trace} />
      </FixtureProvider>,
    );
    await waitFor(() => expect(screen.getByTestId("agent-machine").getAttribute("data-machine")).toBe("chatbot-turn"));
    expect(screen.getByTestId("machine-picker").textContent).toContain("ran this turn");
    expect(document.querySelectorAll(".machine-state-visited").length).toBeGreaterThan(0);
    expect(document.querySelectorAll(".machine-state-final")).toHaveLength(1);
  });

  it("reads each service through the monitored agent it maps to", async () => {
    render(
      <FixtureProvider absentAgents={["knowledge0"]}>
        <AgentPanel trace={trace} initialService="knowledge0" monitorAgentFor={(service) => (service === "knowledge0" ? "chatbot" : service)} />
      </FixtureProvider>,
    );
    await waitFor(() => expect(screen.getByTestId("agent-machine").getAttribute("data-machine")).toBe("chatbot-turn"));
  });

  it("stays usable when a service serves no machines", async () => {
    render(
      <FixtureProvider absentAgents={["knowledge0"]}>
        <AgentPanel trace={trace} initialService="knowledge0" />
      </FixtureProvider>,
    );
    await waitFor(() => expect(screen.getByTestId("machine-unavailable")).toBeTruthy());
  });
});

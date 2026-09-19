// @vitest-environment jsdom
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { topologyManifest, topologyPanel, Topology } from "../../src/panels/Topology";
import { FixtureProvider } from "./support";

const Mount = topologyPanel.component;
const emptyTraces = { traces: [], total: 0, offset: 0, page_size: 15 };

afterEach(cleanup);

describe("Topology", () => {
  it("exports its manifest through definePanel", () => {
    expect(topologyPanel.manifest).toBe(topologyManifest);
    expect(topologyManifest).toEqual({
      id: "topology",
      title: "Topology",
      route: "/topology",
      required_endpoints: ["/monitor/fleet", "/query/traces", "/query/traces/{trace_id}"],
      monitored_agents: [],
    });
  });

  it("derives the graph from recent traces on the declared backend", async () => {
    const { container } = render(
      <FixtureProvider>
        <Mount monitoredAgents={[]} traceBackend="collector" config={{ roleAnnotation: "mesh.example/role", peerRoles: { "api.cohere.com": "Cohere API" } }} />
      </FixtureProvider>,
    );
    await waitFor(() => expect(container.querySelector('[data-node="demo-chatbot-mesh-chatbot"]')).toBeTruthy());
    expect(screen.getByTestId("topology-graph")).toBeTruthy();
    const node = (id: string) => container.querySelector(`[data-node="${id}"]`);
    expect(node("chatbot")?.getAttribute("class")).toContain("topo-node-agent");
    expect(node("api.cohere.com")?.getAttribute("class")).toContain("topo-node-external");
    expect(node("host.docker.internal")?.getAttribute("class")).toContain("topo-node-host");
    expect(node("api.cohere.com")?.querySelector("title")?.textContent).toBe("Cohere API");
    // The deployment is seeded from inventory, quiet, and carries its declared role.
    expect(node("demo-chatbot-mesh-chatbot")?.getAttribute("class")).toContain("topo-node-quiet");
    expect(node("demo-chatbot-mesh-chatbot")?.querySelector("title")?.textContent).toContain("chat front door");
    expect(container.querySelector(".topo-edge-egress title")?.textContent).toMatch(/call/);
  });

  it("degrades to the service list with a stated reason when there are no traces", async () => {
    render(
      <FixtureProvider overrides={{ "/query/traces": emptyTraces }}>
        <Mount monitoredAgents={[]} config={{ roleAnnotation: "mesh.example/role" }} />
      </FixtureProvider>,
    );
    await waitFor(() => expect(screen.getByText("service: demo-chatbot-mesh-chatbot")).toBeTruthy());
    expect(screen.getByTestId("topology-reason").textContent).toBe("no recent traces to derive the graph from — showing the service list");
    expect(screen.getByText("chat front door")).toBeTruthy();
    expect(screen.getByText("chatbot-7d9c8b6f4-x2k9p · 2500u/8704Ki")).toBeTruthy();
    expect(screen.queryByTestId("topology-graph")).toBeNull();
  });

  it("states the reason when the trace backend is not deployed", async () => {
    render(
      <FixtureProvider absentAgents={["collector"]}>
        <Mount monitoredAgents={[]} />
      </FixtureProvider>,
    );
    await waitFor(() => expect(screen.getByTestId("topology-reason").textContent).toMatch(/^trace backend collector unavailable: .*404/));
    expect(screen.getByTestId("topology-list")).toBeTruthy();
  });

  it("renders an empty fleet with no traces as an empty inventory, not a broken panel", async () => {
    render(
      <FixtureProvider overrides={{ "/monitor/fleet": {}, "/query/traces": emptyTraces }}>
        <Mount monitoredAgents={[]} />
      </FixtureProvider>,
    );
    await waitFor(() => expect(screen.getByTestId("topology-reason")).toBeTruthy());
    expect(screen.getByText("no deployments or services discovered yet")).toBeTruthy();
  });

  it("says it is deriving before the first read lands", () => {
    render(<Topology data={{ agents: [], pods: [], deployments: [], services: [], podMetrics: [] }} read={{ status: "deriving" }} />);
    expect(screen.getByText("deriving the graph from recent traces…")).toBeTruthy();
  });

  it("renders the service list without a role when no annotation key is given (chatbot-mesh render case)", () => {
    render(
      <Topology
        read={{ status: "no-traces", reason: "no recent traces to derive the graph from" }}
        data={{
          agents: [],
          pods: [{ metadata: { name: "chatbot-0", labels: { "app.kubernetes.io/component": "chatbot" } } }],
          deployments: [{ metadata: { name: "chatbot", labels: { "app.kubernetes.io/component": "chatbot" }, annotations: { "mesh.example/role": "front door" } } }],
          services: [{ metadata: { name: "chatbot" }, spec: { selector: { "app.kubernetes.io/component": "chatbot" } } }],
          podMetrics: [],
        }}
      />,
    );
    expect(screen.getByText("service: chatbot")).toBeTruthy();
    expect(screen.getByText("chatbot-0 · n/a")).toBeTruthy();
    expect(screen.queryByText("front door")).toBeNull();
  });
});

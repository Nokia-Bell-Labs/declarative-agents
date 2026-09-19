// @vitest-environment jsdom
import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { readFileSync } from "node:fs";
import { afterEach, describe, expect, it } from "vitest";
import { parse } from "yaml";
import { createKitClient } from "../src/client/client";
import { fixtureFetch } from "../src/fixtures";
import type { PanelProps } from "../src/panels/manifest";
import { kitPanelRegistry } from "../src/panels/registry";
import { AppShell } from "../src/shell/AppShell";
import type { UIConfig } from "../src/shell/uiConfig";
import { FakeEventSource } from "./panels/support";

const load = (url: string) => parse(readFileSync(new URL(url, import.meta.url).pathname, "utf8")) as UIConfig;
const v2 = load("../../../magefiles/uiyaml/testdata/v2.yaml");
const chatbotMesh = load("../../chatbot-mesh/agents/chatbot/ui/ui.yaml");

const client = (absentAgents: string[] = []) =>
  createKitClient({ fetch: fixtureFetch({ absentAgents }), EventSource: FakeEventSource as unknown as typeof EventSource });

// A stub domain panel that shows what the shell passed it.
const stub = (name: string) =>
  function Stub({ config, monitoredAgents, traceBackend }: PanelProps) {
    return (
      <p data-testid="mounted" data-config={JSON.stringify(config ?? null)} data-agents={monitoredAgents.map((agent) => agent.name).join(",")} data-backend={traceBackend}>
        {name}
      </p>
    );
  };

const linkLabels = () => screen.getAllByRole("link").map((link) => link.textContent);

afterEach(() => {
  cleanup();
  window.history.replaceState(null, "", "/");
});

describe("AppShell (srd004 R5)", () => {
  it("renders the sidebar and routes from a version 2 ui.yaml", async () => {
    window.history.replaceState(null, "", "/ui/");
    render(<AppShell config={v2} registry={{ chat: stub("chat panel"), help: stub("help page") }} client={client()} />);

    expect(document.querySelector(".sidebar-title")?.textContent).toBe("Demo");
    expect(Array.from(document.querySelectorAll(".nav-group-label")).map((node) => node.textContent)).toEqual(["Talk", "Observe"]);
    // fleet is hidden; help is ungrouped and follows the groups.
    expect(linkLabels()).toEqual(["Chat", "Traces", "Help"]);
    expect(screen.getAllByRole("link").map((link) => link.getAttribute("href"))).toEqual(["/ui/chat", "/ui/traces", "/ui/help"]);

    const mounted = screen.getByTestId("mounted");
    expect(mounted.textContent).toBe("chat panel");
    expect(mounted.getAttribute("data-agents")).toBe("chatbot,rag0");
    expect(mounted.getAttribute("data-backend")).toBe("collector");

    act(() => {
      fireEvent.click(screen.getByText("Help"));
    });
    expect(window.location.pathname).toBe("/ui/help");
    expect(screen.getByTestId("mounted").textContent).toBe("help page");
    await waitFor(() => expect(screen.getByText("Help").closest("a")?.getAttribute("aria-current")).toBe("page"));
  });

  it("mounts a kit panel by its export with no registry entry", async () => {
    window.history.replaceState(null, "", "/ui/traces");
    render(<AppShell config={v2} client={client()} />);
    await waitFor(() => expect(screen.getAllByTestId("trace-list-row").length).toBeGreaterThan(0));
  });

  it("passes the ui.yaml panel config and prefers the application registry", () => {
    window.history.replaceState(null, "", "/ui/traces");
    render(<AppShell config={v2} registry={{ traces: stub("override") }} client={client()} />);
    expect(screen.getByTestId("mounted").getAttribute("data-config")).toBe('{"page_size":25}');
  });

  it("accepts whole kit panels in the registry", () => {
    window.history.replaceState(null, "", "/ui/chat");
    render(<AppShell config={v2} registry={{ ...kitPanelRegistry, chat: stub("chat panel") }} client={client()} />);
    expect(screen.getByTestId("mounted").textContent).toBe("chat panel");
  });

  it("shows a placeholder for a declared id with no component", () => {
    window.history.replaceState(null, "", "/ui/chat");
    render(<AppShell config={v2} client={client()} />);
    expect(screen.getByTestId("panel-placeholder").textContent).toContain('"chat"');
  });

  it("hides a kit panel whose monitored agents are all not deployed (R2.2)", async () => {
    const config: UIConfig = { ...v2, panels: v2.panels!.map((panel) => (panel.id === "fleet" ? { ...panel, hidden: false, sidebar_group: "observe" } : panel)) };
    window.history.replaceState(null, "", "/ui/chat");

    const partial = render(<AppShell config={config} registry={{ chat: stub("chat") }} client={client(["rag0"])} />);
    await waitFor(() => expect(linkLabels()).toContain("Fleet"));
    partial.unmount();

    render(<AppShell config={config} registry={{ chat: stub("chat") }} client={client(["chatbot", "rag0"])} />);
    expect(linkLabels()).toContain("Fleet");
    await waitFor(() => expect(linkLabels()).not.toContain("Fleet"));
    // Traces monitors no agent and stays.
    expect(linkLabels()).toEqual(["Chat", "Traces", "Help"]);
  });
});

describe("AppShell deep links (srd004 R5.3)", () => {
  const registry = { chat: stub("chat"), observability: stub("observability"), provisioning: stub("provisioning") };
  // The chatbot-mesh splitPanelPath cases, replayed through the shell.
  it.each([
    ["/ui/", "chat"],
    ["/ui/observability", "observability"],
    ["/ui/provisioning", "provisioning"],
    ["/ui/nonsense", "chat"],
    ["/", "chat"],
    ["/observability", "observability"],
  ])("%s mounts %s", (path, panel) => {
    window.history.replaceState(null, "", path);
    render(<AppShell config={chatbotMesh} registry={registry} client={client()} />);
    expect(screen.getByTestId("mounted").textContent).toBe(panel);
    expect(linkLabels()).toEqual(["Chat", "Observability", "Provisioning"]);
    expect(document.querySelector(".nav-item-active")?.textContent).toBe(panel[0].toUpperCase() + panel.slice(1));
  });

  it("navigates from an unknown segment by replacing it", () => {
    window.history.replaceState(null, "", "/ui/nonsense");
    render(<AppShell config={chatbotMesh} registry={registry} client={client()} />);
    expect(screen.getByRole("link", { name: "Provisioning" }).getAttribute("href")).toBe("/ui/provisioning");
  });
});

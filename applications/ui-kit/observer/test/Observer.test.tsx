import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { createKitClient, KitClientProvider } from "@declarative-agents/ui-kit";
import { fixtureFetch } from "@declarative-agents/ui-kit/fixtures";
import { Observer } from "../src/Observer";

class SilentEventSource {
  onerror = null;
  addEventListener() {}
  close() {}
}

function renderObserver(config: unknown) {
  const client = createKitClient({
    fetch: fixtureFetch({ overrides: { "/ui-config.json": config } }),
    EventSource: SilentEventSource as unknown as typeof EventSource,
  });
  return render(
    <KitClientProvider client={client}>
      <Observer />
    </KitClientProvider>,
  );
}

afterEach(cleanup);

describe("Observer", () => {
  it("composes status, topology, agent cards, and machines from the declared config", async () => {
    renderObserver({ title: "Mesh observer", monitored_agents: [{ name: "chatbot", label: "Chatbot" }] });
    await waitFor(() => expect(screen.getByRole("heading", { level: 1 }).textContent).toBe("Mesh observer"));
    await waitFor(() => expect(screen.getByText("1/2 reachable")).toBeTruthy());
    expect(screen.getByTestId("topology")).toBeTruthy();
    expect(document.querySelector(".dak-agent-cards")).toBeTruthy();
    await waitFor(() => expect(screen.getByTestId("machine-panel")).toBeTruthy());
  });

  it("falls back to a plain title and no machine section without a config", async () => {
    renderObserver({});
    await waitFor(() => expect(screen.getByText("1/2 reachable")).toBeTruthy());
    expect(screen.getByRole("heading", { level: 1 }).textContent).toBe("Observer");
    expect(screen.queryByTestId("machine-panel")).toBeNull();
  });
});

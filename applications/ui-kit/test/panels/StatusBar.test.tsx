// @vitest-environment jsdom
import { render, screen, waitFor } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { EMPTY_FLEET } from "../../src/hooks/useFleet";
import { StatusBar, StatusBarPanel } from "../../src/panels/StatusBar";
import { FixtureProvider } from "./support";

describe("StatusBar", () => {
  it("renders each poll status", () => {
    const { rerender, container } = render(<StatusBar snapshot={{ status: "connecting", data: EMPTY_FLEET, observerState: "" }} />);
    expect(screen.getByText("Connecting...")).toBeTruthy();
    rerender(<StatusBar snapshot={{ status: "error", error: "HTTP 503", data: EMPTY_FLEET, observerState: "" }} />);
    expect(screen.getByText("Error: HTTP 503")).toBeTruthy();
    expect(container.querySelector(".dot-err")).toBeTruthy();
  });

  it("mounts against the fleet fixture", async () => {
    render(
      <FixtureProvider>
        <StatusBarPanel />
      </FixtureProvider>,
    );
    await waitFor(() => expect(screen.getByText("Connected · observer AwaitingRequest")).toBeTruthy());
    expect(screen.getByText("1/2 reachable")).toBeTruthy();
  });
});

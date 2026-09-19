// @vitest-environment jsdom
import { act, cleanup, fireEvent, render, renderHook, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it } from "vitest";
import type { PanelRouting } from "../src/shell/paths";
import { canonicalPanelPath, navigateTo, PanelLink, ShellRoutingProvider, usePanelPath } from "../src/shell/subPath";

const routing: PanelRouting = {
  defaultPanel: "traces",
  routes: [
    { id: "traces", path: "/traces", label: "Traces" },
    { id: "explore", path: "/explore", label: "Explore" },
  ],
};
const wrapper = ({ children }: { children: ReactNode }) => <ShellRoutingProvider routing={routing}>{children}</ShellRoutingProvider>;

afterEach(() => {
  cleanup();
  window.history.replaceState(null, "", "/");
});

describe("sub-path routing (srd004 R5.3)", () => {
  it("folds a sidebar link taken from a sub-path back to its panel", () => {
    expect(canonicalPanelPath("/traces/abc", routing)).toBe("/traces/abc");
    expect(canonicalPanelPath("/traces/explore", routing)).toBe("/explore");
    expect(canonicalPanelPath("/explore", routing)).toBe("/explore");
  });

  it("re-reads the location on navigation and canonicalizes it", () => {
    window.history.replaceState(null, "", "/traces");
    const { result } = renderHook(() => usePanelPath(), { wrapper });
    expect(result.current).toBe("/traces");
    act(() => navigateTo("/traces/t-1"));
    expect(result.current).toBe("/traces/t-1");
    act(() => navigateTo("/traces/explore"));
    expect(result.current).toBe("/explore");
    expect(window.location.pathname).toBe("/explore");
  });

  it("routes plain clicks in-app and leaves modified clicks to the browser", () => {
    window.history.replaceState(null, "", "/traces");
    render(<PanelLink to="/traces/t-2">open</PanelLink>, { wrapper });
    fireEvent.click(screen.getByText("open"));
    expect(window.location.pathname).toBe("/traces/t-2");
    const stop = (event: Event) => event.preventDefault();
    document.addEventListener("click", stop);
    fireEvent.click(screen.getByText("open"), { ctrlKey: true });
    document.removeEventListener("click", stop);
    expect(screen.getByText("open").getAttribute("href")).toBe("/traces/t-2");
  });
});

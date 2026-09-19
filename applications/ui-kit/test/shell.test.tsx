// @vitest-environment jsdom
import { act, fireEvent, render, renderHook, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { PanelFrame } from "../src/shell/PanelFrame";
import { Sidebar } from "../src/shell/Sidebar";
import type { PanelRouting } from "../src/shell/paths";
import { usePanelLocation } from "../src/shell/usePanelLocation";

const routing: PanelRouting = {
  defaultPanel: "chat",
  routes: [
    { id: "chat", path: "/chat", label: "Chat", group: "chat" },
    { id: "observability", path: "/observability", label: "Traces", group: "observability" },
    { id: "hidden", path: "/hidden", label: "Hidden", hidden: true },
  ],
};

afterEach(() => window.history.replaceState(null, "", "/"));

describe("Sidebar and PanelFrame", () => {
  it("renders visible entries with real hrefs and marks the active one", () => {
    render(
      <PanelFrame
        sidebar={<Sidebar title="Chatbot" routes={routing.routes} active="observability" href={(id) => `/ui/${id}`} onNavigate={() => undefined} />}
      >
        <p>panel body</p>
      </PanelFrame>,
    );
    const links = screen.getAllByRole("link");
    expect(links.map((link) => link.getAttribute("href"))).toEqual(["/ui/chat", "/ui/observability"]);
    expect(screen.getByText("Traces").closest("a")?.className).toBe("nav-item nav-item-active");
    expect(screen.getByText("panel body").closest("main")?.className).toBe("content");
  });

  it("orders groups and renders their labels", () => {
    render(
      <Sidebar
        title="Chatbot"
        routes={routing.routes}
        active="chat"
        href={(id) => id}
        onNavigate={() => undefined}
        groups={[
          { id: "observability", label: "Observe", order: 1 },
          { id: "chat", label: "Talk", order: 0 },
        ]}
      />,
    );
    expect(Array.from(document.querySelectorAll(".nav-group-label")).map((node) => node.textContent)).toEqual(["Talk", "Observe"]);
  });
});

describe("usePanelLocation", () => {
  it("navigates by pushing history and follows back and forward", () => {
    window.history.replaceState(null, "", "/ui/chat");
    const { result } = renderHook(() => usePanelLocation(routing));
    expect(result.current.active).toBe("chat");
    expect(result.current.href("observability")).toBe("/ui/observability");

    render(<a href={result.current.href("observability")} onClick={(event) => result.current.navigate(event, "observability")}>go</a>);
    act(() => {
      fireEvent.click(screen.getByText("go"));
    });
    expect(window.location.pathname).toBe("/ui/observability");
    expect(result.current.active).toBe("observability");

    act(() => {
      window.history.replaceState(null, "", "/ui/chat");
      window.dispatchEvent(new PopStateEvent("popstate"));
    });
    expect(result.current.active).toBe("chat");
  });

  it("leaves modified clicks to the browser", () => {
    window.history.replaceState(null, "", "/ui/chat");
    const { result } = renderHook(() => usePanelLocation(routing));
    render(<a href="/ui/observability" onClick={(event) => result.current.navigate(event, "observability")}>tab</a>);
    // jsdom cannot follow the link the browser would open; stop it after the
    // handler has declined the click.
    const stop = (event: Event) => event.preventDefault();
    document.addEventListener("click", stop);
    fireEvent.click(screen.getByText("tab"), { metaKey: true });
    document.removeEventListener("click", stop);
    expect(window.location.pathname).toBe("/ui/chat");
  });
});

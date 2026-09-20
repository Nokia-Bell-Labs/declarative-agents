// @vitest-environment jsdom
import { act, cleanup, fireEvent, render, renderHook, screen, within } from "@testing-library/react";
import { readFileSync } from "node:fs";
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
    expect(screen.getByText("panel body").closest("main")?.parentElement?.className).toBe("dak-shell shell");
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

// A variable, not a literal: Vite rewrites new URL("literal", import.meta.url)
// to a served asset path.
const SHELL_CSS = "../src/shell/shell.css";

describe("shell styles (GH-2282)", () => {
  const css = readFileSync(new URL(SHELL_CSS, import.meta.url).pathname, "utf8").replace(/\/\*[\s\S]*?\*\//g, "");
  const selectors = Array.from(css.matchAll(/([^{}]+)\{[^}]*\}/g)).flatMap((rule) => rule[1].split(",").map((selector) => selector.trim()));

  it("scopes every rule under .dak-shell and styles the sidebar, nav, and content", () => {
    expect(selectors.filter((selector) => !selector.startsWith(".dak-shell"))).toEqual([]);
    for (const name of ["sidebar", "sidebar-title", "nav-item", "nav-item:hover", "nav-item-active", "nav-group", "nav-group-label", "content", "dak-shell-placeholder"]) {
      expect(selectors).toContain(`.dak-shell .${name}`);
    }
  });

  it("colors only through the kit tokens", () => {
    expect(css).not.toMatch(/#[0-9a-fA-F]{3,8}\b|rgba?\(/);
  });
});

// GH-2292: a sidebar carries content that is not a panel — a chat history
// under one entry, an action that opens an overlay rather than routing, a link
// to another origin. None can be declared in panels[], so the application with
// the most sidebar content was the one not mounting AppShell at all.
//
// This block cleans up after each case. The suite above queries the whole
// document and has no auto-cleanup, so a render that outlived its test would
// be counted by the next one.
describe("Sidebar slots", () => {
  afterEach(cleanup);

  it("renders sidebarExtra under the entry it names and sidebarFooter after all of them", () => {
    const { container } = render(
      <Sidebar
        title="Chatbot"
        routes={routing.routes}
        active="chat"
        href={(id) => `/ui/${id}`}
        onNavigate={() => undefined}
        sidebarExtra={(route) => (route.id === "chat" ? <span data-testid="history">history for {route.label}</span> : null)}
        sidebarFooter={<a href="https://observer.example">Fleet observer</a>}
      />,
    );
    const view = within(container);
    // The extra renders for the entry it names and for no other, and a hidden
    // route gets none at all.
    expect(view.getAllByTestId("history")).toHaveLength(1);
    expect(view.getByTestId("history").textContent).toBe("history for Chat");
    // The footer comes after every panel entry, so a link to another origin
    // cannot land between two panels.
    expect(view.getAllByRole("link").map((link) => link.getAttribute("href"))).toEqual([
      "/ui/chat",
      "/ui/observability",
      "https://observer.example",
    ]);
  });

  it("keeps the extra inside its entry's group when the routes are grouped", () => {
    const { container } = render(
      <Sidebar
        title="Chatbot"
        routes={routing.routes}
        active="chat"
        href={(id) => `/ui/${id}`}
        onNavigate={() => undefined}
        groups={[
          { id: "chat", label: "Talk", order: 1 },
          { id: "observability", label: "Observe", order: 2 },
        ]}
        sidebarExtra={(route) => <span data-testid={`extra-${route.id}`} />}
        sidebarFooter={<span data-testid="footer" />}
      />,
    );
    // Grouped rendering is a separate branch, so it needs its own evidence
    // that the extra follows its entry into the group rather than being lost.
    const group = within(container).getByText("Talk").parentElement;
    expect(group?.querySelector('[data-testid="extra-chat"]')).not.toBeNull();
    expect(group?.querySelector('[data-testid="extra-observability"]')).toBeNull();
    expect(within(container).getByTestId("footer")).not.toBeNull();
  });

  it("renders the entries alone when neither slot is given", () => {
    const { container } = render(
      <Sidebar title="Chatbot" routes={routing.routes} active="chat" href={(id) => `/ui/${id}`} onNavigate={() => undefined} />,
    );
    expect(within(container).getAllByRole("link").map((link) => link.getAttribute("href"))).toEqual([
      "/ui/chat",
      "/ui/observability",
    ]);
  });
});

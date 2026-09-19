import { describe, expect, it } from "vitest";
import { panelForPath, panelHref, splitPanelPath, staticHref, type PanelRouting } from "../src/shell/paths";

// The route tables of the three chatbot copies the kit replaces.
const chatbotMesh: PanelRouting = {
  defaultPanel: "chat",
  routes: [
    { id: "chat", path: "/chat", label: "Chat" },
    { id: "observability", path: "/observability", label: "Observability" },
    { id: "provisioning", path: "/provisioning", label: "Provisioning" },
  ],
};
const cohere: PanelRouting = {
  defaultPanel: "chat",
  routes: [
    { id: "chat", path: "/chat", label: "Chat" },
    { id: "observability", path: "/observability", label: "Traces", hidden: true },
    { id: "corpus", path: "/corpus", label: "Corpus" },
    { id: "provisioning", path: "/provisioning", label: "Provisioning" },
  ],
};
const wiki: PanelRouting = {
  defaultPanel: "chat",
  routes: ["chat", "corpus", "observability", "fleet", "provisioning", "program"].map((id) => ({ id, path: `/${id}`, label: id })),
};

describe("splitPanelPath (srd004 R5.3)", () => {
  const cases: Array<[string, PanelRouting, string, { base: string; panel: string }]> = [
    ["mount root with trailing slash", wiki, "/ui/", { base: "/ui/", panel: "chat" }],
    ["panel under a mount", wiki, "/ui/corpus", { base: "/ui/", panel: "corpus" }],
    ["panel at the dev-server root", wiki, "/corpus", { base: "/", panel: "corpus" }],
    ["unknown segment falls back", wiki, "/ui/nonsense", { base: "/ui/", panel: "chat" }],
    ["chatbot-mesh observability", chatbotMesh, "/ui/observability", { base: "/ui/", panel: "observability" }],
    ["chatbot-mesh bare root", chatbotMesh, "/", { base: "/", panel: "chat" }],
    ["a hidden route still resolves by URL", cohere, "/ui/observability", { base: "/ui/", panel: "observability" }],
    ["a nested mount keeps its base", cohere, "/a/b/ui/corpus", { base: "/a/b/ui/", panel: "corpus" }],
  ];
  it.each(cases)("%s", (_name, routing, path, want) => {
    expect(splitPanelPath(path, routing)).toEqual(want);
  });

  it("replaces an unknown segment instead of nesting under it", () => {
    expect(panelForPath("/ui/nonsense", wiki)).toBe("chat");
    expect(panelHref("/ui/nonsense", wiki, "corpus")).toBe("/ui/corpus");
    expect(panelHref("/ui/", chatbotMesh, "provisioning")).toBe("/ui/provisioning");
    expect(panelHref("/ui/chat", chatbotMesh, "missing")).toBe("/ui/");
  });

  it("resolves static files under the base", () => {
    expect(staticHref("/ui/chat", wiki, "corpus.json")).toBe("/ui/corpus.json");
    expect(staticHref("/", wiki, "corpus.json")).toBe("/corpus.json");
  });
});

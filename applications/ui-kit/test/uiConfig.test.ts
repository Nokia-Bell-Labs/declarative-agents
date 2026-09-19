import { readdirSync, readFileSync, statSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { parse } from "yaml";
import { routingFromConfig, shellTitle, validateUIConfig, type UIConfig } from "../src/shell/uiConfig";

// The Go validator's fixture and the chatbot-mesh ui.yaml are the shared
// inputs: the TypeScript rules must agree with magefiles/uiyaml on both.
const APPLICATIONS = new URL("../..", import.meta.url).pathname;
const V2_FIXTURE = new URL("../../../magefiles/uiyaml/testdata/v2.yaml", import.meta.url).pathname;
const CHATBOT_MESH_UI_YAML = join(APPLICATIONS, "chatbot-mesh/agents/chatbot/ui/ui.yaml");

const load = (path: string) => parse(readFileSync(path, "utf8")) as UIConfig;

function repositoryUIYAMLs(dir: string): string[] {
  return readdirSync(dir).flatMap((name) => {
    const path = join(dir, name);
    if (statSync(path).isDirectory()) return name === "node_modules" || name === "dist" ? [] : repositoryUIYAMLs(path);
    return name === "ui.yaml" ? [path] : [];
  });
}

describe("validateUIConfig (srd004 R7)", () => {
  it("accepts the version 2 fixture and every repository ui.yaml unchanged", () => {
    const files = repositoryUIYAMLs(APPLICATIONS);
    expect(files.length).toBeGreaterThan(0);
    for (const path of [V2_FIXTURE, ...files]) expect([path, validateUIConfig(load(path))]).toEqual([path, []]);
  });

  // The cases of TestValidateRejections in magefiles/uiyaml/uiyaml_test.go.
  const base = readFileSync(V2_FIXTURE, "utf8");
  const rejections: Array<[string, string, string, string]> = [
    ["duplicate panel id", "  - id: fleet\n", "  - id: traces\n", 'duplicate id "traces"'],
    ["panel id reuses a route id", "  - id: fleet\n", "  - id: help\n", 'duplicate id "help"'],
    ["route collision", "    route: /fleet\n", "    route: /traces\n", "route /traces collides"],
    ["panel route collides with a route path", "    route: /fleet\n", "    route: /help\n", "route /help collides"],
    ["undeclared sidebar group", "    sidebar_group: talk\n", "    sidebar_group: nowhere\n", 'sidebar_group "nowhere" is not declared'],
    ["panels without version 2", "version: 2\n", "", "panels require version: 2"],
    ["unsupported version", "version: 2\n", "version: 3\n", "version 3 is not supported"],
    ["nested route", "    route: /chat\n", "    route: /a/chat\n", "must be one lower-case segment"],
    ["kit panel without export", "    export: fleet\n", "", "must name the kit panel in export"],
    ["monitored agent listed twice", "  - name: rag0\n", "  - name: chatbot\n", 'monitored agent "chatbot" is listed twice'],
    ["trace backend without a name", "  name: collector\n", '  name: ""\n', "trace_backend has no name"],
  ];
  it.each(rejections)("rejects %s", (_name, from, to, want) => {
    expect(base).toContain(from);
    const problems = validateUIConfig(parse(base.replace(from, to)));
    expect(problems.join("\n")).toContain(want);
  });

  // The structural cases TestJSONSchemaAgreesWithTheGoType expects the schema
  // to reject, plus type errors the schema implies.
  it.each([
    ["unknown branding key", '  accent: "#005aff"\n', '  accent: "#005aff"\n  font: x\n', 'branding has unknown key "font"'],
    ["panel without package", "    package: local\n", "", 'panel "chat" has no package'],
    ["hidden that is not a boolean", "    hidden: true\n", "    hidden: yes-please\n", "panels[2].hidden must be a boolean"],
    ["monitored agent without label", "    label: RAG server 0\n", "", "monitored_agents[1] has no label"],
  ])("rejects %s", (_name, from, to, want) => {
    expect(base).toContain(from);
    expect(validateUIConfig(parse(base.replace(from, to))).join("\n")).toContain(want);
  });

  it("reports every problem at once, sorted, and rejects a non-mapping", () => {
    const problems = validateUIConfig({ version: 2, panels: [{ id: "a", route: "/A" }] });
    expect(problems).toEqual([...problems].sort());
    expect(problems).toEqual(expect.arrayContaining(["id is required", 'panel "a" has no package', 'panel "a" path "/A" must be one lower-case segment such as /traces']));
    expect(validateUIConfig(["id: x"])).toEqual(["ui.yaml must be a mapping"]);
  });
});

describe("routingFromConfig (srd004 R5.2)", () => {
  it("groups the chatbot-mesh version 1 routes by their own id", () => {
    const routing = routingFromConfig(load(CHATBOT_MESH_UI_YAML));
    expect(routing.defaultPanel).toBe("chat");
    expect(routing.routes).toEqual([
      { id: "chat", path: "/chat", label: "Chat", group: "chat" },
      { id: "observability", path: "/observability", label: "Observability", group: "observability" },
      { id: "provisioning", path: "/provisioning", label: "Provisioning", group: "provisioning" },
    ]);
    expect(routing.groups).toEqual([
      { id: "chat", label: "Chat", order: 0 },
      { id: "observability", label: "Observability", order: 1 },
      { id: "provisioning", label: "Provisioning", order: 2 },
    ]);
    expect(shellTitle(load(CHATBOT_MESH_UI_YAML))).toBe("Chatbot");
  });

  it("lists version 2 routes and panels and defaults to the first sidebar entry", () => {
    const config = load(V2_FIXTURE);
    const routing = routingFromConfig(config);
    expect(routing.routes).toEqual([
      { id: "help", path: "/help", label: "Help" },
      { id: "chat", path: "/chat", label: "Chat", group: "talk" },
      { id: "traces", path: "/traces", label: "Traces", group: "observe" },
      // A kit panel without a label takes its manifest title.
      { id: "fleet", path: "/fleet", label: "Fleet", hidden: true },
    ]);
    // help is declared first but renders ungrouped, after the talk group.
    expect(routing.defaultPanel).toBe("chat");
    expect(shellTitle(config)).toBe("Demo");
  });

  it("falls back to the first route when there are no groups", () => {
    const routing = routingFromConfig({ id: "x", routes: [{ id: "a", path: "/a" }, { id: "b", path: "/b" }] });
    expect(routing).toEqual({ groups: [], defaultPanel: "a", routes: [{ id: "a", path: "/a", label: "a" }, { id: "b", path: "/b", label: "b" }] });
  });
});

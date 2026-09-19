import { mkdirSync, mkdtempSync, readFileSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import type { Plugin, ResolvedConfig } from "vite";
import { describe, expect, it } from "vitest";
import { kitPanelManifests } from "../src/panels/manifests";
import { kitPanelRegistry } from "../src/panels/registry";
import { compositionProblems, UI_CONFIG_MODULE, uiYaml, type UIYamlPluginOptions } from "../src/shell/vite-plugin-ui-yaml";

const FIXTURE = readFileSync(new URL("../../../magefiles/uiyaml/testdata/v2.yaml", import.meta.url).pathname, "utf8");

// app lays out the chatbot-mesh shape: ui.yaml beside the Vite root app/.
function app(uiYamlText: string): string {
  const dir = mkdtempSync(join(tmpdir(), "ui-yaml-"));
  mkdirSync(join(dir, "app"));
  writeFileSync(join(dir, "ui.yaml"), uiYamlText);
  return join(dir, "app");
}

type Hook = (this: unknown, ...args: unknown[]) => unknown;
const hook = (plugin: Plugin, name: keyof Plugin) => plugin[name] as unknown as Hook;

// build drives the plugin hooks in the order Vite calls them and returns the
// virtual module source.
function build(root: string, options: UIYamlPluginOptions = {}): string {
  const plugin = uiYaml(options);
  const watched: string[] = [];
  hook(plugin, "configResolved").call(undefined, { root } as ResolvedConfig);
  hook(plugin, "buildStart").call({ addWatchFile: (file: string) => watched.push(file) });
  expect(watched).toEqual([join(root, "../ui.yaml")]);
  const resolved = hook(plugin, "resolveId").call(undefined, UI_CONFIG_MODULE) as string;
  expect(hook(plugin, "resolveId").call(undefined, "react")).toBeUndefined();
  return hook(plugin, "load").call(undefined, resolved) as string;
}

describe("ui.yaml Vite plugin (srd004 R5.1, R5.2)", () => {
  it("serves a valid ui.yaml as virtual:ui-config", () => {
    const source = build(app(FIXTURE), { registryIds: ["chat"] });
    expect(source.startsWith("export default ")).toBe(true);
    const config = JSON.parse(source.slice("export default ".length).replace(/;\n$/, ""));
    expect(config.id).toBe("demo-ui");
    expect(config.panels.map((panel: { id: string }) => panel.id)).toEqual(["chat", "traces", "fleet"]);
    expect(config.panels[1].config).toEqual({ page_size: 25 });
  });

  it("reads a path relative to the Vite root", () => {
    const root = app("id: other\n");
    writeFileSync(join(root, "custom.yaml"), FIXTURE);
    const plugin = uiYaml({ path: "custom.yaml" });
    hook(plugin, "configResolved").call(undefined, { root } as ResolvedConfig);
    expect(hook(plugin, "load").call(undefined, `\0${UI_CONFIG_MODULE}`)).toContain('"demo-ui"');
  });

  it("fails the build with every violation of an invalid ui.yaml", () => {
    const invalid = FIXTURE.replace("version: 2\n", "").replace("    sidebar_group: talk\n", "    sidebar_group: nowhere\n");
    expect(() => build(app(invalid))).toThrow(/invalid ui\.yaml .*ui\.yaml:\n {2}panel "chat" sidebar_group "nowhere" is not declared under sidebar\.groups\n {2}panels require version: 2/);
  });

  it("fails the build for a panel with no registry entry", () => {
    expect(() => build(app(FIXTURE), { registryIds: [] })).toThrow('panel "chat" from local has no registry entry');
    // Without registryIds the check is left to the runtime placeholder.
    expect(() => build(app(FIXTURE))).not.toThrow();
  });

  it("fails the build for a kit export that is not a kit panel", () => {
    expect(() => build(app(FIXTURE.replace("    export: trace\n", "    export: traces\n")), { registryIds: ["chat"] })).toThrow(
      'panel "traces" exports "traces", which is not a kit panel',
    );
  });

  it("fails the build for unparsable YAML", () => {
    expect(() => build(app("id: [unclosed\n"))).toThrow(/^ui\.yaml .*ui\.yaml: /);
  });

  it("checks kit exports against the same ids the shell mounts", () => {
    expect(kitPanelManifests.map((manifest) => manifest.id).sort()).toEqual(Object.keys(kitPanelRegistry).sort());
    expect(compositionProblems({ id: "x", version: 2, panels: [{ id: "a", package: "@declarative-agents/ui-kit", export: "machine-view", route: "/a" }] }, [])).toEqual([]);
  });
});

import { readdirSync, readFileSync, statSync } from "node:fs";
import { join, relative } from "node:path";
import { describe, expect, it } from "vitest";
import { CONTRACT_ENDPOINTS } from "../../src/contract";
import { definePanel } from "../../src/panels/manifest";
import { kitPanelRegistry, kitPanels } from "../../src/panels/registry";

// srd004 R4.4: the complete kit panel set once GH-2157 lands.
const R4_4_PANELS = ["status-bar", "fleet", "trace", "machine-view", "topology", "agent-card"];

const PANELS = new URL("../../src/panels", import.meta.url).pathname;
function sources(dir: string): string[] {
  return readdirSync(dir).flatMap((name) => {
    const path = join(dir, name);
    return statSync(path).isDirectory() ? sources(path) : /\.(ts|tsx)$/.test(name) ? [path] : [];
  });
}

describe("kit panels (srd004 R4)", () => {
  it("export manifests with every field and only contract endpoints", () => {
    for (const { manifest } of kitPanels) {
      expect(manifest.id).toMatch(/^[a-z][a-z-]*$/);
      expect(manifest.title).not.toBe("");
      expect(manifest.route).toMatch(/^\/[a-z-]+$/);
      expect(manifest.monitored_agents === "declared" || Array.isArray(manifest.monitored_agents)).toBe(true);
      for (const endpoint of manifest.required_endpoints) expect(CONTRACT_ENDPOINTS).toContain(endpoint);
    }
  });

  it("key the registry by unique manifest id within the R4.4 set", () => {
    expect(Object.keys(kitPanelRegistry)).toHaveLength(kitPanels.length);
    for (const id of Object.keys(kitPanelRegistry)) expect(R4_4_PANELS).toContain(id);
  });

  it("reject a manifest outside the contract", () => {
    expect(() =>
      definePanel({ id: "x", title: "X", route: "/x", required_endpoints: ["/api/v1/documents" as never], monitored_agents: [] }, () => null),
    ).toThrow("outside the presentation contract");
  });

  it("import nothing from an application directory", () => {
    const offenders = sources(PANELS).filter((path) => /from\s+["'](\.\.\/){3,}|applications\//.test(readFileSync(path, "utf8")));
    expect(offenders.map((path) => relative(PANELS, path))).toEqual([]);
  });
});

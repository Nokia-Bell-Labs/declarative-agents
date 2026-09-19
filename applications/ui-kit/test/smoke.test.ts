import { readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";
import { KIT_VERSION } from "../src/index";

describe("ui-kit package", () => {
  it("exports the version declared in package.json", () => {
    const pkg = JSON.parse(readFileSync(new URL("../package.json", import.meta.url), "utf8"));
    expect(KIT_VERSION).toBe(pkg.version);
  });
});

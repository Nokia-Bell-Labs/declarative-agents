import { readdirSync, readFileSync, statSync } from "node:fs";
import { join, relative } from "node:path";
import { describe, expect, it } from "vitest";
import { CONTRACT_ENDPOINTS, CONTRACT_VERSION } from "../src/contract";
import { fixtures } from "../src/fixtures";

const SRC = new URL("../src", import.meta.url).pathname;

function sources(dir: string): string[] {
  return readdirSync(dir).flatMap((name) => {
    const path = join(dir, name);
    if (statSync(path).isDirectory()) return sources(path);
    return /\.(ts|tsx)$/.test(name) ? [path] : [];
  });
}

describe("presentation contract", () => {
  it("exports major version 1 and the closed endpoint list (srd004 R1.1, R3.1)", () => {
    expect(CONTRACT_VERSION).toBe(1);
    expect(new Set(CONTRACT_ENDPOINTS).size).toBe(CONTRACT_ENDPOINTS.length);
  });

  it("records a fixture for every contract endpoint (srd004 R3.2)", () => {
    const recorded = new Set(Object.keys(fixtures));
    for (const endpoint of Object.keys(fixtures)) expect(CONTRACT_ENDPOINTS).toContain(endpoint);
    // /monitor/fleet is recorded with the fleet client (GH-2262).
    expect(CONTRACT_ENDPOINTS.filter((endpoint) => !recorded.has(endpoint))).toEqual(["/monitor/fleet"]);
  });

  it("performs I/O only in the client module (srd004 R4.3, R6.1)", () => {
    const offenders = sources(SRC)
      .filter((path) => !relative(SRC, path).startsWith("client/"))
      .filter((path) => /\bfetch\s*\(|new\s+EventSource\b/.test(readFileSync(path, "utf8")));
    expect(offenders.map((path) => relative(SRC, path))).toEqual([]);
  });
});

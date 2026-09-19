import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { createRequire } from "node:module";
import path from "node:path";
import test from "node:test";
import { fileURLToPath } from "node:url";

test("loads the canonical kit token source without local declarations", async () => {
  const testDir = path.dirname(fileURLToPath(import.meta.url));
  const appCSS = await readFile(path.join(testDir, "../src/App.css"), "utf8");
  const match = appCSS.match(/^@import\s+"([^"]+)";/);
  assert.ok(match, "App.css starts with a canonical token import");
  assert.equal((appCSS.match(/--[a-z-]+\s*:/g) ?? []).length, 0, "App.css declares no copied tokens");

  assert.equal(match[1], "@declarative-agents/ui-kit/tokens.css");

  const resolved = createRequire(import.meta.url).resolve(match[1]);
  const canonical = await readFile(resolved, "utf8");
  assert.match(canonical, /:root\s*\{/);
  assert.match(canonical, /--bg-primary:\s*#ffffff/);
  assert.match(canonical, /--accent:\s*#005aff/);
});

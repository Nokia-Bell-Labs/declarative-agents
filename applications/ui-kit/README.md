<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

# @declarative-agents/ui-kit

The shared presentation package for declarative-agents user interfaces. It owns the design tokens, the injectable HTTP and SSE client, the presentation-contract clients and their recorded fixtures, the generic application shell, shared hooks, and the observability panels. The contract it binds to is `applications/docs/specs/software-requirements/srd004-declarative-ux.yaml`; the design guide is `docs/guides/ui-panels.md`.

The kit is a library: React 19 is a peer dependency, and the built `dist/` is not committed. Applications compose kit panels with their own domain panels at build time.

## Consuming the kit

Inside this repository a UI depends on the kit by path, for example `"@declarative-agents/ui-kit": "file:../../../../ui-kit"`. Build the kit first with `mage uikit:build`; the `mage uidist` gate stages and builds it automatically for every UI that declares the dependency. A UI that mounts kit components adds `resolve: { dedupe: ["react", "react-dom"] }` to its Vite config so the linked kit and the application share one React instance.

Every dependent UI's `package-lock.json` records the kit's `package.json` fields, so a change to the kit's version or dependencies is followed by `npm install` in each dependent UI and a commit of the refreshed lockfiles; otherwise `npm ci` in the `mage uidist` gate rejects the stale lock. The design tokens are the file `src/tokens.css`, exported as `@declarative-agents/ui-kit/tokens.css`; a UI imports them on the first line of its `App.css`.

Repositories outside this one depend on the release tarball (srd004 R8.3):

```json
"@declarative-agents/ui-kit": "https://github.com/Nokia-Bell-Labs/declarative-agents/releases/download/ui-kit%2Fv0.1.0/declarative-agents-ui-kit-0.1.0.tgz"
```

## Panels

A panel is a component plus a manifest (srd004 R4). Each lives in `src/panels/<Name>/` as `<Name>.tsx`, its scoped stylesheet, `manifest.ts`, and `index.ts`, which exports the view component, the mountable component, and `definePanel(manifest, mount)`. `definePanel` rejects a manifest that names an endpoint outside the presentation contract. A mountable component receives `PanelProps`: the ui.yaml panel `config`, the declared `monitoredAgents`, and the declared `traceBackend`. `kitPanelRegistry` keys every published panel by manifest id.

Panel styles sit under a root class (`dak-<panel>`), use only token variables, and build into `dist/ui-kit.css`. An application imports them once with `import "@declarative-agents/ui-kit/styles.css";`. Tests render each panel against the recorded fixtures through `test/panels/support.tsx`.

## Shell

An application built on the kit shell holds no routing code: its `ui.yaml` declares the sidebar and routes, and its entrypoint supplies a registry of domain panels (srd004 R5). The Vite plugin `@declarative-agents/ui-kit/vite` reads `ui.yaml` during the build (default path `../ui.yaml` relative to the Vite root, the chatbot-mesh layout), fails the build with every violation the Go validator `magefiles/uiyaml` would report, and serves the parsed file as the virtual module `virtual:ui-config`. Given `registryIds`, it also fails the build for a panel outside `@declarative-agents/ui-kit` with no registry entry; a kit panel's `export` must name a kit panel manifest id.

```ts
// vite.config.ts
import react from "@vitejs/plugin-react";
import uiYaml from "@declarative-agents/ui-kit/vite";
import { defineConfig } from "vite";

export default defineConfig({
  base: "./",
  plugins: [react(), uiYaml({ registryIds: ["chat", "provisioning"] })],
  resolve: { dedupe: ["react", "react-dom"] },
});
```

```tsx
// src/main.tsx; src/vite-env.d.ts adds /// <reference types="@declarative-agents/ui-kit/ui-config" />
import { createRoot } from "react-dom/client";
import config from "virtual:ui-config";
import { AppShell } from "@declarative-agents/ui-kit";
import "@declarative-agents/ui-kit/styles.css";
import { ChatPanel, ProvisioningPanel } from "./panels";

createRoot(document.getElementById("root")!).render(<AppShell config={config} registry={{ chat: ChatPanel, provisioning: ProvisioningPanel }} />);
```

`AppShell` mounts the active panel from the registry by panel id, or, for a `@declarative-agents/ui-kit` panel with no registry entry, the kit panel its `export` names. A registry value is a component taking `PanelProps` or a whole kit panel, so `{...kitPanelRegistry, ...domainPanels}` works. Version 1 `routes[]` mount from the registry by route id and group under the sidebar group of the same id. A declared id with no component renders a placeholder rather than failing. The sidebar hides a kit panel whose manifest monitors agents that the monitor proxy reports all not deployed (srd004 R2.2); the shell probes each agent's `monitor/state` once on mount and re-probes absent agents every 60 seconds. The optional `client` prop points every panel at another base URL; the default is same-origin. `examples/minimal/` is a complete application of two kit panels composed only from its `ui.yaml`; `npm test` builds it.

## Mage targets

Table: ui-kit Mage targets

| Target | Effect |
|---|---|
| `mage uikit:build` | `npm ci`, then the Vite library build and declaration emit into `dist/` |
| `mage uikit:test` | `npm ci`, type-check, and Vitest; `mage test` runs it too |
| `mage uikit:pack` | build, then `npm pack` into `out/` |
| `mage uikit:release` | pack from a clean tree, check the `ui-kit/vX.Y.Z` tag against `package.json`, and print the tag and `gh release create` commands; it publishes nothing |

A release bumps `version` in `package.json` and `KIT_VERSION` in `src/index.ts` together; a kit test fails when they differ.

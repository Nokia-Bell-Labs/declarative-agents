/// <reference types="node" />
import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import type { Plugin } from "vite";
import { parse } from "yaml";
import { kitPanelManifestById } from "../panels/manifests";
import { KIT_PACKAGE, validateUIConfig, type UIConfig } from "./uiConfig";

// The build-time half of the kit shell (srd004 R5.1, R5.2): the plugin reads
// the application's ui.yaml during the Vite build, fails the build on any
// violation, and serves the parsed file as the virtual module
// virtual:ui-config. Runtime module loading is not supported.

export const UI_CONFIG_MODULE = "virtual:ui-config";
const RESOLVED_UI_CONFIG = `\0${UI_CONFIG_MODULE}`;

export interface UIYamlPluginOptions {
  // ui.yaml location relative to the Vite root; the default is the
  // chatbot-mesh layout, where the app sits one directory below its ui.yaml.
  path?: string;
  // The application registry's panel ids. When given, a non-kit panel id
  // missing from it fails the build (srd004 R5.2, R7.4).
  registryIds?: string[];
}

// compositionProblems checks what the file alone cannot: every kit panel names
// a published kit panel, and every other panel has a registry entry.
export function compositionProblems(config: UIConfig, registryIds?: string[]): string[] {
  const problems: string[] = [];
  const registered = registryIds ? new Set(registryIds) : undefined;
  for (const panel of config.panels ?? []) {
    if (panel.package === KIT_PACKAGE) {
      if (panel.export && !Object.prototype.hasOwnProperty.call(kitPanelManifestById, panel.export)) {
        problems.push(`panel ${JSON.stringify(panel.id)} exports ${JSON.stringify(panel.export)}, which is not a kit panel (${Object.keys(kitPanelManifestById).join(", ")})`);
      }
    } else if (registered && !registered.has(panel.id)) {
      problems.push(`panel ${JSON.stringify(panel.id)} from ${panel.package} has no registry entry`);
    }
  }
  return problems;
}

// readUIConfig parses and checks one ui.yaml and throws every violation at once.
export function readUIConfig(file: string, registryIds?: string[]): UIConfig {
  let doc: unknown;
  try {
    doc = parse(readFileSync(file, "utf8"));
  } catch (err) {
    throw new Error(`ui.yaml ${file}: ${err instanceof Error ? err.message : String(err)}`);
  }
  const problems = validateUIConfig(doc);
  if (problems.length === 0) problems.push(...compositionProblems(doc as UIConfig, registryIds));
  if (problems.length > 0) throw new Error(`invalid ui.yaml ${file}:\n  ${problems.join("\n  ")}`);
  return doc as UIConfig;
}

export function uiConfigModuleSource(config: UIConfig): string {
  return `export default ${JSON.stringify(config)};\n`;
}

export function uiYaml(options: UIYamlPluginOptions = {}): Plugin {
  let file = resolve(options.path ?? "../ui.yaml");
  const read = () => readUIConfig(file, options.registryIds);
  return {
    name: "declarative-agents:ui-yaml",
    configResolved(config) {
      file = resolve(config.root, options.path ?? "../ui.yaml");
    },
    buildStart() {
      this.addWatchFile(file);
      read();
    },
    resolveId(id) {
      return id === UI_CONFIG_MODULE ? RESOLVED_UI_CONFIG : undefined;
    },
    load(id) {
      // Read on every load so the dev server serves the file as it is now.
      return id === RESOLVED_UI_CONFIG ? uiConfigModuleSource(read()) : undefined;
    },
    configureServer(server) {
      server.watcher.add(file);
      server.watcher.on("change", (changed) => {
        if (resolve(changed) !== file) return;
        const module = server.moduleGraph.getModuleById(RESOLVED_UI_CONFIG);
        if (module) server.moduleGraph.invalidateModule(module);
        server.ws.send({ type: "full-reload" });
      });
    },
  };
}

export default uiYaml;

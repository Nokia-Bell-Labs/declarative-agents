// Types for the virtual module the kit Vite plugin serves
// (@declarative-agents/ui-kit/vite). An application references them once, for
// example in src/vite-env.d.ts:
//   /// <reference types="@declarative-agents/ui-kit/ui-config" />
declare module "virtual:ui-config" {
  import type { UIConfig } from "@declarative-agents/ui-kit";
  const config: UIConfig;
  export default config;
}

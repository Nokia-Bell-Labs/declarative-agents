import { definePanel } from "../manifest";
import { statusBarManifest } from "./manifest";
import { StatusBar, StatusBarPanel } from "./StatusBar";

export { StatusBar, StatusBarPanel, statusBarManifest };
export const statusBarPanel = definePanel(statusBarManifest, StatusBarPanel);

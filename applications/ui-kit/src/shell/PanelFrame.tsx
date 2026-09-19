import type { ReactNode } from "react";
import "./shell.css";

// PanelFrame is the two-column shell: navigation beside the active panel. The
// dak-shell root scopes the kit shell styles; shell stays for application
// stylesheets that target it.
export function PanelFrame({ sidebar, children }: { sidebar: ReactNode; children: ReactNode }) {
  return (
    <div className="dak-shell shell">
      {sidebar}
      <main className="content">{children}</main>
    </div>
  );
}

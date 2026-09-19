import type { ReactNode } from "react";

// PanelFrame is the two-column shell: navigation beside the active panel.
export function PanelFrame({ sidebar, children }: { sidebar: ReactNode; children: ReactNode }) {
  return (
    <div className="shell">
      {sidebar}
      <main className="content">{children}</main>
    </div>
  );
}

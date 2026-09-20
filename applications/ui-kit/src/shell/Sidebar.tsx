import { Fragment, type MouseEvent, type ReactNode } from "react";
import type { PanelRoute } from "./paths";

export interface SidebarGroup {
  id: string;
  label: string;
  order?: number;
}

export interface SidebarProps {
  title: string;
  routes: PanelRoute[];
  active: string;
  href(panel: string): string;
  onNavigate(event: MouseEvent<HTMLAnchorElement>, panel: string): void;
  // When given, entries render under their group's label in group order;
  // entries without a known group follow ungrouped.
  groups?: SidebarGroup[];
  // Rendered under one entry, for sidebar content that belongs to a panel but
  // is not itself a panel: a history list under Chat, a per-row action. It is
  // called for every visible entry and may return null (srd004 R5, GH-2292).
  sidebarExtra?(route: PanelRoute): ReactNode;
  // Rendered after all entries, for content that belongs to no panel: an
  // overlay action, a link to another origin.
  sidebarFooter?: ReactNode;
}

// Sidebar renders the panel navigation: sidebar, sidebar-title, nav-item,
// nav-item-active, nav-group, and nav-group-label, styled by shell.css under
// the PanelFrame's dak-shell root.
//
// Not every sidebar is only panel entries. An application may carry a chat
// history under one entry, an action that opens an overlay rather than routing,
// or a link to another origin — none of which can be declared in panels[]. The
// application with the most sidebar content was therefore the one not mounting
// AppShell at all; sidebarExtra and sidebarFooter are what let it (GH-2292).
export function Sidebar({ title, routes, active, href, onNavigate, groups, sidebarExtra, sidebarFooter }: SidebarProps) {
  const visible = routes.filter((route) => !route.hidden);
  const entry = (route: PanelRoute) => {
    const link = (
      <a
        href={href(route.id)}
        className={`nav-item${active === route.id ? " nav-item-active" : ""}`}
        aria-current={active === route.id ? "page" : undefined}
        onClick={(event) => onNavigate(event, route.id)}
      >
        <span>{route.label}</span>
      </a>
    );
    if (!sidebarExtra) return <Fragment key={route.id}>{link}</Fragment>;
    return (
      <Fragment key={route.id}>
        {link}
        {sidebarExtra(route)}
      </Fragment>
    );
  };

  if (!groups || groups.length === 0) {
    return (
      <nav className="sidebar">
        <div className="sidebar-title">{title}</div>
        {visible.map(entry)}
        {sidebarFooter}
      </nav>
    );
  }
  const ordered = [...groups].sort((a, b) => (a.order ?? 0) - (b.order ?? 0));
  const known = new Set(ordered.map((group) => group.id));
  return (
    <nav className="sidebar">
      <div className="sidebar-title">{title}</div>
      {ordered.map((group) => {
        const members = visible.filter((route) => route.group === group.id);
        if (members.length === 0) return null;
        return (
          <div key={group.id} className="nav-group">
            <div className="nav-group-label">{group.label}</div>
            {members.map(entry)}
          </div>
        );
      })}
      {visible.filter((route) => !route.group || !known.has(route.group)).map(entry)}
      {sidebarFooter}
    </nav>
  );
}

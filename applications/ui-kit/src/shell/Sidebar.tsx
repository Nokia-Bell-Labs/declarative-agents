import type { MouseEvent } from "react";
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
}

// Sidebar renders the panel navigation with the class names the application
// stylesheets already style: sidebar, sidebar-title, nav-item, nav-item-active.
export function Sidebar({ title, routes, active, href, onNavigate, groups }: SidebarProps) {
  const visible = routes.filter((route) => !route.hidden);
  const entry = (route: PanelRoute) => (
    <a
      key={route.id}
      href={href(route.id)}
      className={`nav-item${active === route.id ? " nav-item-active" : ""}`}
      aria-current={active === route.id ? "page" : undefined}
      onClick={(event) => onNavigate(event, route.id)}
    >
      <span>{route.label}</span>
    </a>
  );

  if (!groups || groups.length === 0) {
    return (
      <nav className="sidebar">
        <div className="sidebar-title">{title}</div>
        {visible.map(entry)}
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
    </nav>
  );
}

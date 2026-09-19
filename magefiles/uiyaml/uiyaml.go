// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

// Package uiyaml parses and validates an application's ui.yaml. Version 2 makes
// the file the composition input of the ui-kit shell (applications srd004 R7):
// panels, monitored agents, the trace backend, branding, and presentation
// flags. A file without a version is version 1 and stays valid.
package uiyaml

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// KitPackage is the panels package whose exports a shell mounts without an
// application registry entry.
const KitPackage = "@declarative-agents/ui-kit"

// Document is one ui.yaml. Unknown top-level keys are ignored so application
// extensions stay valid.
type Document struct {
	Version         int            `yaml:"version,omitempty" json:"version,omitempty"`
	ID              string         `yaml:"id" json:"id"`
	Title           string         `yaml:"title" json:"title"`
	SourceOwner     string         `yaml:"source_owner,omitempty" json:"source_owner,omitempty"`
	Routes          []Route        `yaml:"routes,omitempty" json:"routes,omitempty"`
	Sidebar         Sidebar        `yaml:"sidebar,omitempty" json:"sidebar,omitempty"`
	Actions         map[string]any `yaml:"actions,omitempty" json:"actions,omitempty"`
	Panels          []Panel        `yaml:"panels,omitempty" json:"panels,omitempty"`
	MonitoredAgents []Agent        `yaml:"monitored_agents,omitempty" json:"monitored_agents,omitempty"`
	TraceBackend    *TraceBackend  `yaml:"trace_backend,omitempty" json:"trace_backend,omitempty"`
	Branding        *Branding      `yaml:"branding,omitempty" json:"branding,omitempty"`
	Presentation    map[string]any `yaml:"presentation,omitempty" json:"presentation,omitempty"`
	DeploymentAPI   map[string]any `yaml:"deployment_api,omitempty" json:"deployment_api,omitempty"`
}

// Route is a navigable path that is not a composed panel (version 1 form).
type Route struct {
	ID       string `yaml:"id" json:"id"`
	Path     string `yaml:"path" json:"path"`
	Label    string `yaml:"label,omitempty" json:"label,omitempty"`
	Action   string `yaml:"action,omitempty" json:"action,omitempty"`
	Resource string `yaml:"resource,omitempty" json:"resource,omitempty"`
}

// Sidebar titles the navigation and orders its groups.
type Sidebar struct {
	Title  string           `yaml:"title,omitempty" json:"title,omitempty"`
	Groups map[string]Group `yaml:"groups,omitempty" json:"groups,omitempty"`
}

// Group is one sidebar section.
type Group struct {
	Label string `yaml:"label" json:"label"`
	Order int    `yaml:"order,omitempty" json:"order,omitempty"`
}

// Panel composes one registered component at a route. For KitPackage, Export
// names the kit panel manifest id; any other package requires an application
// registry entry keyed by ID.
type Panel struct {
	ID           string         `yaml:"id" json:"id"`
	Package      string         `yaml:"package" json:"package"`
	Export       string         `yaml:"export,omitempty" json:"export,omitempty"`
	Route        string         `yaml:"route" json:"route"`
	Label        string         `yaml:"label,omitempty" json:"label,omitempty"`
	SidebarGroup string         `yaml:"sidebar_group,omitempty" json:"sidebar_group,omitempty"`
	Hidden       bool           `yaml:"hidden,omitempty" json:"hidden,omitempty"`
	Config       map[string]any `yaml:"config,omitempty" json:"config,omitempty"`
}

// Agent is one monitored agent reached through the monitor proxy.
type Agent struct {
	Name  string `yaml:"name" json:"name"`
	Label string `yaml:"label" json:"label"`
}

// TraceQuerySuffix ends every TraceBackend.QueryPath; the prefix before it is
// the same-origin path the kit reads /query/traces under (srd004 R2.4, R7.1).
const TraceQuerySuffix = "/query/traces/{trace_id}"

// TraceBackend names the agent whose proxy serves the trace queries. When
// QueryPath is set, the kit reads the traces same-origin under its prefix.
type TraceBackend struct {
	Name      string `yaml:"name" json:"name"`
	QueryPath string `yaml:"query_path,omitempty" json:"query_path,omitempty"`
}

// Branding is the shell's title and look.
type Branding struct {
	Title  string `yaml:"title,omitempty" json:"title,omitempty"`
	Logo   string `yaml:"logo,omitempty" json:"logo,omitempty"`
	Accent string `yaml:"accent,omitempty" json:"accent,omitempty"`
}

// routePattern is one path segment: the shell's URL scheme treats the last
// segment as the panel slot (srd004 R5.3).
var routePattern = regexp.MustCompile(`^/[a-z0-9][a-z0-9-]*$`)

// Parse decodes and validates a ui.yaml.
func Parse(data []byte) (Document, error) {
	var doc Document
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return Document{}, fmt.Errorf("parse ui.yaml: %w", err)
	}
	return doc, doc.Validate()
}

// EffectiveVersion is 1 for a file that declares none.
func (d Document) EffectiveVersion() int {
	if d.Version == 0 {
		return 1
	}
	return d.Version
}

// Validate applies the srd004 R7 rules and returns every violation at once.
func (d Document) Validate() error {
	var problems []string
	add := func(format string, args ...any) { problems = append(problems, fmt.Sprintf(format, args...)) }

	switch d.EffectiveVersion() {
	case 1:
		if len(d.Panels) > 0 {
			add("panels require version: 2")
		}
	case 2:
	default:
		add("version %d is not supported (1 or 2)", d.Version)
	}
	if strings.TrimSpace(d.ID) == "" {
		add("id is required")
	}

	ids := map[string]string{}
	paths := map[string]string{}
	claim := func(kind, id, path string) {
		if id == "" {
			add("%s has no id", kind)
		} else if prev, ok := ids[id]; ok {
			add("duplicate id %q (%s and %s)", id, prev, kind)
		} else {
			ids[id] = kind
		}
		if !routePattern.MatchString(path) {
			add("%s %q path %q must be one lower-case segment such as /traces", kind, id, path)
		} else if prev, ok := paths[path]; ok {
			add("route %s collides: %s and %s %q", path, prev, kind, id)
		} else {
			paths[path] = fmt.Sprintf("%s %q", kind, id)
		}
	}
	for _, route := range d.Routes {
		claim("route", route.ID, route.Path)
	}
	for _, panel := range d.Panels {
		claim("panel", panel.ID, panel.Route)
		if strings.TrimSpace(panel.Package) == "" {
			add("panel %q has no package", panel.ID)
		}
		if panel.Package == KitPackage && strings.TrimSpace(panel.Export) == "" {
			add("panel %q from %s must name the kit panel in export", panel.ID, KitPackage)
		}
		if panel.SidebarGroup != "" {
			if _, ok := d.Sidebar.Groups[panel.SidebarGroup]; !ok {
				add("panel %q sidebar_group %q is not declared under sidebar.groups", panel.ID, panel.SidebarGroup)
			}
		}
	}

	agents := map[string]bool{}
	for _, agent := range d.MonitoredAgents {
		if agent.Name == "" {
			add("monitored_agents entry has no name")
		} else if agents[agent.Name] {
			add("monitored agent %q is listed twice", agent.Name)
		}
		agents[agent.Name] = true
	}
	if d.TraceBackend != nil {
		if strings.TrimSpace(d.TraceBackend.Name) == "" {
			add("trace_backend has no name")
		}
		if qp := d.TraceBackend.QueryPath; qp != "" && !(strings.HasPrefix(qp, "/") && strings.HasSuffix(qp, TraceQuerySuffix)) {
			add("trace_backend query_path %q must be an absolute path ending in %s", qp, TraceQuerySuffix)
		}
	}

	if len(problems) == 0 {
		return nil
	}
	sort.Strings(problems)
	return fmt.Errorf("invalid ui.yaml:\n  %s", strings.Join(problems, "\n  "))
}

// CheckHelmRenderable enforces srd004 R7.3: a chart must be able to render the
// file with plain templating, so it holds one document with no anchors or
// aliases.
func CheckHelmRenderable(data []byte) error {
	decoder := yaml.NewDecoder(strings.NewReader(string(data)))
	var first yaml.Node
	if err := decoder.Decode(&first); err != nil {
		return fmt.Errorf("parse ui.yaml: %w", err)
	}
	var second yaml.Node
	if err := decoder.Decode(&second); err == nil {
		return fmt.Errorf("ui.yaml holds more than one YAML document")
	}
	return walkNodes(&first, func(node *yaml.Node) error {
		if node.Anchor != "" || node.Kind == yaml.AliasNode {
			return fmt.Errorf("ui.yaml line %d uses a YAML anchor or alias", node.Line)
		}
		return nil
	})
}

func walkNodes(node *yaml.Node, visit func(*yaml.Node) error) error {
	if err := visit(node); err != nil {
		return err
	}
	for _, child := range node.Content {
		if err := walkNodes(child, visit); err != nil {
			return err
		}
	}
	return nil
}

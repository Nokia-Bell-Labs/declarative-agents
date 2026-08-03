// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"testing"

	"gopkg.in/yaml.v3"
)

// This is the check that would have caught GH-723. ui.yaml declared three panel
// paths and the SPA implemented none of them: it held the active panel in
// component state, so every declared path rendered the chat panel. Each artifact
// read alone looks correct -- the config lists routes, the app renders panels --
// and only the two read together show the declared surface is not served.
//
// So this asserts the config and the app agree on the route set, and it reads
// the app's route table rather than its rendering, because the table is the
// contract the config is co-generated against (srd002 R5, srd003).

type uiRoutesDoc struct {
	Routes []struct {
		ID   string `yaml:"id"`
		Path string `yaml:"path"`
	} `yaml:"routes"`
}

// panelRouteRE matches the entries of PANEL_ROUTES in agents/chatbot/ui/app/src/routes.ts, for
// example: { id: "chat", path: "/chat", label: "Chat" }
var panelRouteRE = regexp.MustCompile(`\{\s*id:\s*"([a-z-]+)",\s*path:\s*"(/[a-z-]*)"`)

func meshUIRoot(t *testing.T) string {
	t.Helper()
	root, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	// The magefiles package runs from applications/chatbot-mesh/magefiles.
	return filepath.Join(filepath.Dir(root), "agents", "chatbot", "ui")
}

func declaredUIRoutes(t *testing.T) map[string]string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(meshUIRoot(t), "ui.yaml"))
	if err != nil {
		t.Fatalf("read ui.yaml: %v", err)
	}
	var doc uiRoutesDoc
	if err := yaml.Unmarshal(data, &doc); err != nil {
		t.Fatalf("parse ui.yaml: %v", err)
	}
	routes := map[string]string{}
	for _, route := range doc.Routes {
		routes[route.ID] = route.Path
	}
	if len(routes) == 0 {
		t.Fatal("ui.yaml declares no routes; the cross-check would pass vacuously")
	}
	return routes
}

func implementedPanelRoutes(t *testing.T) map[string]string {
	t.Helper()
	path := filepath.Join(meshUIRoot(t), "app", "src", "routes.ts")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read routes.ts: %v", err)
	}
	routes := map[string]string{}
	for _, match := range panelRouteRE.FindAllStringSubmatch(string(data), -1) {
		routes[match[1]] = match[2]
	}
	if len(routes) == 0 {
		t.Fatalf("no PANEL_ROUTES entries parsed from %s; the table shape changed and this guard went blind", path)
	}
	return routes
}

// TestUIDeclaredRoutesAreImplemented fails when ui.yaml and the SPA disagree,
// in either direction: a route declared and not implemented renders the wrong
// panel, and a route implemented and not declared is a surface the co-generated
// config does not know about.
func TestUIDeclaredRoutesAreImplemented(t *testing.T) {
	t.Parallel()
	declared := declaredUIRoutes(t)
	implemented := implementedPanelRoutes(t)

	for id, path := range declared {
		got, ok := implemented[id]
		if !ok {
			t.Errorf("ui.yaml declares route %q (%s) that the SPA does not implement", id, path)
			continue
		}
		if got != path {
			t.Errorf("route %q path: ui.yaml has %q, SPA has %q", id, path, got)
		}
	}
	for id, path := range implemented {
		if _, ok := declared[id]; !ok {
			t.Errorf("SPA implements route %q (%s) that ui.yaml does not declare", id, path)
		}
	}
}

// TestUIRouteIDsMatchSidebarGroups keeps the nav and the routes in step: the
// sidebar groups name the panels an operator can reach, so a group without a
// route is an unreachable entry.
func TestUIRouteIDsMatchSidebarGroups(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile(filepath.Join(meshUIRoot(t), "ui.yaml"))
	if err != nil {
		t.Fatalf("read ui.yaml: %v", err)
	}
	var doc struct {
		Sidebar struct {
			Groups map[string]struct{} `yaml:"groups"`
		} `yaml:"sidebar"`
	}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		t.Fatalf("parse ui.yaml: %v", err)
	}
	declared := declaredUIRoutes(t)

	var missing []string
	for group := range doc.Sidebar.Groups {
		if _, ok := declared[group]; !ok {
			missing = append(missing, group)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Errorf("sidebar groups with no declared route: %v", missing)
	}
}

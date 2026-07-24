// Copyright (c) 2026 Nokia. All rights reserved.

package docsapi

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const uxConfigPath = "ui/ux.yaml"

// UXConfig defines profile-owned Knowledge Manager UI routes and actions.
type UXConfig struct {
	ID           string              `json:"id" yaml:"id"`
	Title        string              `json:"title" yaml:"title"`
	SourceOwner  string              `json:"source_owner" yaml:"source_owner"`
	Routes       []UXRoute           `json:"routes" yaml:"routes"`
	Sidebar      UXSidebar           `json:"sidebar" yaml:"sidebar"`
	Actions      map[string]UXAction `json:"actions" yaml:"actions"`
	Presentation UXPresentation      `json:"presentation" yaml:"presentation"`
}

// UXRoute defines one browser route backed by a configured UI action.
type UXRoute struct {
	ID       string `json:"id" yaml:"id"`
	Path     string `json:"path" yaml:"path"`
	Label    string `json:"label" yaml:"label"`
	Action   string `json:"action" yaml:"action"`
	Resource string `json:"resource" yaml:"resource"`
}

// UXSidebar defines sidebar labels and grouping order.
type UXSidebar struct {
	Title  string                 `json:"title" yaml:"title"`
	Groups map[string]UXGroupMeta `json:"groups" yaml:"groups"`
}

// UXGroupMeta defines one sidebar category label and order.
type UXGroupMeta struct {
	Label string `json:"label" yaml:"label"`
	Order int    `json:"order" yaml:"order"`
}

// UXAction maps a UI action to profile-owned route and machine words.
type UXAction struct {
	UIAction             string `json:"ui_action" yaml:"ui_action"`
	RequestMachineAction string `json:"request_machine_action,omitempty" yaml:"request_machine_action,omitempty"`
	RequestMachineRoute  string `json:"request_machine_route,omitempty" yaml:"request_machine_route,omitempty"`
	Route                string `json:"route" yaml:"route"`
}

// UXPresentation controls optional UI panels.
type UXPresentation struct {
	RawYAMLToggle bool `json:"raw_yaml_toggle" yaml:"raw_yaml_toggle"`
	StateDiagram  bool `json:"state_diagram" yaml:"state_diagram"`
	ConfigViewer  bool `json:"config_viewer" yaml:"config_viewer"`
	SourceViewer  bool `json:"source_viewer" yaml:"source_viewer"`
}

// LoadCuratorUXConfig loads the profile-owned UX config beside profile.yaml.
func LoadCuratorUXConfig(profilePath string) (UXConfig, error) {
	if profilePath == "" {
		return UXConfig{}, fmt.Errorf("profile_path is required to load UX config")
	}
	primary := filepath.Join(filepath.Dir(profilePath), uxConfigPath)
	cfg, err := LoadUXConfig(primary)
	if err == nil {
		return cfg, nil
	}
	return UXConfig{}, err
}

// LoadUXConfig loads and validates a Knowledge Manager UX config file.
func LoadUXConfig(path string) (UXConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return UXConfig{}, fmt.Errorf("load UX config %s: %w", path, err)
	}
	var cfg UXConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return UXConfig{}, fmt.Errorf("parse UX config %s: %w", path, err)
	}
	if err := validateUXConfig(cfg); err != nil {
		return UXConfig{}, fmt.Errorf("UX config %s: %w", path, err)
	}
	return cfg, nil
}

func validateUXConfig(cfg UXConfig) error {
	if cfg.ID == "" {
		return fmt.Errorf("id is required")
	}
	routes := uxRoutesByID(cfg.Routes)
	requiredRoutes := map[string]UXRoute{
		"docs_index":  {Path: "/docs", Action: "doc_list"},
		"docs_detail": {Path: "/docs/*", Action: "doc_get"},
	}
	for id, want := range requiredRoutes {
		route := routes[id]
		if route.Path != want.Path {
			return fmt.Errorf("route %q must use path %q", id, want.Path)
		}
		if route.Action != want.Action {
			return fmt.Errorf("route %q must use action %q", id, want.Action)
		}
	}
	return validateUXActions(cfg.Actions, routes)
}

func validateUXActions(actions map[string]UXAction, routes map[string]UXRoute) error {
	type requiredAction struct {
		uiAction, machineAction, machineRoute string
	}
	requiredActions := map[string]requiredAction{
		"list_documents":    {uiAction: "doc_list", machineAction: "doc_list_resource"},
		"read_document":     {uiAction: "doc_get", machineAction: "doc_read_resource"},
		"validate_document": {uiAction: "doc_validate", machineAction: "doc_validate", machineRoute: "/actions/validate"},
		"suggest_changes":   {uiAction: "doc_suggest_changes", machineAction: "doc_suggest_changes", machineRoute: "/actions/suggest"},
		"approve_patch":     {uiAction: "doc_patch_approve", machineAction: "doc_patch_approve", machineRoute: "/actions/patches/{patch_id}/approve"},
		"reject_patch":      {uiAction: "doc_patch_reject", machineAction: "doc_patch_reject", machineRoute: "/actions/patches/{patch_id}/reject"},
	}
	for name, want := range requiredActions {
		action, ok := actions[name]
		if !ok {
			return fmt.Errorf("action %q is required", name)
		}
		if action.UIAction != want.uiAction {
			return fmt.Errorf("action %q must use UI action %q", name, want.uiAction)
		}
		if action.RequestMachineAction != want.machineAction {
			return fmt.Errorf("action %q must use request machine action %q", name, want.machineAction)
		}
		if action.RequestMachineRoute != want.machineRoute {
			return fmt.Errorf("action %q must use request machine route %q", name, want.machineRoute)
		}
		if _, ok := routes[action.Route]; !ok {
			return fmt.Errorf("action %q references unknown route %q", name, action.Route)
		}
	}
	return nil
}

func uxRoutesByID(routes []UXRoute) map[string]UXRoute {
	byID := make(map[string]UXRoute, len(routes))
	for _, route := range routes {
		byID[route.ID] = route
	}
	return byID
}

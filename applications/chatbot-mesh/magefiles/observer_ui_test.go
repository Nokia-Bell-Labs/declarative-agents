// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestObserverUIRouteAndPackageContract(t *testing.T) {
	root := filepath.Join("..", "agents", "observer")
	rest, err := os.ReadFile(filepath.Join(root, "rest.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(rest)
	for _, contract := range []string{
		"path: /monitor/fleet",
		"path: /monitor/state",
		"path: /ui/{path...}",
		`root: "${OBSERVER_UI_ROOT:-agents/observer/ui/dist}"`,
		"index: index.html",
		"spa: false",
		"redirect: {location: /ui/, status: 302}",
	} {
		if !strings.Contains(text, contract) {
			t.Errorf("observer rest.yaml missing UI contract %q", contract)
		}
	}

	data, err := os.ReadFile(filepath.Join(root, "ui", "package.json"))
	if err != nil {
		t.Fatal(err)
	}
	var pkg struct {
		Scripts map[string]string `json:"scripts"`
	}
	if err := json.Unmarshal(data, &pkg); err != nil {
		t.Fatal(err)
	}
	for _, script := range []string{"dev", "build", "preview", "test"} {
		if strings.TrimSpace(pkg.Scripts[script]) == "" {
			t.Errorf("observer UI package missing %q script", script)
		}
	}
	for _, path := range []string{
		filepath.Join(root, "ui", "dist", "index.html"),
		filepath.Join(root, "ui", "src", "App.tsx"),
		filepath.Join(root, "ui", "src", "api.ts"),
		filepath.Join(root, "ui", "src", "useFleet.ts"),
	} {
		if info, err := os.Stat(path); err != nil || info.IsDir() {
			t.Errorf("observer UI package member %s missing: %v", path, err)
		}
	}
}

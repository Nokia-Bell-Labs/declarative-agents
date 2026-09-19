// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package conformance

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// kitPackage is the panels package the kit shell mounts by export without an
// application registry entry (srd004 R7.1).
const kitPackage = "@declarative-agents/ui-kit"

// servedShellUIs are the catalog UIs composed on the kit shell (GH-2274); all
// of their I/O goes through the kit client.
var servedShellUIs = []struct {
	name string
	dir  string
}{
	{"bench", "agents/bench/ui"},
	{"collector", "agents/collector/ui"},
}

var registryKey = regexp.MustCompile(`(?m)^\s+([A-Za-z_][A-Za-z0-9_]*):`)

// TestServedUIsComposeOnKitShell proves each served catalog UI takes its
// sidebar and routes from a version 2 ui.yaml, registers every local panel it
// declares in src/panels.ts, and reads through the kit client rather than raw
// fetch or EventSource (srd004 R5.2, R6.1).
func TestServedUIsComposeOnKitShell(t *testing.T) {
	t.Parallel()
	for _, ui := range servedShellUIs {
		t.Run(ui.name, func(t *testing.T) {
			t.Parallel()
			dir := ProfilePath(ui.dir)

			var doc struct {
				Version int `yaml:"version"`
				Panels  []struct {
					ID      string `yaml:"id"`
					Package string `yaml:"package"`
				} `yaml:"panels"`
			}
			data, err := os.ReadFile(filepath.Join(dir, "ui.yaml"))
			if err != nil {
				t.Fatalf("read ui.yaml: %v", err)
			}
			if err := yaml.Unmarshal(data, &doc); err != nil {
				t.Fatalf("parse ui.yaml: %v", err)
			}
			if doc.Version != 2 || len(doc.Panels) == 0 {
				t.Fatalf("ui.yaml version %d with %d panels, want version 2 with panels", doc.Version, len(doc.Panels))
			}

			registry, err := os.ReadFile(filepath.Join(dir, "src", "panels.ts"))
			if err != nil {
				t.Fatalf("read src/panels.ts: %v", err)
			}
			registered := map[string]bool{}
			for _, m := range registryKey.FindAllStringSubmatch(string(registry), -1) {
				registered[m[1]] = true
			}
			for _, panel := range doc.Panels {
				if panel.Package != kitPackage && !registered[panel.ID] {
					t.Errorf("ui.yaml panel %q has no entry in src/panels.ts", panel.ID)
				}
			}

			main, err := os.ReadFile(filepath.Join(dir, "src", "main.tsx"))
			if err != nil {
				t.Fatalf("read src/main.tsx: %v", err)
			}
			for _, want := range []string{"virtual:ui-config", "<AppShell"} {
				if !strings.Contains(string(main), want) {
					t.Errorf("src/main.tsx does not mount the kit shell: missing %q", want)
				}
			}

			fetches := 0
			err = filepath.WalkDir(filepath.Join(dir, "src"), func(path string, d fs.DirEntry, err error) error {
				if err != nil || d.IsDir() || !(strings.HasSuffix(path, ".ts") || strings.HasSuffix(path, ".tsx")) {
					return err
				}
				source, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				if strings.Contains(string(source), "new EventSource") {
					t.Errorf("%s opens an EventSource outside the kit client", path)
				}
				fetches += strings.Count(string(source), "fetch(")
				return nil
			})
			if err != nil {
				t.Fatalf("walk src: %v", err)
			}
			if fetches != 0 {
				t.Errorf("src/ holds %d raw fetch( calls; read and write through the kit client", fetches)
			}
		})
	}
}

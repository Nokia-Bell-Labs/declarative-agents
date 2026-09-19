// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/Nokia-Bell-Labs/declarative-agents/magefiles/uiyaml"
)

// This is the check that would have caught GH-723. ui.yaml declared three panel
// paths and the SPA implemented none of them: it held the active panel in
// component state, so every declared path rendered the chat panel. Each artifact
// read alone looks correct -- the config lists panels, the app renders panels --
// and only the two read together show the declared surface is not served.
//
// ui.yaml is now the only route table: the kit shell derives routes and sidebar
// from it (applications srd004 R5, R7.4). What can still drift is the registry
// that maps each declared panel to a component, so this asserts the two agree in
// both directions. Kit panels are mounted by export and need no registry entry.

// registryEntryRE matches one `id: Component,` entry of the registry object
// literal in agents/chatbot/ui/app/src/panels.ts.
var registryEntryRE = regexp.MustCompile(`(?m)^\s+"?([a-z][a-z0-9-]*)"?:\s*[A-Za-z_][A-Za-z0-9_]*,?\s*$`)

func meshUIRoot(t *testing.T) string {
	t.Helper()
	root, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	// The magefiles package runs from applications/chatbot-mesh/magefiles.
	return filepath.Join(filepath.Dir(root), "agents", "chatbot", "ui")
}

func registryKeys(panelsTS string) []string {
	start := strings.Index(panelsTS, "export const registry")
	if start < 0 {
		return nil
	}
	body := panelsTS[start:]
	if end := strings.Index(body, "};"); end >= 0 {
		body = body[:end]
	}
	var keys []string
	for _, match := range registryEntryRE.FindAllStringSubmatch(body, -1) {
		keys = append(keys, match[1])
	}
	sort.Strings(keys)
	return keys
}

// panelRegistryMismatch lists the declared non-kit panels with no registry
// entry and the registry entries no panel declares.
func panelRegistryMismatch(uiYAML, panelsTS []byte) ([]string, error) {
	doc, err := uiyaml.Parse(uiYAML)
	if err != nil {
		return nil, err
	}
	keys := registryKeys(string(panelsTS))
	if len(keys) == 0 {
		return nil, fmt.Errorf("no registry entries parsed from panels.ts; the object shape changed and this guard went blind")
	}
	registered := map[string]bool{}
	for _, key := range keys {
		registered[key] = true
	}
	declared := map[string]bool{}
	var problems []string
	for _, panel := range doc.Panels {
		if panel.Package == uiyaml.KitPackage {
			continue
		}
		declared[panel.ID] = true
		if !registered[panel.ID] {
			problems = append(problems, fmt.Sprintf("ui.yaml panel %q (%s) has no registry entry", panel.ID, panel.Route))
		}
	}
	for _, key := range keys {
		if !declared[key] {
			problems = append(problems, fmt.Sprintf("registry entry %q is declared by no ui.yaml panel", key))
		}
	}
	return problems, nil
}

func readMeshUI(t *testing.T, rel string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(meshUIRoot(t), filepath.FromSlash(rel)))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestUIDeclaredPanelsAreRegistered(t *testing.T) {
	t.Parallel()
	problems, err := panelRegistryMismatch(readMeshUI(t, "ui.yaml"), readMeshUI(t, "app/src/panels.ts"))
	if err != nil {
		t.Fatal(err)
	}
	for _, problem := range problems {
		t.Error(problem)
	}
}

// Adding a panel takes one ui.yaml entry and one registry line; either alone
// fails.
func TestUIAddingAPanelNeedsOnlyUIYAMLAndARegistryLine(t *testing.T) {
	t.Parallel()
	uiYAML := string(readMeshUI(t, "ui.yaml"))
	panelsTS := string(readMeshUI(t, "app/src/panels.ts"))
	const entry = "  - id: diagnostics\n    package: local\n    route: /diagnostics\n    label: Diagnostics\n"
	withPanel := strings.Replace(uiYAML, "sidebar:\n", entry+"sidebar:\n", 1)
	withLine := strings.Replace(panelsTS, "  provisioning: ProvisioningPanel,\n", "  provisioning: ProvisioningPanel,\n  diagnostics: DiagnosticsPanel,\n", 1)
	if withPanel == uiYAML || withLine == panelsTS {
		t.Fatal("fixture anchors moved; update the test")
	}

	for name, pair := range map[string][2]string{
		"both":               {withPanel, withLine},
		"ui.yaml only":       {withPanel, panelsTS},
		"registry line only": {uiYAML, withLine},
	} {
		problems, err := panelRegistryMismatch([]byte(pair[0]), []byte(pair[1]))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if (len(problems) == 0) != (name == "both") {
			t.Errorf("%s: problems = %v", name, problems)
		}
	}
}

// The shell has no second route table: routes.ts is gone and App reads the
// bundled ui.yaml.
func TestUIShellReadsRoutesFromUIYAMLOnly(t *testing.T) {
	t.Parallel()
	if _, err := os.Stat(filepath.Join(meshUIRoot(t), "app", "src", "routes.ts")); !os.IsNotExist(err) {
		t.Errorf("app/src/routes.ts exists (err=%v); routes derive from ui.yaml (srd004 R7.4)", err)
	}
	if app := string(readMeshUI(t, "app/src/App.tsx")); !strings.Contains(app, `from "virtual:ui-config"`) {
		t.Error("App.tsx does not read the bundled ui.yaml")
	}
}

// The committed bundle carries every declared panel route, so the served app
// resolves each deep link ui.yaml declares.
func TestUIBundleCarriesDeclaredPanelRoutes(t *testing.T) {
	t.Parallel()
	doc, err := uiyaml.Parse(readMeshUI(t, "ui.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	scripts, err := filepath.Glob(filepath.Join(meshUIRoot(t), "app", "dist", "assets", "*.js"))
	if err != nil || len(scripts) == 0 {
		t.Fatalf("no built script under app/dist/assets: %v", err)
	}
	var bundle strings.Builder
	for _, script := range scripts {
		data, err := os.ReadFile(script)
		if err != nil {
			t.Fatal(err)
		}
		bundle.Write(data)
	}
	for _, panel := range doc.Panels {
		if !strings.Contains(bundle.String(), `"`+panel.Route+`"`) {
			t.Errorf("built bundle does not carry panel route %s", panel.Route)
		}
	}
}

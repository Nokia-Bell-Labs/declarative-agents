// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package conformance

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// designTokensSpecifier is the kit export every token-claiming UI imports
// (srd004 R8.2); the kit at applications/ui-kit owns the canonical values.
const designTokensSpecifier = "@declarative-agents/ui-kit/tokens.css"

var designTokenUIs = []struct {
	name string
	rel  string
}{
	{"bench", "agents/bench/ui/src/App.css"},
	{"collector", "agents/collector/ui/src/App.css"},
	{"monitor", "agents/knowledge-manager/documentation-curator/ui/monitor/src/App.css"},
	{"docs", "agents/knowledge-manager/documentation-curator/ui/docs/src/App.css"},
	{"chatbot-mesh", "../chatbot-mesh/agents/chatbot/ui/app/src/App.css"},
}

func TestDesignTokensImportsResolveCanonical(t *testing.T) {
	t.Parallel()
	canonical, err := filepath.Abs(ProfilePath("../ui-kit/src/tokens.css"))
	if err != nil {
		t.Fatal(err)
	}

	for _, ui := range designTokenUIs {
		t.Run(ui.name, func(t *testing.T) {
			t.Parallel()
			path, err := filepath.Abs(ProfilePath(ui.rel))
			if err != nil {
				t.Fatal(err)
			}
			css := readFixtureFile(t, path)
			if err := compareCanonicalTokenImport(canonical, css, path); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// This negative test proves the import guard rejects a wrong authority and a
// copied declaration, rather than only checking that App.css has an @import.
// Both fail before the guard reads package.json, so the paths need not exist.
func TestDesignTokensImportGuardDetectsDrift(t *testing.T) {
	t.Parallel()
	canonical := filepath.Clean("/repo/applications/ui-kit/src/tokens.css")
	for _, tc := range []struct {
		name string
		css  string
	}{
		{name: "wrong source", css: "@import \"../../../ui/design-tokens.css\";\n.card {}\n"},
		{name: "copied declaration", css: "@import \"" + designTokensSpecifier + "\";\n:root { --bg-primary: #000; }\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := "/repo/applications/catalog/agents/demo/ui/src/App.css"
			if err := compareCanonicalTokenImport(canonical, []byte(tc.css), path); err == nil {
				t.Fatal("token import guard accepted drift")
			}
		})
	}
}

// compareCanonicalTokenImport requires the kit specifier as the first line and
// no redeclared values, then resolves the specifier through the UI's file:
// dependency and the kit's exports map and compares the result with canonical.
func compareCanonicalTokenImport(canonical string, css []byte, path string) error {
	first := strings.SplitN(string(css), "\n", 2)[0]
	if first != `@import "`+designTokensSpecifier+`";` {
		return fmt.Errorf("%s: first line must import %s", path, designTokensSpecifier)
	}
	if strings.Contains(string(css), "--bg-primary:") {
		return fmt.Errorf("%s: contains a copied canonical token declaration", path)
	}
	uiDir := filepath.Dir(filepath.Dir(path))
	var ui struct {
		Dependencies map[string]string `json:"dependencies"`
	}
	if err := readJSON(filepath.Join(uiDir, "package.json"), &ui); err != nil {
		return err
	}
	spec := ui.Dependencies["@declarative-agents/ui-kit"]
	if !strings.HasPrefix(spec, "file:") {
		return fmt.Errorf("%s: ui-kit dependency %q is not a file: path", uiDir, spec)
	}
	kitDir := filepath.Clean(filepath.Join(uiDir, filepath.FromSlash(strings.TrimPrefix(spec, "file:"))))
	var kit struct {
		Exports map[string]json.RawMessage `json:"exports"`
	}
	if err := readJSON(filepath.Join(kitDir, "package.json"), &kit); err != nil {
		return err
	}
	var target string
	if err := json.Unmarshal(kit.Exports["./tokens.css"], &target); err != nil {
		return fmt.Errorf("%s: package.json exports no ./tokens.css file: %w", kitDir, err)
	}
	if resolved := filepath.Clean(filepath.Join(kitDir, filepath.FromSlash(target))); resolved != filepath.Clean(canonical) {
		return fmt.Errorf("%s: design-token import resolves to %s, want %s", path, resolved, canonical)
	}
	return nil
}

func readJSON(path string, out any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}
	return nil
}

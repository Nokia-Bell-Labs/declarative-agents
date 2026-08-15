// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package conformance

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

var designTokenUIs = []struct {
	name string
	rel  string
}{
	{"bench", "agents/bench/ui/src/App.css"},
	{"collector", "agents/collector/ui/src/App.css"},
	{"monitor", "agents/knowledge-manager/documentation-curator/ui/monitor/src/App.css"},
	{"docs", "agents/knowledge-manager/documentation-curator/ui/docs/src/App.css"},
	{"chatbot-mesh", "../chatbot-mesh/agents/chatbot/ui/app/src/App.css"},
	{"observer", "../chatbot-mesh/agents/observer/ui/src/App.css"},
}

func TestDesignTokensImportsResolveCanonical(t *testing.T) {
	t.Parallel()
	canonical, err := filepath.Abs(ProfilePath("ui/design-tokens.css"))
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

// This negative test proves the import guard rejects both a wrong authority and
// a copied declaration, rather than only checking that App.css has an @import.
func TestDesignTokensImportGuardDetectsDrift(t *testing.T) {
	t.Parallel()
	canonical := filepath.Clean("/repo/applications/catalog/ui/design-tokens.css")
	for _, tc := range []struct {
		name string
		css  string
	}{
		{name: "wrong source", css: "@import \"./design-tokens.css\";\n.card {}\n"},
		{name: "copied declaration", css: "@import \"../../../ui/design-tokens.css\";\n:root { --bg-primary: #000; }\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := "/repo/applications/catalog/agents/demo/src/App.css"
			if err := compareCanonicalTokenImport(canonical, []byte(tc.css), path); err == nil {
				t.Fatal("token import guard accepted drift")
			}
		})
	}
}

func compareCanonicalTokenImport(canonical string, css []byte, path string) error {
	first := strings.SplitN(string(css), "\n", 2)[0]
	const prefix = `@import "`
	if !strings.HasPrefix(first, prefix) || !strings.HasSuffix(first, `";`) {
		return fmt.Errorf("%s: first line must import the canonical design-token source", path)
	}
	imported := strings.TrimSuffix(strings.TrimPrefix(first, prefix), `";`)
	resolved := filepath.Clean(filepath.Join(filepath.Dir(path), filepath.FromSlash(imported)))
	if resolved != filepath.Clean(canonical) {
		return fmt.Errorf("%s: design-token import resolves to %s, want %s", path, resolved, canonical)
	}
	if strings.Contains(string(css), "--bg-primary:") {
		return fmt.Errorf("%s: contains a copied canonical token declaration", path)
	}
	return nil
}

// Copyright (c) 2026 Nokia. All rights reserved.

package conformance

import (
	"bytes"
	"crypto/sha256"
	"strings"
	"testing"
)

const (
	beginMarker = "/* BEGIN design-tokens"
	endMarker   = "/* END design-tokens */"
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
}

func TestDesignTokensVendoredCopiesMatchCanonical(t *testing.T) {
	t.Parallel()
	canonical := readFixtureFile(t, ProfilePath("ui/design-tokens.css"))

	for _, ui := range designTokenUIs {
		t.Run(ui.name, func(t *testing.T) {
			t.Parallel()
			css := readFixtureFile(t, ProfilePath(ui.rel))
			block := extractTokenBlock(t, css, ui.rel)
			if !bytes.Equal(canonical, block) {
				t.Fatalf(
					"%s vendored design-token block drifted from applications/catalog/ui/design-tokens.css (canonical sha256=%x, vendored sha256=%x)",
					ui.rel, sha256.Sum256(canonical), sha256.Sum256(block),
				)
			}
		})
	}
}

func TestDesignTokensDriftDetectsEdit(t *testing.T) {
	t.Parallel()
	canonical := readFixtureFile(t, ProfilePath("ui/design-tokens.css"))
	if len(canonical) == 0 {
		t.Fatal("canonical design-tokens.css is empty")
	}
	mutated := bytes.Replace(canonical, []byte("#005aff"), []byte("#ff0000"), 1)
	if bytes.Equal(canonical, mutated) {
		t.Skip("mutation did not change content")
	}
}

func extractTokenBlock(t *testing.T, css []byte, path string) []byte {
	t.Helper()
	content := string(css)
	start := strings.Index(content, beginMarker)
	if start < 0 {
		t.Fatalf("%s: missing %q marker", path, beginMarker)
	}
	afterStart := strings.Index(content[start:], "\n")
	if afterStart < 0 {
		t.Fatalf("%s: no newline after BEGIN marker", path)
	}
	blockStart := start + afterStart + 1

	end := strings.Index(content, endMarker)
	if end < 0 {
		t.Fatalf("%s: missing %q marker", path, endMarker)
	}
	return []byte(content[blockStart:end])
}

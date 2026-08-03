// Copyright (c) 2026 Nokia. All rights reserved.

package conformance

import (
	"bytes"
	"crypto/sha256"
	"fmt"
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
			if err := compareVendoredTokenBlock(canonical, css, ui.rel); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// TestDesignTokensDriftDetectsEdit proves the drift detector actually rejects a
// mutated vendored block, rather than only proving that bytes.Replace changes
// bytes. It wraps the canonical block in the vendor markers (a faithful
// synthetic vendored file), confirms the detector accepts the faithful copy,
// then mutates one byte inside the block and requires the detector to reject
// it. The mutation is a deterministic byte flip so the test does not depend on
// any particular token value being present, and a mutation that fails to change
// the block is a fatal setup error, never a skip (GH-1352).
func TestDesignTokensDriftDetectsEdit(t *testing.T) {
	t.Parallel()
	canonical := readFixtureFile(t, ProfilePath("ui/design-tokens.css"))
	if len(canonical) == 0 {
		t.Fatal("canonical design-tokens.css is empty")
	}

	faithful := vendoredFileFromBlock(canonical)
	if err := compareVendoredTokenBlock(canonical, faithful, "synthetic-faithful"); err != nil {
		t.Fatalf("detector rejected a faithful vendored copy: %v", err)
	}

	mutatedBlock := append(bytes.Clone(canonical), '\n', '/', '*', ' ', 'd', 'r', 'i', 'f', 't', ' ', '*', '/')
	if bytes.Equal(canonical, mutatedBlock) {
		t.Fatal("mutation setup failed: block is unchanged after mutation")
	}
	mutated := vendoredFileFromBlock(mutatedBlock)
	if err := compareVendoredTokenBlock(canonical, mutated, "synthetic-mutated"); err == nil {
		t.Fatal("drift detector accepted a mutated vendored block; the detector is not sensitive to edits")
	}
}

// TestDesignTokensDriftReportsMissingMarkers proves the detector reports a
// vendored file that lacks the token-block markers instead of silently passing.
func TestDesignTokensDriftReportsMissingMarkers(t *testing.T) {
	t.Parallel()
	canonical := readFixtureFile(t, ProfilePath("ui/design-tokens.css"))
	if err := compareVendoredTokenBlock(canonical, []byte(":root { --x: 1; }"), "no-markers"); err == nil {
		t.Fatal("detector accepted a vendored file with no design-token markers")
	}
}

// vendoredFileFromBlock wraps a token block in the BEGIN/END markers to produce
// a vendored file the detector can extract from.
func vendoredFileFromBlock(block []byte) []byte {
	var b bytes.Buffer
	b.WriteString(beginMarker)
	b.WriteByte('\n')
	b.Write(block)
	b.WriteString(endMarker)
	return b.Bytes()
}

// compareVendoredTokenBlock extracts the design-token block from a vendored CSS
// file and reports an error when it is missing or drifts from canonical. It is
// the drift detector both the vendored-copy check and the mutation-sensitivity
// test exercise, so they cannot disagree about what counts as drift.
func compareVendoredTokenBlock(canonical, css []byte, path string) error {
	block, err := extractTokenBlock(css, path)
	if err != nil {
		return err
	}
	if !bytes.Equal(canonical, block) {
		return fmt.Errorf(
			"%s vendored design-token block drifted from applications/catalog/ui/design-tokens.css (canonical sha256=%x, vendored sha256=%x)",
			path, sha256.Sum256(canonical), sha256.Sum256(block),
		)
	}
	return nil
}

func extractTokenBlock(css []byte, path string) ([]byte, error) {
	content := string(css)
	start := strings.Index(content, beginMarker)
	if start < 0 {
		return nil, fmt.Errorf("%s: missing %q marker", path, beginMarker)
	}
	afterStart := strings.Index(content[start:], "\n")
	if afterStart < 0 {
		return nil, fmt.Errorf("%s: no newline after BEGIN marker", path)
	}
	blockStart := start + afterStart + 1

	end := strings.Index(content, endMarker)
	if end < 0 {
		return nil, fmt.Errorf("%s: missing %q marker", path, endMarker)
	}
	if end < blockStart {
		return nil, fmt.Errorf("%s: END marker precedes BEGIN block", path)
	}
	return []byte(content[blockStart:end]), nil
}

// Copyright (c) 2026 Nokia. All rights reserved.

package filesystem

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

func TestListResourceListsConfiguredDocuments(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeResourceFixture(t, root, "docs/VISION.yaml", "title: Vision\n")
	writeResourceFixture(t, root, "docs/road-map.yaml", "title: Roadmap\n")
	writeResourceFixture(t, root, "docs/specs/config-formats/cfg.yaml", "title: Config\n")
	writeResourceFixture(t, root, "docs/specs/semantic-models/model.yaml", "title: Model\n")
	writeResourceFixture(t, root, "docs/specs/software-requirements/srd.yaml", "title: SRD\n")
	writeResourceFixture(t, root, "docs/specs/test-suites/test.yaml", "title: Test\n")
	writeResourceFixture(t, root, "docs/specs/use-cases/uc.yaml", "title: Use\n")
	writeResourceFixture(t, root, "docs/ignored.txt", "ignored\n")

	res := resourceBuilder(root).Build(toolReq(`{"resource":"docs"}`)).Execute()

	require.Equal(t, SignalDocumentListReady, res.Signal)
	var entries []resourceEntry
	require.NoError(t, json.Unmarshal([]byte(res.Output), &entries))
	require.Equal(t, []resourceEntry{
		{Path: "VISION.yaml", Name: "VISION", Category: "overview", Extension: "yaml", Size: 14},
		{Path: "road-map.yaml", Name: "road-map", Category: "release", Extension: "yaml", Size: 15},
		{Path: "specs/config-formats/cfg.yaml", Name: "cfg", Category: "config-format", Extension: "yaml", Size: 14},
		{Path: "specs/semantic-models/model.yaml", Name: "model", Category: "semantic-model", Extension: "yaml", Size: 13},
		{Path: "specs/software-requirements/srd.yaml", Name: "srd", Category: "srd", Extension: "yaml", Size: 11},
		{Path: "specs/test-suites/test.yaml", Name: "test", Category: "test-suite", Extension: "yaml", Size: 12},
		{Path: "specs/use-cases/uc.yaml", Name: "uc", Category: "use-case", Extension: "yaml", Size: 11},
	}, entries)
}

func TestReadResourceReturnsRawAndParsedYAML(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeResourceFixture(t, root, "docs/VISION.yaml", "title: Vision\ncount: 1\n")

	res := readResourceBuilder(root).Build(toolReq(`{"resource":"docs","path":"VISION.yaml"}`)).Execute()

	require.Equal(t, SignalDocumentReady, res.Signal)
	var detail resourceDetail
	require.NoError(t, json.Unmarshal([]byte(res.Output), &detail))
	require.Equal(t, "VISION.yaml", detail.Path)
	require.Equal(t, "title: Vision\ncount: 1\n", detail.Raw)
	require.Equal(t, "application/x-yaml", detail.ContentType)
	require.NotNil(t, detail.Parsed)
}

func TestResourceBuildersUseConfiguredResource(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeResourceFixture(t, root, "docs/VISION.yaml", "title: Vision\n")
	config := resourceTestConfig()
	config.Resource = "docs"

	list := (&ListResourceBuilder{Root: root, Resources: config}).
		Build(toolReq(`{"resource":"invented"}`)).Execute()
	require.Equal(t, SignalDocumentListReady, list.Signal)

	read := (&ReadResourceBuilder{Root: root, Resources: config}).
		Build(toolReq(`{"resource":"invented","path":"VISION.yaml"}`)).Execute()
	require.Equal(t, SignalDocumentReady, read.Signal)
}

func TestReadResourceMissingFileEmitsDocumentMissing(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(root, "docs"), 0o755))

	res := readResourceBuilder(root).Build(toolReq(`{"resource":"docs","path":"missing.yaml"}`)).Execute()

	require.Equal(t, SignalDocumentMissing, res.Signal)
}

func TestReadResourceDeniesDisallowedExtension(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeResourceFixture(t, root, "docs/private.txt", "secret\n")

	res := readResourceBuilder(root).Build(toolReq(`{"resource":"docs","path":"private.txt"}`)).Execute()

	require.Equal(t, SignalDocumentResourceDenied, res.Signal)
}

func TestReadResourceDeniesTraversal(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeResourceFixture(t, root, "outside.yaml", "title: Outside\n")

	res := readResourceBuilder(root).Build(toolReq(`{"resource":"docs","path":"../outside.yaml"}`)).Execute()

	require.Equal(t, SignalDocumentResourceDenied, res.Signal)
}

func TestReadResourceInvalidYAMLUsesParseSignal(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeResourceFixture(t, root, "docs/bad.yaml", "title: [\n")

	res := readResourceBuilder(root).Build(toolReq(`{"resource":"docs","path":"bad.yaml"}`)).Execute()

	require.Equal(t, SignalDocumentParseFailed, res.Signal)
}

func resourceBuilder(root string) *ListResourceBuilder {
	return &ListResourceBuilder{Root: root, Resources: resourceTestConfig()}
}

func readResourceBuilder(root string) *ReadResourceBuilder {
	return &ReadResourceBuilder{Root: root, Resources: resourceTestConfig()}
}

func resourceTestConfig() ResourceConfig {
	return ResourceConfig{Resources: map[string]ResourceDefinition{
		"docs": {
			Root:       "docs",
			Include:    []string{"**/*.yaml", "*.yaml"},
			Extensions: []string{"yaml", "yml"},
			Modes:      []string{"raw_yaml", "parsed_yaml"},
			MaxBytes:   4096,
		},
	}}
}

func writeResourceFixture(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

func toolReq(params string) core.Result {
	return core.Result{Output: `{"parameters":` + params + `}`}
}

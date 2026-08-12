// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

var plainTextToolInits = map[string]bool{
	"invoke_llm": true, "parse_response": true, "report_parse_error": true,
	"reset_history": true, "nudge_reread": true,
	"file_read": true, "file_write": true, "file_edit": true, "file_find": true,
	"validate_specs": true, "reduce_grep_checks": true, "format_report": true,
	"load_graph": true, "extract_task": true,
}

type outputContractBundle struct {
	Tools []struct {
		Name   string `yaml:"name"`
		Type   string `yaml:"type"`
		Init   string `yaml:"init"`
		Output struct {
			Schema map[string]any `yaml:"schema"`
		} `yaml:"output"`
		Config map[string]any `yaml:"config"`
	} `yaml:"tools"`
}

// TestShippedToolOutputKindsMatchRuntimeFamilies is the repository-wide
// regression gate for word families whose Go implementation returns plain text
// and for load_corpus's machine-selected plan arrays (#1543).
func TestShippedToolOutputKindsMatchRuntimeFamilies(t *testing.T) {
	root := filepath.Clean("..")
	paths, err := discoverShippedToolContractYAML(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range paths {
		checkOutputContractBundle(t, path, readOutputContractBundle(t, path))
	}
}

func discoverShippedToolContractYAML(root string) ([]string, error) {
	var paths []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		rel = filepath.ToSlash(rel)
		if walkErr != nil {
			// A generated tree can disappear while another package or
			// integration replaces it. It is outside shipped-source discovery
			// regardless, so that churn must not affect this scan.
			if isGeneratedToolContractTree(rel) {
				return nil
			}
			return walkErr
		}
		if entry.IsDir() {
			if isGeneratedToolContractTree(rel) {
				return filepath.SkipDir
			}
		}
		if entry.IsDir() || (filepath.Ext(path) != ".yaml" && filepath.Ext(path) != ".yml") {
			return nil
		}
		paths = append(paths, path)
		return nil
	})
	sort.Strings(paths)
	return paths, err
}

func isGeneratedToolContractTree(rel string) bool {
	if isGeneratedProfileTree(rel) {
		return true
	}
	for _, part := range strings.Split(filepath.ToSlash(rel), "/") {
		if part == ".git" || part == "generated-files" {
			return true
		}
	}
	return false
}

type outputContractReporter interface {
	Helper()
	Errorf(string, ...any)
}

func checkOutputContractBundle(
	t outputContractReporter,
	path string,
	bundle outputContractBundle,
) {
	t.Helper()
	for _, tool := range bundle.Tools {
		if plainTextToolInits[tool.Init] ||
			(tool.Type == "exec" && plainTextExecWords[tool.Name]) {
			if got := tool.Output.Schema["type"]; got != "string" {
				t.Errorf("%s tool %s init %s output type = %v, want string",
					path, tool.Name, tool.Init, got)
			}
		}
		if tool.Init == "load_corpus" {
			requireLoadCorpusOutput(t, path, tool.Name, tool.Output.Schema)
		}
		if tool.Init == "spool_get_metric" {
			requireMetricPageOutput(t, path, tool.Name, tool.Output.Schema, tool.Config)
		}
		if tool.Init == "otlp_receiver_stop" {
			requireReceiverStopOutput(t, path, tool.Name, tool.Output.Schema)
		}
		if tool.Init == "relay_spans" {
			requireRelayOutput(t, path, tool.Name, tool.Output.Schema)
		}
	}
}

var plainTextExecWords = map[string]bool{
	"build": true, "vet": true, "lint": true, "test": true,
}

func requireReceiverStopOutput(
	t outputContractReporter,
	path, name string,
	schema map[string]any,
) {
	t.Helper()
	properties, _ := schema["properties"].(map[string]any)
	for _, field := range []string{
		"receiver", "address", "status", "queued_batches", "dropped_on_stop",
		"dropped_batches", "dropped_spans", "queued_metrics",
		"dropped_metrics_on_stop", "dropped_metric_batches", "dropped_data_points",
		"drain_policy",
	} {
		if _, ok := properties[field]; !ok {
			t.Errorf("%s tool %s output omits %s", path, name, field)
		}
	}
}

func requireRelayOutput(
	t outputContractReporter,
	path, name string,
	schema map[string]any,
) {
	t.Helper()
	properties, _ := schema["properties"].(map[string]any)
	for _, field := range []string{"endpoint", "span_count"} {
		if _, ok := properties[field]; !ok {
			t.Errorf("%s tool %s output omits %s", path, name, field)
		}
	}
	if _, present := properties["accepted_span_count"]; present {
		t.Errorf("%s tool %s declares dead accepted_span_count", path, name)
	}
}

func requireMetricPageOutput(
	t outputContractReporter,
	path, name string,
	schema, config map[string]any,
) {
	t.Helper()
	properties, _ := schema["properties"].(map[string]any)
	for _, field := range []string{
		"metric_name", "records", "record_count", "page_record_count", "total",
		"data_point_count", "offset", "page_size", "skipped_lines",
	} {
		if _, ok := properties[field]; !ok {
			t.Errorf("%s tool %s output omits %s", path, name, field)
		}
	}
	for _, field := range []string{"path", "metric_name", "page_size", "max_page_size", "offset"} {
		if _, ok := config[field]; !ok {
			t.Errorf("%s tool %s config omits %s", path, name, field)
		}
	}
}

func readOutputContractBundle(t *testing.T, path string) outputContractBundle {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var bundle outputContractBundle
	_ = yaml.Unmarshal(data, &bundle)
	return bundle
}

func requireLoadCorpusOutput(
	t outputContractReporter,
	path, name string,
	schema map[string]any,
) {
	t.Helper()
	if schema["type"] != "object" {
		t.Errorf("%s tool %s output type = %v, want object", path, name, schema["type"])
	}
	properties, _ := schema["properties"].(map[string]any)
	for _, field := range []string{"summary", "grep_checks", "ref_checks", "consistency_checks"} {
		if _, ok := properties[field]; !ok {
			t.Errorf("%s tool %s output omits %s", path, name, field)
		}
	}
}

func TestShippedToolContractDiscoveryExcludesGeneratedTrees(t *testing.T) {
	root := t.TempDir()
	canonical := filepath.Join(
		root, "applications", "catalog", "agents", "example", "declarations.yaml")
	writeToolContractFixture(t, canonical, "string")
	for _, generated := range []string{
		"applications/coding-agent/build/profiles/stale.yaml",
		"applications/coding-agent/helm/profiles/stale.yaml",
		"applications/coding-agent/helm/dist/stale.yaml",
		"generated-files/stale.yaml",
		"node_modules/stale.yaml",
	} {
		writeToolContractFixture(t, filepath.Join(root, filepath.FromSlash(generated)), "object")
	}

	paths, err := discoverShippedToolContractYAML(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 1 || paths[0] != canonical {
		t.Fatalf("discovered paths = %v, want only %s", paths, canonical)
	}
}

func TestShippedToolContractDiscoveryIgnoresConcurrentGeneratedChurn(t *testing.T) {
	root := t.TempDir()
	canonical := filepath.Join(
		root, "applications", "catalog", "agents", "example", "declarations.yaml")
	writeToolContractFixture(t, canonical, "string")
	generated := filepath.Join(
		root, "applications", "coding-agent", "build", "profiles")
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case <-stop:
				return
			default:
			}
			_ = os.MkdirAll(generated, 0o755)
			_ = os.WriteFile(filepath.Join(generated, "stale.yaml"),
				[]byte("tools: [{name: stale, init: file_write, output: {schema: {type: object}}}]\n"),
				0o644)
			_ = os.RemoveAll(generated)
		}
	}()
	for range 50 {
		paths, err := discoverShippedToolContractYAML(root)
		if err != nil {
			close(stop)
			<-done
			t.Fatal(err)
		}
		if len(paths) != 1 || paths[0] != canonical {
			close(stop)
			<-done
			t.Fatalf("discovered paths during generated churn = %v, want only %s",
				paths, canonical)
		}
	}
	close(stop)
	<-done
}

type outputContractRecorder struct {
	messages []string
}

func (*outputContractRecorder) Helper() {}

func (reporter *outputContractRecorder) Errorf(format string, args ...any) {
	reporter.messages = append(reporter.messages, fmt.Sprintf(format, args...))
}

func TestCanonicalToolOutputMismatchRetainsExactDiagnostic(t *testing.T) {
	path := filepath.Join(
		t.TempDir(), "applications", "catalog", "agents", "example", "declarations.yaml")
	writeToolContractFixture(t, path, "object")
	reporter := &outputContractRecorder{}
	checkOutputContractBundle(reporter, path, readOutputContractBundle(t, path))
	want := fmt.Sprintf(
		"%s tool write init file_write output type = object, want string", path)
	if len(reporter.messages) != 1 || reporter.messages[0] != want {
		t.Fatalf("diagnostics = %v, want [%s]", reporter.messages, want)
	}
}

func writeToolContractFixture(t *testing.T, path, outputType string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	data := fmt.Sprintf(
		"tools:\n  - name: write\n    init: file_write\n    output: {schema: {type: %s}}\n",
		outputType)
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}

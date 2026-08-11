// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"io/fs"
	"os"
	"path/filepath"
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
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			switch entry.Name() {
			case ".git", "node_modules", "generated-files":
				return filepath.SkipDir
			}
		}
		if entry.IsDir() || (filepath.Ext(path) != ".yaml" && filepath.Ext(path) != ".yml") {
			return nil
		}
		bundle := readOutputContractBundle(t, path)
		for _, tool := range bundle.Tools {
			if plainTextToolInits[tool.Init] {
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
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func requireReceiverStopOutput(t *testing.T, path, name string, schema map[string]any) {
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

func requireRelayOutput(t *testing.T, path, name string, schema map[string]any) {
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
	t *testing.T, path, name string, schema, config map[string]any,
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

func requireLoadCorpusOutput(t *testing.T, path, name string, schema map[string]any) {
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

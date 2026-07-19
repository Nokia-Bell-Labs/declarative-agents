// Copyright (c) 2026 Nokia. All rights reserved.

package main

import "testing"

func TestDocsYAMLCategory(t *testing.T) {
	tests := map[string]string{
		"docs/ARCHITECTURE.yaml":                                   "top_level",
		"docs/specs/config-formats/machine-format.yaml":            "config_formats",
		"docs/specs/semantic-models/tool-language.yaml":            "semantic_models",
		"docs/specs/software-requirements/srd001-core-types.yaml":  "software_requirements",
		"docs/specs/use-cases/rel01.0-uc001-generator-coding.yaml": "use_cases",
		"docs/specs/test-suites/test-rel00.0.yaml":                 "test_suites",
		"docs/specs/other/example.yaml":                            "specs_other",
		"docs/notes/internal/example.yaml":                         "docs_other",
	}
	for path, want := range tests {
		if got := docsYAMLCategory(path); got != want {
			t.Fatalf("docsYAMLCategory(%q) = %q, want %q", path, got, want)
		}
	}
}

func TestConfigsYAMLCategory(t *testing.T) {
	tests := map[string]string{
		"tools/builtin.yaml":                 "shared_tools",
		"agents/executor/llm/default.yaml":  "llm_configs",
		"agents/critic/llm/devstral.yaml": "llm_configs",
		"agents/executor/machine.yaml":      "executor",
		"agents/planner/machine.yaml":        "planner",
		"agents/critic/machine.yaml":      "critic",
		"agents/bench/machine.yaml":          "bench",
		"agents/jurist/machine.yaml":         "jurist",
		"configs/experimental/machine.yaml":  "configs_other",
	}
	for path, want := range tests {
		if got := configsYAMLCategory(path); got != want {
			t.Fatalf("configsYAMLCategory(%q) = %q, want %q", path, got, want)
		}
	}
}

func TestAddYAMLStats(t *testing.T) {
	var stats yamlStatsJSON
	stats.Docs.Categories = make(map[string]fileLineStats)
	stats.Configs.Categories = make(map[string]fileLineStats)

	addYAMLStats(&stats, "docs/specs/config-formats/machine-format.yaml", 10)
	addYAMLStats(&stats, "tools/builtin.yaml", 20)
	addYAMLStats(&stats, "agents/executor/machine.yaml", 7)
	addYAMLStats(&stats, "README.yaml", 3)

	if stats.Total.Files != 4 || stats.Total.Lines != 40 {
		t.Fatalf("total = %+v, want files=4 lines=40", stats.Total)
	}
	if got := stats.Docs.Categories["config_formats"]; got.Files != 1 || got.Lines != 10 {
		t.Fatalf("docs config_formats = %+v, want files=1 lines=10", got)
	}
	if got := stats.Configs.Categories["shared_tools"]; got.Files != 1 || got.Lines != 20 {
		t.Fatalf("configs shared_tools = %+v, want files=1 lines=20", got)
	}
	if got := stats.Configs.Categories["executor"]; got.Files != 1 || got.Lines != 7 {
		t.Fatalf("configs generator = %+v, want files=1 lines=7", got)
	}
	if stats.Other.Files != 1 || stats.Other.Lines != 3 {
		t.Fatalf("other = %+v, want files=1 lines=3", stats.Other)
	}
}

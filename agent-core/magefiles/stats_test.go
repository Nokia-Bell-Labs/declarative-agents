// Copyright (c) 2026 Nokia. All rights reserved.

package main

import "testing"

func TestDocsYAMLCategory(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		path string
		want string
	}{
		{name: "top level", path: "docs/ARCHITECTURE.yaml", want: "top_level"},
		{name: "config format", path: "docs/specs/config-formats/machine-format.yaml", want: "config_formats"},
		{name: "semantic model", path: "docs/specs/semantic-models/tool-language.yaml", want: "semantic_models"},
		{name: "software requirement", path: "docs/specs/software-requirements/srd001-core-types.yaml", want: "software_requirements"},
		{name: "use case", path: "docs/specs/use-cases/rel01.0-uc001-generator-coding.yaml", want: "use_cases"},
		{name: "test suite", path: "docs/specs/test-suites/test-rel00.0.yaml", want: "test_suites"},
		{name: "other spec", path: "docs/specs/other/example.yaml", want: "specs_other"},
		{name: "other docs", path: "docs/notes/internal/example.yaml", want: "docs_other"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := docsYAMLCategory(tt.path); got != tt.want {
				t.Fatalf("docsYAMLCategory(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestConfigsYAMLCategory(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		path string
		want string
	}{
		{name: "shared tools", path: "tools/builtin.yaml", want: "shared_tools"},
		{name: "executor LLM", path: "agents/executor/llm/default.yaml", want: "llm_configs"},
		{name: "critic LLM", path: "agents/critic/llm/devstral.yaml", want: "llm_configs"},
		{name: "executor", path: "agents/executor/machine.yaml", want: "executor"},
		{name: "planner", path: "agents/planner/machine.yaml", want: "planner"},
		{name: "critic", path: "agents/critic/machine.yaml", want: "critic"},
		{name: "bench", path: "agents/bench/machine.yaml", want: "bench"},
		{name: "jurist", path: "agents/jurist/machine.yaml", want: "jurist"},
		{name: "other config", path: "configs/experimental/machine.yaml", want: "configs_other"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := configsYAMLCategory(tt.path); got != tt.want {
				t.Fatalf("configsYAMLCategory(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
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

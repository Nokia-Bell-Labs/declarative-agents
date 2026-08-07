// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	"gopkg.in/yaml.v3"
)

type golangciConfig struct {
	Version string `yaml:"version"`
	Linters struct {
		Enable   []string `yaml:"enable"`
		Settings struct {
			Forbidigo struct {
				AnalyzeTypes bool `yaml:"analyze-types"`
				Forbid       []struct {
					Pattern string `yaml:"pattern"`
				} `yaml:"forbid"`
			} `yaml:"forbidigo"`
		} `yaml:"settings"`
		Exclusions struct {
			Rules []struct {
				Path    string   `yaml:"path"`
				Linters []string `yaml:"linters"`
			} `yaml:"rules"`
		} `yaml:"exclusions"`
	} `yaml:"linters"`
	// Formatters is a sibling of Linters in the v2 schema, not a member of
	// linters.enable, which is why enabling gofmt there would be ignored.
	Formatters struct {
		Enable []string `yaml:"enable"`
	} `yaml:"formatters"`
}

func TestLintModulesCoverRepositoryGoModules(t *testing.T) {
	var modules []string
	err := filepath.WalkDir("..", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() && (entry.Name() == ".git" || entry.Name() == "node_modules" || entry.Name() == "testdata") {
			return filepath.SkipDir
		}
		if entry.Name() != "go.mod" {
			return nil
		}
		module, relErr := filepath.Rel("..", filepath.Dir(path))
		if relErr != nil {
			return relErr
		}
		modules = append(modules, filepath.ToSlash(module))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(modules)
	want := slices.Clone(lintModuleDirs)
	slices.Sort(want)
	if !reflect.DeepEqual(modules, want) {
		t.Fatalf("Go modules = %#v, lint modules = %#v", modules, want)
	}
}

func TestForbidigoConfigsRejectProcessEnvAndPermitTestStaging(t *testing.T) {
	for _, module := range lintModuleDirs {
		t.Run(module, func(t *testing.T) {
			config := readGolangciConfig(t, module)
			if config.Version != "2" || !slices.Contains(config.Linters.Enable, "forbidigo") {
				t.Fatalf("config version/enabled = %q/%v, want v2 forbidigo", config.Version, config.Linters.Enable)
			}
			if !config.Linters.Settings.Forbidigo.AnalyzeTypes {
				t.Fatal("forbidigo analyze-types must be enabled")
			}
			var patterns []string
			for _, forbidden := range config.Linters.Settings.Forbidigo.Forbid {
				patterns = append(patterns, forbidden.Pattern)
			}
			wantPatterns := []string{`^os\.Getenv$`, `^os\.LookupEnv$`, `^os\.Setenv$`}
			if !reflect.DeepEqual(patterns, wantPatterns) {
				t.Fatalf("forbidden patterns = %#v, want %#v", patterns, wantPatterns)
			}
			if !excludesForbidigoTests(config) {
				t.Fatal("forbidigo must exclude _test.go staging")
			}
		})
	}
}

func TestLintDispatchesEveryModule(t *testing.T) {
	var got []string
	if err := lintSubModules(lintModuleDirs, func(dir string) error {
		got = append(got, dir)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, lintModuleDirs) {
		t.Fatalf("linted modules = %#v, want %#v", got, lintModuleDirs)
	}
}

// TestFormatterConfigsEnableGofmt keeps the declared policy and the enforced
// rule the same rule. Formatting was enforced nowhere because the configs pin an
// explicit linter set and never declared a formatter, and gofmt cannot be
// declared in linters.enable: it is a formatter in the v2 schema, so that entry
// would be ignored rather than rejected. magefiles/format_test.go is what fails
// a build; this asserts the configs agree with it (GH-1477).
func TestFormatterConfigsEnableGofmt(t *testing.T) {
	for _, module := range lintModuleDirs {
		t.Run(module, func(t *testing.T) {
			config := readGolangciConfig(t, module)
			if !slices.Contains(config.Formatters.Enable, "gofmt") {
				t.Fatalf("formatters.enable = %v, want gofmt", config.Formatters.Enable)
			}
			if slices.Contains(config.Linters.Enable, "gofmt") {
				t.Fatal("gofmt belongs in formatters.enable; under linters.enable it is ignored")
			}
		})
	}
}

func readGolangciConfig(t *testing.T, module string) golangciConfig {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", filepath.FromSlash(module), ".golangci.yml"))
	if err != nil {
		t.Fatal(err)
	}
	var config golangciConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		t.Fatal(err)
	}
	return config
}

func excludesForbidigoTests(config golangciConfig) bool {
	for _, rule := range config.Linters.Exclusions.Rules {
		if rule.Path == `_test\.go$` && slices.Contains(rule.Linters, "forbidigo") {
			return true
		}
	}
	return false
}

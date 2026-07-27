// Copyright (c) 2026 Nokia. All rights reserved.

package conformance

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

const alignmentMigrationPath = "docs/migrations/v0.20260727.0-agent-role-realization-alignment.yaml"

func TestAlignmentMigrationReleaseAndConsumerPins(t *testing.T) {
	t.Parallel()
	type migration struct {
		OldName       string   `yaml:"old_name"`
		NewName       string   `yaml:"new_name"`
		OldPath       string   `yaml:"old_path"`
		OldPaths      []string `yaml:"old_paths"`
		NewPath       string   `yaml:"new_path"`
		Compatibility string   `yaml:"compatibility"`
	}
	var note struct {
		Release            string      `yaml:"release"`
		LegacyReleaseAlias string      `yaml:"legacy_release_alias"`
		RootRelease        string      `yaml:"root_release"`
		Status             string      `yaml:"status"`
		Migrations         []migration `yaml:"migrations"`
	}
	readRoleYAML(t, alignmentMigrationPath, &note)

	const root = "v0.20260727.0"
	if note.RootRelease != root ||
		note.Release != "applications/catalog/"+root ||
		note.LegacyReleaseAlias != "agent-profiles/"+root ||
		note.Status != "release-ready" {
		t.Fatalf("migration release metadata = %#v, want exact release-ready canonical/legacy pair for %s", note, root)
	}
	if !regexp.MustCompile(`^v0\.[0-9]{8}\.[0-9]+$`).MatchString(note.RootRelease) {
		t.Errorf("root release %q is not an exact daily v0 release", note.RootRelease)
	}

	got := map[string]string{}
	for _, item := range note.Migrations {
		got[item.OldName] = item.NewName
		if strings.TrimSpace(item.Compatibility) == "" {
			t.Errorf("%s -> %s has no compatibility policy", item.OldName, item.NewName)
		}
		if item.OldPath == "" && len(item.OldPaths) == 0 {
			t.Errorf("%s -> %s has no migration path entry", item.OldName, item.NewName)
		}
	}
	want := map[string]string{
		"assembler":              "scenario-critic",
		"monitor":                "runtime-state-reader",
		"jurist":                 "specification-critic",
		"chatbot-turn-validator": "chatbot-turn-critic",
		"rag-query-validator":    "rag-query-critic",
		"coordinator":            "provisioning-workflow-orchestrator",
	}
	for oldName, newName := range want {
		if got[oldName] != newName {
			t.Errorf("migration table %q = %q, want %q", oldName, got[oldName], newName)
		}
	}

	var coding struct {
		AgentProfiles struct {
			CompatibleRelease string `yaml:"compatible_release"`
		} `yaml:"agent_profiles"`
	}
	readRoleYAML(t, "../coding-agent/agents/application.yaml", &coding)
	if coding.AgentProfiles.CompatibleRelease != note.Release {
		t.Errorf("coding-agent compatible_release = %q, want %q", coding.AgentProfiles.CompatibleRelease, note.Release)
	}
	var chart struct {
		Annotations map[string]string `yaml:"annotations"`
	}
	readRoleYAML(t, "../chatbot-mesh/helm/Chart.yaml", &chart)
	if chart.Annotations["declarative-agents.nokia.com/catalog-compatible-release"] != note.Release {
		t.Errorf("chatbot-mesh catalog release annotation = %q, want %q",
			chart.Annotations["declarative-agents.nokia.com/catalog-compatible-release"], note.Release)
	}
}

func TestAlignmentHasNoStaleActiveRuntimeIdentities(t *testing.T) {
	t.Parallel()
	applicationsRoot := filepath.Clean(filepath.Join(ProfilesRoot(), ".."))
	roots := []string{
		ProfilePath("agents"),
		filepath.Join(applicationsRoot, "coding-agent", "agents"),
		filepath.Join(applicationsRoot, "coding-agent", "helm"),
		filepath.Join(applicationsRoot, "chatbot-mesh", "agents"),
		filepath.Join(applicationsRoot, "chatbot-mesh", "helm", "templates"),
		filepath.Join(applicationsRoot, "chatbot-mesh", "testdata"),
	}
	stale := []string{
		"chatbot-turn-" + "validator",
		"rag-query-" + "validator",
		"agents/" + "coordinator",
		"coordinator" + ".yaml",
	}
	var hits []string
	for _, root := range roots {
		err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() {
				if path == ProfilePath("agents/assembler") ||
					path == ProfilePath("agents/monitor") ||
					path == ProfilePath("agents/jurist") {
					return filepath.SkipDir
				}
				return nil
			}
			switch filepath.Ext(path) {
			case ".go", ".md", ".yaml", ".yml", ".tpl":
			default:
				return nil
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			for _, retired := range stale {
				if strings.Contains(string(data), retired) {
					rel, _ := filepath.Rel(filepath.Dir(applicationsRoot), path)
					hits = append(hits, filepath.ToSlash(rel)+": "+retired)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	sort.Strings(hits)
	if len(hits) != 0 {
		t.Errorf("stale active runtime identities outside compatibility/history allowlists:\n%s", strings.Join(hits, "\n"))
	}
}

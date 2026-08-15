// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"errors"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/pkg/profileaudit"
	"github.com/Nokia-Bell-Labs/declarative-agents/magefiles/appmanifest"
)

func TestAgentArchitectureProfileTimeoutEnvelopes(t *testing.T) {
	resolved, profiles := stageAgentArchitectureTimeoutProfiles(t)
	covered := make([]string, 0, len(profiles))
	for _, profile := range profiles {
		if err := profileaudit.ValidateWithOptions(
			profile, profileaudit.Options{CoreRoot: resolved.Core},
		); err != nil {
			t.Errorf("%s: %v", profile, err)
		}
		covered = append(covered, filepath.Base(profile))
	}
	sort.Strings(covered)
	want := []string{"apply-profile.yaml", "profile.yaml", "rollout-profile.yaml"}
	if strings.Join(covered, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("manifest-owned local profiles = %v, want %v", covered, want)
	}
}

func TestAgentArchitectureTimeoutAtMachineEnvelopeIsRejected(t *testing.T) {
	resolved, profiles := stageAgentArchitectureTimeoutProfiles(t)
	root := profileNamed(t, profiles, "profile.yaml")
	report, err := profileaudit.InspectWithOptions(
		root, profileaudit.Options{CoreRoot: resolved.Core},
	)
	if err != nil {
		t.Fatal(err)
	}
	var target profileaudit.Operation
	for _, operation := range report.Operations {
		if operation.Action == "await_applier_control" &&
			strings.Contains(operation.Authority, "queue.timeout") {
			target = operation
			break
		}
	}
	if target.Action == "" {
		t.Fatal("manifest-owned applier closure exposes no control queue timeout")
	}

	restPath := filepath.Join(filepath.Dir(root), "rest.yaml")
	setYAMLPath(t, restPath, target.CommandTimeout.String(),
		"rest", "servers", "applier_control", "queue", "timeout")
	err = profileaudit.ValidateWithOptions(
		root, profileaudit.Options{CoreRoot: resolved.Core},
	)
	var validation *profileaudit.ValidationError
	if !errors.As(err, &validation) {
		t.Fatalf("operation equal to its machine envelope was not rejected: %v", err)
	}
	for _, diagnostic := range validation.Diagnostics {
		if diagnostic.Action == target.Action &&
			diagnostic.Duration == target.CommandTimeout {
			return
		}
	}
	t.Fatalf("mutation rejection did not identify %s at %s: %v",
		target.Action, target.CommandTimeout, validation.Diagnostics)
}

func stageAgentArchitectureTimeoutProfiles(t *testing.T) (roots, []string) {
	t.Helper()
	resolved, err := resolveRootsFromWorkingDirectory()
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := appmanifest.Load(
		filepath.Join(resolved.Application, "agents", "application.yaml"),
		appmanifest.Options{
			ApplicationRoot: resolved.Application,
			CatalogRoot:     resolved.Catalog,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	chart := t.TempDir()
	if err := prepareChartProfiles(resolved.Application, resolved.Catalog, chart); err != nil {
		t.Fatal(err)
	}
	var prepared preparedManifest
	if err := readStrictYAML(
		filepath.Join(chart, "profiles", preparedManifestFilename), &prepared,
	); err != nil {
		t.Fatal(err)
	}

	localRoots := make(map[string]string)
	for _, root := range manifest.Roots {
		if root.Ownership == "local" {
			localRoots[root.ID] = filepath.ToSlash(filepath.Dir(root.RuntimePath))
		}
	}
	var profiles []string
	for _, role := range prepared.Roles {
		directory, local := localRoots[role.Root]
		if !local {
			continue
		}
		for _, filename := range role.Files {
			slash := filepath.ToSlash(filename)
			if filepath.ToSlash(filepath.Dir(slash)) != directory ||
				!strings.HasSuffix(filepath.Base(slash), "profile.yaml") {
				continue
			}
			profiles = append(profiles,
				filepath.Join(chart, "profiles", role.Path, filepath.FromSlash(slash)))
		}
	}
	sort.Strings(profiles)
	if len(profiles) == 0 {
		t.Fatal("application manifest resolved no local shipped profiles")
	}
	return resolved, profiles
}

func profileNamed(t *testing.T, profiles []string, name string) string {
	t.Helper()
	for _, profile := range profiles {
		if filepath.Base(profile) == name {
			return profile
		}
	}
	t.Fatalf("manifest-owned profiles contain no %s: %v", name, profiles)
	return ""
}

func setYAMLPath(t *testing.T, filename, value string, path ...string) {
	t.Helper()
	var document map[string]interface{}
	if err := readStrictYAML(filename, &document); err != nil {
		t.Fatal(err)
	}
	current := document
	for _, key := range path[:len(path)-1] {
		next, ok := current[key].(map[string]interface{})
		if !ok {
			t.Fatalf("%s path %v has no mapping at %s", filename, path, key)
		}
		current = next
	}
	current[path[len(path)-1]] = value
	if err := writeYAML(filename, document); err != nil {
		t.Fatal(err)
	}
}

// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/pkg/profileaudit"
)

func TestAgentCoreProfileTimeoutEnvelopes(t *testing.T) {
	coreRoot := coreRootForPolicyTest(t)
	profiles := discoverCoreProfiles(t, coreRoot)
	for _, profile := range profiles {
		if err := profileaudit.ValidateWithOptions(
			profile, profileaudit.Options{CoreRoot: coreRoot},
		); err != nil {
			t.Errorf("%s: %v", profile, err)
		}
	}
	t.Logf("validated %d core-owned profiles", len(profiles))
}

func TestAgentCoreProfileTimeoutMutationIsRejected(t *testing.T) {
	coreRoot := coreRootForPolicyTest(t)
	source := filepath.Join(coreRoot, "testdata", "integration", "profiles", "otlp-replay")
	mutated := filepath.Join(t.TempDir(), "otlp-replay")
	copyPolicyFixture(t, source, mutated)
	profile := filepath.Join(mutated, "profile.yaml")

	report, err := profileaudit.InspectWithOptions(
		profile, profileaudit.Options{CoreRoot: coreRoot},
	)
	if err != nil {
		t.Fatal(err)
	}
	var selected profileaudit.Operation
	for _, operation := range report.Operations {
		if operation.Action == "relay_replay_batch" {
			selected = operation
			break
		}
	}
	if selected.Action == "" {
		t.Fatal("OTLP replay profile exposes no relay operation authority")
	}
	declaration := filepath.Join(mutated, "declarations.yaml")
	data, err := os.ReadFile(declaration)
	if err != nil {
		t.Fatal(err)
	}
	before := "timeout: " + selected.RawDuration
	after := "timeout: " + selected.CommandTimeout.String()
	changed := strings.Replace(string(data), before, after, 1)
	if changed == string(data) {
		t.Fatalf("selected operation authority %q not found in %s", before, declaration)
	}
	if err := os.WriteFile(declaration, []byte(changed), 0o644); err != nil {
		t.Fatal(err)
	}
	err = profileaudit.ValidateWithOptions(
		profile, profileaudit.Options{CoreRoot: coreRoot},
	)
	var validation *profileaudit.ValidationError
	if !errors.As(err, &validation) {
		t.Fatalf("operation authority equal to its machine envelope was not rejected by policy: %v", err)
	}
	if len(validation.Diagnostics) == 0 ||
		validation.Diagnostics[0].Action != selected.Action {
		t.Fatalf("mutation rejection did not identify %s: %v", selected.Action, validation.Diagnostics)
	}
}

func coreRootForPolicyTest(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func discoverCoreProfiles(t *testing.T, coreRoot string) []string {
	t.Helper()
	var profiles []string
	err := filepath.WalkDir(coreRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !isCoreProfileName(entry.Name()) {
			return nil
		}
		profiles = append(profiles, path)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(profiles) == 0 {
		t.Fatal("no core-owned profiles discovered")
	}
	sort.Strings(profiles)
	return profiles
}

func isCoreProfileName(name string) bool {
	return name == "profile.yaml" ||
		strings.HasPrefix(name, "profile-") && strings.HasSuffix(name, ".yaml") ||
		strings.HasSuffix(name, "-profile.yaml")
}

func copyPolicyFixture(t *testing.T, source, destination string) {
	t.Helper()
	err := filepath.WalkDir(source, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, rel)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err := os.WriteFile(target, data, 0o644); err != nil {
			return fmt.Errorf("copy %s: %w", path, err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

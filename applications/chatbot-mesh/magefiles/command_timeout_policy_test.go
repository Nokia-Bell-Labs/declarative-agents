// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/pkg/profileaudit"
)

const observerPollIntervalPattern = `^(?:(?:[1-9]|[1-9][0-9]|[1-5][0-9]{2})s|(?:[1-9]|10)m)$`

func TestChatbotMeshProfileTimeoutEnvelopes(t *testing.T) {
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	coreRoot := demoCoreRoot(root)
	profiles, compositionProfiles := stageMeshTimeoutProfiles(t, root)
	for _, profile := range profiles {
		if err := profileaudit.ValidateWithOptions(
			profile, profileaudit.Options{CoreRoot: coreRoot},
		); err != nil {
			t.Errorf("%s: %v", profile, err)
		}
	}

	orchestrator := compositionProfiles["provisioning-workflow-orchestrator"]
	report, err := profileaudit.InspectWithOptions(
		orchestrator, profileaudit.Options{CoreRoot: coreRoot},
	)
	if err != nil {
		t.Fatal(err)
	}
	requestMachines := map[string]bool{}
	for _, operation := range report.Operations {
		requestMachines[filepath.Base(operation.Machine)] = true
	}
	for _, name := range []string{"request-machine.yaml", "rollout-machine.yaml", "state-machine.yaml"} {
		if !requestMachines[name] {
			t.Errorf("orchestrator closure omitted request variant %s", name)
		}
	}
	t.Logf(
		"validated %d mesh profiles (%d composition roots)",
		len(profiles), len(compositionProfiles),
	)
}

func TestObserverPollIntervalAtMachineEnvelopeIsRejected(t *testing.T) {
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	coreRoot := demoCoreRoot(root)
	profile := filepath.Join(root, "agents", "observer", "profile.yaml")
	report, err := profileaudit.InspectWithOptions(
		profile, profileaudit.Options{CoreRoot: coreRoot},
	)
	if err != nil {
		t.Fatal(err)
	}
	var observerAwait profileaudit.Operation
	for _, operation := range report.Operations {
		if operation.Action == "await_observer_control" &&
			operation.Authority == "ToolDef config.timeout" {
			observerAwait = operation
			break
		}
	}
	if observerAwait.Action == "" {
		t.Fatal("observer closure exposes no polling interval authority")
	}
	t.Setenv("OBSERVER_POLL_INTERVAL", observerAwait.CommandTimeout.String())
	err = profileaudit.ValidateWithOptions(
		profile, profileaudit.Options{CoreRoot: coreRoot},
	)
	var validation *profileaudit.ValidationError
	if !errors.As(err, &validation) {
		t.Fatalf("observer poll interval equal to its machine envelope was not rejected by policy: %v", err)
	}
	if len(validation.Diagnostics) == 0 ||
		validation.Diagnostics[0].Action != observerAwait.Action {
		t.Fatalf("observer mutation rejection did not identify %s: %v",
			observerAwait.Action, validation.Diagnostics)
	}
}

func TestObserverPollIntervalHelmSchemaConstraint(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile("../helm/values.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	var schema map[string]interface{}
	if err := json.Unmarshal(data, &schema); err != nil {
		t.Fatal(err)
	}
	properties := schema["properties"].(map[string]interface{})
	observer := properties["observer"].(map[string]interface{})
	observerProperties := observer["properties"].(map[string]interface{})
	pollInterval := observerProperties["pollInterval"].(map[string]interface{})
	if pollInterval["pattern"] != observerPollIntervalPattern {
		t.Errorf("observer pollInterval pattern = %q, want %q", pollInterval["pattern"], observerPollIntervalPattern)
	}

	declaration, err := os.ReadFile("../agents/observer/declarations.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(declaration), "timeout: ${OBSERVER_POLL_INTERVAL") {
		t.Error("observer await no longer reads the deployment poll interval")
	}
}

func stageMeshTimeoutProfiles(
	t *testing.T,
	root string,
) ([]string, map[string]string) {
	t.Helper()
	catalogRoot, err := resolveCatalogRoot("chatbot-mesh timeout policy", root)
	if err != nil {
		t.Fatal(err)
	}
	composition, err := resolveChatbotComposition(root, catalogRoot)
	if err != nil {
		t.Fatal(err)
	}
	chart, cleanup, err := stagePackageChart(filepath.Join(root, "helm"), root, catalogRoot)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cleanup)

	profileSet := map[string]bool{}
	compositionProfiles := make(map[string]string, len(composition.manifest.Roots))
	for _, manifestRoot := range composition.manifest.Roots {
		profile := filepath.Join(
			chart, "profiles", filepath.FromSlash(manifestRoot.RuntimePath),
		)
		profileSet[profile] = true
		compositionProfiles[manifestRoot.ID] = profile
	}
	smokeProfiles, err := meshProfiles(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, profile := range smokeProfiles {
		relative, err := filepath.Rel(filepath.Join(root, "agents"), profile)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(filepath.ToSlash(relative), "/tests/") {
			profileSet[profile] = true
		}
	}
	profiles := make([]string, 0, len(profileSet))
	for profile := range profileSet {
		profiles = append(profiles, profile)
	}
	sort.Strings(profiles)
	return profiles, compositionProfiles
}

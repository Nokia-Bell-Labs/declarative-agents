// Copyright (c) 2026 Nokia. All rights reserved.

package service

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// Scenario is one discovered test scenario: a tests/<name>/ directory beside
// the agent it exercises (srd018 R2.1).
type Scenario struct {
	Subject    string   `json:"subject"`
	SubjectDir string   `json:"subject_dir"`
	Name       string   `json:"name"`
	Dir        string   `json:"dir"`
	Validators []string `json:"validators"`
	Fixtures   []string `json:"fixtures"`

	// realized is set once the scenario's machine.yaml marker is seen; markers
	// under tests/<name>/ without one are not scenarios (srd018 R2.1).
	realized bool
}

const (
	testsDirName    = "tests"
	machineFileName = "machine.yaml"
	profileFileName = "profile.yaml"
	mocksDirName    = "mocks"
	// scenarioMaxDepth bounds the find scan to root/<subject>/tests/<name>/…
	// two levels deep (nested validator profiles and mocks fixtures).
	scenarioMaxDepth = "5"
)

// ListScenarios enumerates scenarios under the given roots. A root holds
// agent directories; each agent may carry a tests/ directory whose
// subdirectories are scenarios. Roots come from configuration or run input,
// never hardcoded paths (srd040 R5.4), so the same rig serves the agent
// families and the agents under applications/.
//
// Directory traversal is externalized to a single find per root
// (externalize-to-CLI-tools, #1384); Go only reduces the listed marker files
// into Scenario structs. A root that does not exist is skipped rather than
// failing, so a caller can declare optional roots. The result is sorted for
// determinism (R5.3).
func ListScenarios(roots []string) ([]Scenario, error) {
	byDir := map[string]*Scenario{}
	for _, root := range roots {
		info, err := os.Stat(root)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("discover scenarios in %s: %w", root, err)
		}
		if !info.IsDir() {
			continue
		}
		paths, err := findScenarioMarkers(root)
		if err != nil {
			return nil, err
		}
		reduceScenarioMarkers(root, paths, byDir)
	}
	return sortedScenarios(byDir), nil
}

// findScenarioMarkers lists every scenario marker file under root in one find:
// the machine.yaml that defines a scenario, the profile.yaml validators (its
// own and one per nested validator directory), and the mocks/ fixture files.
func findScenarioMarkers(root string) ([]string, error) {
	cmd := exec.Command("find", "-P", root,
		"-mindepth", "1", "-maxdepth", scenarioMaxDepth, "-type", "f",
		"(", "-name", machineFileName, "-o", "-name", profileFileName,
		"-o", "-path", "*/"+mocksDirName+"/*", ")")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("discover scenarios in %s: %w", root, err)
	}
	var paths []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line != "" {
			paths = append(paths, line)
		}
	}
	return paths, nil
}

// reduceScenarioMarkers groups marker paths into scenarios keyed by their
// tests/<name>/ directory. A scenario is realized only once its machine.yaml is
// seen; anything under tests/ without one is ignored (srd018 R2.1).
func reduceScenarioMarkers(root string, paths []string, byDir map[string]*Scenario) {
	for _, path := range paths {
		rel, ok := scenarioRelParts(root, path)
		if !ok {
			continue
		}
		subject, name, remainder := rel[0], rel[2], rel[3:]
		dir := filepath.Join(root, subject, testsDirName, name)
		scenario := byDir[dir]
		if scenario == nil {
			scenario = &Scenario{
				Subject: subject, SubjectDir: filepath.Join(root, subject),
				Name: name, Dir: dir,
			}
			byDir[dir] = scenario
		}
		classifyScenarioMarker(scenario, path, remainder)
	}
}

// scenarioRelParts splits a marker path into its root-relative components and
// reports whether it sits under <subject>/tests/<name>/, the only shape a
// scenario marker can take.
func scenarioRelParts(root, path string) ([]string, bool) {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return nil, false
	}
	parts := strings.Split(rel, string(filepath.Separator))
	if len(parts) < 4 || parts[1] != testsDirName {
		return nil, false
	}
	return parts, true
}

// classifyScenarioMarker records one marker against its scenario: the machine
// file realizes the scenario, a profile.yaml (own or one nested validator dir)
// is a validator, and a direct mocks/ file is a fixture.
func classifyScenarioMarker(scenario *Scenario, path string, remainder []string) {
	switch {
	case len(remainder) == 1 && remainder[0] == machineFileName:
		scenario.realized = true
	case len(remainder) == 1 && remainder[0] == profileFileName:
		scenario.Validators = append(scenario.Validators, path)
	case len(remainder) == 2 && remainder[1] == profileFileName && remainder[0] != mocksDirName:
		scenario.Validators = append(scenario.Validators, path)
	case len(remainder) == 2 && remainder[0] == mocksDirName:
		scenario.Fixtures = append(scenario.Fixtures, path)
	}
}

// sortedScenarios drops unrealized scenarios (no machine.yaml), sorts each
// scenario's validators and fixtures, and orders scenarios by subject then name.
func sortedScenarios(byDir map[string]*Scenario) []Scenario {
	var found []Scenario
	for _, scenario := range byDir {
		if !scenario.realized {
			continue
		}
		sort.Strings(scenario.Validators)
		sort.Strings(scenario.Fixtures)
		found = append(found, *scenario)
	}
	sort.Slice(found, func(i, j int) bool {
		if found[i].SubjectDir != found[j].SubjectDir {
			return found[i].SubjectDir < found[j].SubjectDir
		}
		return found[i].Name < found[j].Name
	})
	return found
}

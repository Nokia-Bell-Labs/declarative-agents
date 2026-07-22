// Copyright (c) 2026 Nokia. All rights reserved.

package service

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
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
}

const (
	testsDirName    = "tests"
	machineFileName = "machine.yaml"
	profileFileName = "profile.yaml"
	mocksDirName    = "mocks"
)

// ListScenarios enumerates scenarios under the given roots. A root holds
// agent directories; each agent may carry a tests/ directory whose
// subdirectories are scenarios. Roots come from configuration or run input,
// never hardcoded paths (srd040 R5.4), so the same rig serves the agent
// families and the agents under examples/.
//
// A root that does not exist is skipped rather than failing, so a caller can
// declare optional roots. The result is sorted for determinism (R5.3).
func ListScenarios(roots []string) ([]Scenario, error) {
	var found []Scenario
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
		scenarios, err := scenariosUnderRoot(root)
		if err != nil {
			return nil, err
		}
		found = append(found, scenarios...)
	}

	sort.Slice(found, func(i, j int) bool {
		if found[i].SubjectDir != found[j].SubjectDir {
			return found[i].SubjectDir < found[j].SubjectDir
		}
		return found[i].Name < found[j].Name
	})
	return found, nil
}

func scenariosUnderRoot(root string) ([]Scenario, error) {
	agents, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("read root %s: %w", root, err)
	}
	var found []Scenario
	for _, agent := range agents {
		if !agent.IsDir() {
			continue
		}
		subjectDir := filepath.Join(root, agent.Name())
		testsDir := filepath.Join(subjectDir, testsDirName)
		entries, err := os.ReadDir(testsDir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("read %s: %w", testsDir, err)
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			scenarioDir := filepath.Join(testsDir, entry.Name())
			// A scenario is a directory holding a validator machine; anything
			// else under tests/ is ignored rather than reported as a scenario.
			if _, err := os.Stat(filepath.Join(scenarioDir, machineFileName)); err != nil {
				continue
			}
			scenario, err := readScenario(agent.Name(), subjectDir, entry.Name(), scenarioDir)
			if err != nil {
				return nil, err
			}
			found = append(found, scenario)
		}
	}
	return found, nil
}

func readScenario(subject, subjectDir, name, dir string) (Scenario, error) {
	validators, err := scenarioValidators(dir)
	if err != nil {
		return Scenario{}, err
	}
	fixtures, err := scenarioFixtures(dir)
	if err != nil {
		return Scenario{}, err
	}
	return Scenario{
		Subject: subject, SubjectDir: subjectDir, Name: name, Dir: dir,
		Validators: validators, Fixtures: fixtures,
	}, nil
}

// scenarioValidators collects the scenario's validator profiles. The
// scenario's own profile.yaml binds its machine.yaml; each nested directory
// holding a profile is an additional validator, which is how a scenario
// declares several (srd018 R2.2).
func scenarioValidators(dir string) ([]string, error) {
	var validators []string
	if _, err := os.Stat(filepath.Join(dir, profileFileName)); err == nil {
		validators = append(validators, filepath.Join(dir, profileFileName))
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read scenario %s: %w", dir, err)
	}
	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == mocksDirName {
			continue
		}
		nested := filepath.Join(dir, entry.Name(), profileFileName)
		if _, err := os.Stat(nested); err == nil {
			validators = append(validators, nested)
		}
	}
	sort.Strings(validators)
	return validators, nil
}

// scenarioFixtures collects the twin fixture files a scenario mounts. A
// scenario without a mocks directory simply has none.
func scenarioFixtures(dir string) ([]string, error) {
	mocksDir := filepath.Join(dir, mocksDirName)
	entries, err := os.ReadDir(mocksDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read fixtures for %s: %w", dir, err)
	}
	var fixtures []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		fixtures = append(fixtures, filepath.Join(mocksDir, entry.Name()))
	}
	sort.Strings(fixtures)
	return fixtures, nil
}

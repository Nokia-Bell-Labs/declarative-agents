// Copyright (c) 2026 Nokia. All rights reserved.

package service

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

// The scenario session is the assembler's work list. It follows the critic's
// session shape: one state object holds the discovered work, a cursor over it,
// and the accumulated results, so the machine's words stay small and the
// per-scenario steps remain visible as machine transitions rather than
// collapsing into one opaque word (srd018 R1).

// ScenarioVerdict is one scenario's outcome.
type ScenarioVerdict struct {
	Subject    string             `json:"subject"`
	Scenario   string             `json:"scenario"`
	Passed     bool               `json:"passed"`
	Reason     string             `json:"reason,omitempty"`
	Validators []ValidatorOutcome `json:"validators,omitempty"`
}

// ScenarioManifest is the optional scenario.yaml a scenario may declare when
// convention is not enough. Every field is optional.
type ScenarioManifest struct {
	// SubjectProfile overrides the default of <subject-dir>/profile.yaml.
	SubjectProfile string `yaml:"subject_profile,omitempty"`
	// SubjectHealthPath is the path polled after the subject starts. A full
	// http URL is used as-is, for a subject whose health lives on a different
	// listener than the one the validator drives.
	SubjectHealthPath string `yaml:"subject_health_path,omitempty"`
	// SubjectAddress pins the subject's address instead of allocating one, for
	// a shipped subject that binds fixed ports (for example the mesh agents).
	// The published SUBJECT_URL and the health probe use it.
	SubjectAddress string `yaml:"subject_address,omitempty"`
	// SubjectRequest is passed to the subject as --request.
	SubjectRequest string `yaml:"subject_request,omitempty"`
	// Env adds literal environment entries to the subject.
	Env []string `yaml:"env,omitempty"`
	// FixtureEnv maps a fixture base name to the environment variable that
	// should carry that twin's base URL, overriding the derived name.
	FixtureEnv map[string]string `yaml:"fixture_env,omitempty"`
	// FixtureAddress pins a fixture's twin to a fixed address instead of an
	// allocated one, for a subject whose network limits allow only its real
	// dependency's port. The twin then stands exactly where the dependency
	// stands. Scenarios run sequentially, so a pinned port does not collide.
	FixtureAddress map[string]string `yaml:"fixture_address,omitempty"`
}

// runningTwin is one twin started for the current scenario.
type runningTwin struct {
	Fixture string `json:"fixture"`
	Service string `json:"service"`
	EnvVar  string `json:"env_var"`
	BaseURL string `json:"base_url"`
}

// ScenarioSessionState holds the assembler's discovered work and the state of
// the scenario currently being composed. Children started for a scenario are
// tracked so teardown can reap them even on a failure path (srd018 R1.5).
type ScenarioSessionState struct {
	mu sync.Mutex

	Services *State

	scenarios []Scenario
	cursor    int

	current  *Scenario
	manifest ScenarioManifest
	twins    []runningTwin
	subject  struct {
		service string
		baseURL string
	}
	validators []ValidatorOutcome

	verdicts []ScenarioVerdict
}

// NewScenarioSession returns an empty session sharing one service state, so
// every child started during the run is reachable for teardown.
func NewScenarioSession(services *State) *ScenarioSessionState {
	if services == nil {
		services = NewState()
	}
	return &ScenarioSessionState{Services: services}
}

// Seed discovers scenarios under the roots and resets the cursor.
func (s *ScenarioSessionState) Seed(roots []string) (int, error) {
	scenarios, err := ListScenarios(roots)
	if err != nil {
		return 0, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.scenarios = scenarios
	s.cursor = 0
	s.verdicts = nil
	return len(scenarios), nil
}

// Next advances the cursor and loads the next scenario's manifest. It reports
// false when the work list is exhausted.
func (s *ScenarioSessionState) Next() (Scenario, bool, error) {
	s.mu.Lock()
	if s.cursor >= len(s.scenarios) {
		s.mu.Unlock()
		return Scenario{}, false, nil
	}
	scenario := s.scenarios[s.cursor]
	s.cursor++
	s.mu.Unlock()

	manifest, err := loadScenarioManifest(scenario.Dir)
	if err != nil {
		return Scenario{}, false, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.current = &scenario
	s.manifest = manifest
	s.twins = nil
	s.validators = nil
	s.subject.service = ""
	s.subject.baseURL = ""
	return scenario, true, nil
}

// Current returns the scenario being composed.
func (s *ScenarioSessionState) Current() (Scenario, ScenarioManifest, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.current == nil {
		return Scenario{}, ScenarioManifest{}, false
	}
	return *s.current, s.manifest, true
}

// SubjectProfile resolves the profile of the agent under test: the manifest's
// override, or the subject directory's own profile.yaml.
func (s *ScenarioSessionState) SubjectProfile() (string, error) {
	current, manifest, ok := s.Current()
	if !ok {
		return "", fmt.Errorf("no current scenario")
	}
	if manifest.SubjectProfile != "" {
		if filepath.IsAbs(manifest.SubjectProfile) {
			return manifest.SubjectProfile, nil
		}
		return filepath.Join(current.Dir, manifest.SubjectProfile), nil
	}
	return filepath.Join(current.SubjectDir, profileFileName), nil
}

// RecordTwin remembers a started twin and the environment variable that
// carries its base URL to the subject.
func (s *ScenarioSessionState) RecordTwin(twin runningTwin) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.twins = append(s.twins, twin)
}

// Twins returns the twins started for the current scenario.
func (s *ScenarioSessionState) Twins() []runningTwin {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]runningTwin, len(s.twins))
	copy(out, s.twins)
	return out
}

// SubjectEnv builds the subject's environment additions from the twins started
// for this scenario. This is the dependency redirection: a twin's base URL is
// only known once it binds a port, and it reaches the subject here rather than
// through static config (srd018 R4.1).
func (s *ScenarioSessionState) SubjectEnv() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	env := make([]string, 0, len(s.twins)+len(s.manifest.Env))
	for _, twin := range s.twins {
		env = append(env, twin.EnvVar+"="+twin.BaseURL)
	}
	env = append(env, s.manifest.Env...)
	return env
}

// RecordSubject remembers the started subject.
func (s *ScenarioSessionState) RecordSubject(service, baseURL string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.subject.service = service
	s.subject.baseURL = baseURL
}

// Subject returns the started subject's service name and base URL.
func (s *ScenarioSessionState) Subject() (string, string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.subject.service, s.subject.baseURL
}

// RecordValidators stores the current scenario's validator outcomes.
func (s *ScenarioSessionState) RecordValidators(outcomes []ValidatorOutcome) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.validators = outcomes
}

// CollectVerdict derives and stores the current scenario's verdict. A scenario
// passes only when every validator in it passed (srd018 R6.1), and a failure
// names its cause (R6.2).
func (s *ScenarioSessionState) CollectVerdict(reasonOverride string) ScenarioVerdict {
	s.mu.Lock()
	current := s.current
	outcomes := s.validators
	s.mu.Unlock()

	verdict := ScenarioVerdict{Validators: outcomes}
	if current != nil {
		verdict.Subject = current.Subject
		verdict.Scenario = current.Name
	}
	switch {
	case reasonOverride != "":
		verdict.Passed = false
		verdict.Reason = reasonOverride
	case len(outcomes) == 0:
		verdict.Passed = false
		verdict.Reason = "no validator ran"
	case AllPassed(outcomes):
		verdict.Passed = true
	default:
		failure, _ := FirstFailure(outcomes)
		verdict.Passed = false
		verdict.Reason = failureReason(failure)
	}

	s.mu.Lock()
	s.verdicts = append(s.verdicts, verdict)
	s.mu.Unlock()
	return verdict
}

// Verdicts returns every scenario verdict collected so far.
func (s *ScenarioSessionState) Verdicts() []ScenarioVerdict {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]ScenarioVerdict, len(s.verdicts))
	copy(out, s.verdicts)
	return out
}

// Report reduces the collected verdicts into the run's aggregate result.
func (s *ScenarioSessionState) Report() map[string]interface{} {
	verdicts := s.Verdicts()
	passed := len(verdicts) > 0
	failed := make([]ScenarioVerdict, 0, len(verdicts))
	for _, verdict := range verdicts {
		if !verdict.Passed {
			passed = false
			failed = append(failed, verdict)
		}
	}
	report := map[string]interface{}{
		"scenarios": len(verdicts),
		"passed":    passed,
		"verdicts":  verdicts,
	}
	if len(failed) > 0 {
		report["failed"] = failed
		report["first_failure"] = failed[0]
	}
	return report
}

func failureReason(outcome ValidatorOutcome) string {
	switch {
	case outcome.TimedOut:
		return fmt.Sprintf("validator %q timed out", outcome.Name)
	case outcome.Error != "":
		return fmt.Sprintf("validator %q failed to run: %s", outcome.Name, outcome.Error)
	default:
		return fmt.Sprintf("validator %q exited %d", outcome.Name, outcome.ExitCode)
	}
}

func loadScenarioManifest(dir string) (ScenarioManifest, error) {
	path := filepath.Join(dir, scenarioManifestName)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return ScenarioManifest{}, nil
		}
		return ScenarioManifest{}, fmt.Errorf("read %s: %w", path, err)
	}
	var manifest ScenarioManifest
	if err := yaml.Unmarshal(data, &manifest); err != nil {
		return ScenarioManifest{}, fmt.Errorf("parse %s: %w", path, err)
	}
	return manifest, nil
}

const scenarioManifestName = "scenario.yaml"

var nonEnvChars = regexp.MustCompile(`[^A-Za-z0-9]+`)

// fixtureEnvVar derives the environment variable a fixture's twin URL is
// published as: mocks/chroma.yaml becomes CHROMA_URL. A subject declares its
// dependency as base_url: ${CHROMA_URL:-...}, so the fixture name and the
// subject's declared variable line up without a manifest. A scenario that
// needs a different name declares fixture_env.
func fixtureEnvVar(fixturePath string, overrides map[string]string) string {
	base := fixtureBase(fixturePath)
	if override, ok := overrides[base]; ok && override != "" {
		return override
	}
	name := strings.ToUpper(nonEnvChars.ReplaceAllString(base, "_"))
	name = strings.Trim(name, "_")
	if name == "" {
		name = "TWIN"
	}
	return name + "_URL"
}

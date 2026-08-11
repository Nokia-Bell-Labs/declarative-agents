// Copyright (c) 2026 Nokia. All rights reserved.

package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"gopkg.in/yaml.v3"
)

// These are the scenario critic's per-scenario steps. Each reads the current
// scenario from the session rather than static config, which is what lets the
// pipeline stay visible as machine transitions while still working on data
// discovered at runtime.

const (
	defaultSubjectHealthPath = "/healthz"
	defaultMockAddressEnv    = "MOCK_ADDRESS"
	defaultSubjectAddressEnv = "SUBJECT_ADDRESS"
)

// addressEnvName resolves the variable a child reads to learn the address the
// rig allocated to it.
func addressEnvName(declared, fallback string) string {
	if declared != "" {
		return declared
	}
	return fallback
}

// SeedSession discovers scenarios and resets the cursor.
func (c command) initSession() core.Result {
	count, err := c.session.Seed(c.cfg.Roots)
	if err != nil {
		return commandError(c.toolName, err)
	}
	signal := SignalSessionSeeded
	if count == 0 {
		signal = SignalNoScenarios
	}
	return core.Result{
		Signal: signal, CommandName: c.toolName,
		Output: jsonOutput(map[string]interface{}{"scenarios": count, "roots": c.cfg.Roots}),
	}
}

// nextScenario advances the work list, mirroring the critic's next_point.
func (c command) nextScenario() core.Result {
	scenario, ok, err := c.session.Next()
	if err != nil {
		return commandError(c.toolName, err)
	}
	if !ok {
		return core.Result{
			Signal: SignalAllScenariosDone, CommandName: c.toolName,
			Output: jsonOutput(map[string]interface{}{"exhausted": true}),
		}
	}
	return core.Result{
		Signal: SignalScenarioReady, CommandName: c.toolName,
		Output: jsonOutput(map[string]interface{}{
			"subject": scenario.Subject, "scenario": scenario.Name,
			"validators": validatorItems(scenario.Validators),
			"fixtures":   fixtureItems(scenario.Fixtures),
		}),
	}
}

func fixtureItems(fixtures []string) []map[string]string {
	items := make([]map[string]string, 0, len(fixtures))
	for _, fixture := range fixtures {
		items = append(items, map[string]string{"path": fixture})
	}
	return items
}

func validatorItems(profiles []string) []map[string]string {
	items := make([]map[string]string, 0, len(profiles))
	for _, profile := range profiles {
		items = append(items, map[string]string{
			"name": validatorName(profile), "profile": profile,
		})
	}
	return items
}

// startScenarioMock starts the one mock bound by MachineSpec for_each.
func (c command) startScenarioMock() core.Result {
	scenario, manifest, ok := c.session.Current()
	if !ok {
		return commandError(c.toolName, fmt.Errorf("%s: no current scenario", c.toolName))
	}
	if c.cfg.Profile == "" {
		return commandError(c.toolName, fmt.Errorf("%s: requires the mock profile", c.toolName))
	}
	fixtureValue, err := core.ResolveFromSelector(c.commandState, c.cfg.Fixture)
	if err != nil {
		return commandError(c.toolName, err)
	}
	fixture, ok := fixtureValue.(string)
	if !ok || fixture == "" {
		return commandError(c.toolName, fmt.Errorf("%s: fixture selector did not resolve to a string", c.toolName))
	}
	mock, err := c.startOneMock(scenario, manifest, fixture)
	if err != nil {
		return commandError(c.toolName, err)
	}
	c.session.RecordMock(mock)
	return core.Result{
		Signal: SignalMockStarted, CommandName: c.toolName,
		Output:  jsonOutput(map[string]interface{}{"mock": mock}),
		Receipt: jsonOutput(map[string]interface{}{"service": mock.Service}),
	}
}

// startOneMock starts a single mock for one fixture, telling it which address
// to bind and which fixture to serve.
func (c command) startOneMock(scenario Scenario, manifest ScenarioManifest, fixture string) (runningMock, error) {
	name := mockServiceName(scenario, fixture)
	address := manifest.FixtureAddress[fixtureBase(fixture)]
	if address == "" {
		free, err := FreeAddress()
		if err != nil {
			return runningMock{}, err
		}
		address = free
	}
	env := append([]string{
		addressEnvName(c.cfg.AddressEnv, defaultMockAddressEnv) + "=" + address,
		"MOCK_FIXTURES=" + fixture,
	}, c.cfg.Env...)

	out, err := c.session.Services.Start(StartSpec{
		Name: name, Binary: c.cfg.Binary, Profile: c.cfg.Profile,
		CoreRoot: c.coreRoot, Directory: c.cfg.Directory, Address: address, Env: env,
	})
	if err != nil {
		return runningMock{}, err
	}
	baseURL, ok := out["base_url"].(string)
	if !ok || baseURL == "" {
		c.session.Services.Stop(name, defaultStopGrace)
		return runningMock{}, fmt.Errorf("started mock returned no base_url")
	}
	return runningMock{
		Fixture: fixture, Service: name,
		EnvVar:  fixtureEnvVar(fixture, manifest.FixtureEnv),
		BaseURL: baseURL,
	}, nil
}

// startSubject starts the agent under test with every mock's base URL injected
// into its environment, so its declared ${VAR:-default} base_url resolves at
// the mock rather than a live service.
func (c command) startSubject() core.Result {
	scenario, _, ok := c.session.Current()
	if !ok {
		return commandError(c.toolName, fmt.Errorf("%s: no current scenario", c.toolName))
	}
	profile, err := c.session.SubjectProfile()
	if err != nil {
		return commandError(c.toolName, err)
	}
	_, manifest, _ := c.session.Current()

	address, err := subjectAddress(manifest)
	if err != nil {
		return commandError(c.toolName, err)
	}
	healthRoute, err := parseSubjectHealthRoute(manifest.SubjectHealthPath)
	if err != nil {
		return commandError(c.toolName, err)
	}
	name := subjectServiceName(scenario)
	env := append([]string{
		addressEnvName(c.cfg.AddressEnv, defaultSubjectAddressEnv) + "=" + address,
	}, c.session.SubjectEnv()...)
	env = append(env, c.cfg.Env...)

	out, err := c.session.Services.Start(StartSpec{
		Name: name, Binary: c.cfg.Binary, Profile: profile, Address: address,
		CoreRoot: c.coreRoot, Directory: c.cfg.Directory, Request: manifest.SubjectRequest, Env: env,
	})
	if err != nil {
		return commandError(c.toolName, err)
	}
	return c.recordStartedSubject(name, profile, env, healthRoute, out)
}

func (c command) recordStartedSubject(
	name, profile string,
	env []string,
	healthRoute subjectHealthRoute,
	out map[string]interface{},
) core.Result {
	baseURL, ok := out["base_url"].(string)
	if !ok || baseURL == "" {
		c.session.Services.Stop(name, defaultStopGrace)
		return commandError(c.toolName, fmt.Errorf("started subject returned no base_url"))
	}
	healthBaseURL, healthPath := healthRoute.resolve(baseURL)
	c.session.RecordSubject(name, baseURL)

	return core.Result{
		Signal: SignalSubjectStarted, CommandName: c.toolName,
		Output: jsonOutput(map[string]interface{}{
			"subject": name, "profile": profile, "base_url": baseURL, "env": env,
			"health_base_url": healthBaseURL, "health_path": healthPath,
			"started_at": out["started_at"],
		}),
		Receipt: jsonOutput(map[string]interface{}{"service": name}),
	}
}

type subjectHealthRoute struct {
	baseURL string
	path    string
}

func (r subjectHealthRoute) resolve(subjectBaseURL string) (string, string) {
	if r.baseURL != "" {
		return r.baseURL, r.path
	}
	return subjectBaseURL, r.path
}

// subjectAddress resolves where the subject binds: the manifest's pinned
// address for a subject that ships fixed ports, or a freshly reserved one.
func subjectAddress(manifest ScenarioManifest) (string, error) {
	if manifest.SubjectAddress != "" {
		return manifest.SubjectAddress, nil
	}
	return FreeAddress()
}

// subjectHealthTarget separates the trusted runtime authority from the
// scenario-authored path so the REST operation can select both from the
// labeled subject-start result without accepting a caller-supplied URL.
func subjectHealthTarget(baseURL, declared string) (string, string, error) {
	route, err := parseSubjectHealthRoute(declared)
	if err != nil {
		return "", "", err
	}
	targetBaseURL, targetPath := route.resolve(baseURL)
	return targetBaseURL, targetPath, nil
}

func parseSubjectHealthRoute(declared string) (subjectHealthRoute, error) {
	if declared == "" {
		declared = defaultSubjectHealthPath
	}
	target, err := url.Parse(declared)
	if err != nil {
		return subjectHealthRoute{}, fmt.Errorf("subject health path %q: %w", declared, err)
	}
	if target.IsAbs() {
		if target.Scheme != "http" && target.Scheme != "https" {
			return subjectHealthRoute{}, fmt.Errorf("subject health URL scheme %q is not allowed", target.Scheme)
		}
		if target.Host == "" || target.User != nil {
			return subjectHealthRoute{}, fmt.Errorf("subject health URL must have a host and no user information")
		}
		return subjectHealthRoute{
			baseURL: target.Scheme + "://" + target.Host,
			path:    strings.TrimPrefix(target.EscapedPath(), "/"),
		}, nil
	}
	return subjectHealthRoute{path: strings.TrimPrefix(declared, "/")}, nil
}

// runScenarioValidator runs the one validator bound by MachineSpec for_each.
func (c command) runScenarioValidator(ctx context.Context) core.Result {
	profile, name, err := c.validatorBinding()
	if err != nil {
		return commandError(c.toolName, err)
	}
	_, subjectURL := c.session.Subject()
	if subjectURL == "" {
		return commandError(c.toolName, fmt.Errorf("%s: no subject started", c.toolName))
	}
	env := append([]string{"SUBJECT_URL=" + subjectURL}, c.session.SubjectEnv()...)
	outcome := runOneValidator(ctx, c.cfg.Binary, ValidatorSpec{
		Name: name, Profile: profile, CoreRoot: c.coreRoot, Directory: c.cfg.Directory,
		OTLPEndpoint: c.cfg.OTLPEndpoint, Env: append(env, c.cfg.Env...),
	}, parseDuration(c.cfg.Timeout, defaultRunTimeout))
	signal := SignalValidatorCompleted
	if outcome.TimedOut || outcome.Error != "" {
		signal = SignalValidatorIncomplete
	}
	return core.Result{
		Signal: signal, CommandName: c.toolName,
		Output: jsonOutput(outcome),
	}
}

func (c command) validatorBinding() (string, string, error) {
	value, err := core.ResolveFromSelector(c.commandState, c.cfg.Validator)
	if err != nil {
		return "", "", err
	}
	profile, ok := value.(string)
	if !ok || profile == "" {
		return "", "", fmt.Errorf("%s: validator selector did not resolve to a string", c.toolName)
	}
	return profile, validatorName(profile), nil
}

// recordScenarioValidators stores the ordered outcomes from the visible join.
func (c command) recordScenarioValidators() core.Result {
	value, err := core.ResolveFromSelector(c.commandState, c.cfg.Outcomes)
	if err != nil {
		return commandError(c.toolName, err)
	}
	items, ok := value.([]interface{})
	if !ok {
		return commandError(c.toolName, fmt.Errorf("%s: outcomes selector did not resolve to an array", c.toolName))
	}
	outcomes, err := decodeValidatorOutcomes(items)
	if err != nil {
		return commandError(c.toolName, err)
	}
	c.session.RecordValidators(outcomes)
	return core.Result{
		Signal: SignalValidatorsRecorded, CommandName: c.toolName,
		Output: jsonOutput(map[string]interface{}{"validators": outcomes, "passed": AllPassed(outcomes)}),
	}
}

func decodeValidatorOutcomes(items []interface{}) ([]ValidatorOutcome, error) {
	outcomes := make([]ValidatorOutcome, 0, len(items))
	for index, item := range items {
		data, err := json.Marshal(item)
		if err != nil {
			return nil, fmt.Errorf("validator outcome %d: %w", index, err)
		}
		var joined struct {
			Result struct {
				Output string `json:"output"`
			} `json:"result"`
		}
		if err := json.Unmarshal(data, &joined); err != nil {
			return nil, fmt.Errorf("validator outcome %d: %w", index, err)
		}
		var outcome ValidatorOutcome
		if err := json.Unmarshal([]byte(joined.Result.Output), &outcome); err != nil {
			return nil, fmt.Errorf("validator outcome %d output: %w", index, err)
		}
		outcomes = append(outcomes, outcome)
	}
	return outcomes, nil
}

// collectVerdict derives this scenario's verdict from its validator outcomes.
func (c command) collectVerdict() core.Result {
	verdict := c.session.CollectVerdict(c.cfg.Reason)
	signal := SignalScenarioPassed
	if !verdict.Passed {
		signal = SignalScenarioFailed
	}
	return core.Result{Signal: signal, CommandName: c.toolName, Output: jsonOutput(verdict)}
}

// listScenarioChildren exposes subject-first teardown items to MachineSpec.
func (c command) listScenarioChildren() core.Result {
	if _, _, ok := c.session.Current(); !ok {
		return commandError(c.toolName, fmt.Errorf("%s: no current scenario", c.toolName))
	}
	return core.Result{
		Signal: SignalScenarioChildrenListed, CommandName: c.toolName,
		Output: jsonOutput(map[string]interface{}{"children": c.session.Children()}),
	}
}

// reportSession reduces every scenario verdict into the run's result.
func (c command) reportSession() core.Result {
	report := c.session.Report()
	signal := SignalSessionPassed
	if passed, _ := report["passed"].(bool); !passed {
		signal = SignalSessionFailed
	}
	return core.Result{Signal: signal, CommandName: c.toolName, Output: jsonOutput(report)}
}

func mockServiceName(scenario Scenario, fixture string) string {
	base := strings.TrimSuffix(filepath.Base(fixture), filepath.Ext(fixture))
	return fmt.Sprintf("mock-%s-%s-%s", scenario.Subject, scenario.Name, base)
}

func subjectServiceName(scenario Scenario) string {
	return fmt.Sprintf("subject-%s-%s", scenario.Subject, scenario.Name)
}

// fixtureBase names a fixture by its file base without extension, the key the
// manifest's fixture_env and fixture_address maps use.
func fixtureBase(fixturePath string) string {
	return strings.TrimSuffix(filepath.Base(fixturePath), filepath.Ext(fixturePath))
}

// validatorName labels a validator by its declared profile identity, so runtime
// verdicts name the concrete Critic realization while the directory keeps the
// scenario scope. Invalid fixture profiles retain the path-based fallback.
func validatorName(profilePath string) string {
	data, err := os.ReadFile(profilePath)
	if err == nil {
		var profile struct {
			Name string `yaml:"name"`
		}
		if yaml.Unmarshal(data, &profile) == nil && profile.Name != "" {
			return profile.Name
		}
	}
	dir := filepath.Dir(profilePath)
	return filepath.Base(dir)
}

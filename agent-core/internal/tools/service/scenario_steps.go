// Copyright (c) 2026 Nokia. All rights reserved.

package service

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

// These are the assembler's per-scenario steps. Each reads the current
// scenario from the session rather than static config, which is what lets the
// pipeline stay visible as machine transitions while still working on data
// discovered at runtime.

const (
	defaultSubjectHealthPath = "/healthz"
	defaultTwinAddressEnv    = "TWIN_ADDRESS"
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
			Output: jsonOutput(c.session.Report()),
		}
	}
	return core.Result{
		Signal: SignalScenarioReady, CommandName: c.toolName,
		Output: jsonOutput(map[string]interface{}{
			"subject": scenario.Subject, "scenario": scenario.Name,
			"validators": len(scenario.Validators), "fixtures": len(scenario.Fixtures),
		}),
	}
}

// startTwins starts one twin per fixture the scenario declares, recording each
// twin's runtime base URL against the environment variable the subject reads.
func (c command) startTwins() core.Result {
	scenario, manifest, ok := c.session.Current()
	if !ok {
		return commandError(c.toolName, fmt.Errorf("%s: no current scenario", c.toolName))
	}
	if c.cfg.Profile == "" {
		return commandError(c.toolName, fmt.Errorf("%s: requires the twin profile", c.toolName))
	}

	started := make([]runningTwin, 0, len(scenario.Fixtures))
	for _, fixture := range scenario.Fixtures {
		twin, err := c.startOneTwin(scenario, manifest, fixture)
		if err != nil {
			// Leave teardown to the machine's failure edge; children already
			// started stay tracked in the shared service state.
			return commandError(c.toolName, err)
		}
		c.session.RecordTwin(twin)
		started = append(started, twin)
	}

	return core.Result{
		Signal: SignalTwinsStarted, CommandName: c.toolName,
		Output: jsonOutput(map[string]interface{}{"twins": started}),
	}
}

// startOneTwin starts a single twin for one fixture, telling it which address
// to bind and which fixture to serve.
func (c command) startOneTwin(scenario Scenario, manifest ScenarioManifest, fixture string) (runningTwin, error) {
	name := twinServiceName(scenario, fixture)
	address := manifest.FixtureAddress[fixtureBase(fixture)]
	if address == "" {
		free, err := FreeAddress()
		if err != nil {
			return runningTwin{}, err
		}
		address = free
	}
	env := append([]string{
		addressEnvName(c.cfg.AddressEnv, defaultTwinAddressEnv) + "=" + address,
		"TWIN_FIXTURES=" + fixture,
	}, c.cfg.Env...)

	out, err := c.session.Services.Start(StartSpec{
		Name: name, Binary: c.cfg.Binary, Profile: c.cfg.Profile,
		Directory: c.cfg.Directory, Address: address, Env: env,
	})
	if err != nil {
		return runningTwin{}, err
	}
	return runningTwin{
		Fixture: fixture, Service: name,
		EnvVar:  fixtureEnvVar(fixture, manifest.FixtureEnv),
		BaseURL: out["base_url"].(string),
	}, nil
}

// startSubject starts the agent under test with every twin's base URL injected
// into its environment, so its declared ${VAR:-default} base_url resolves at
// the twin rather than a live service.
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
	name := subjectServiceName(scenario)
	env := append([]string{
		addressEnvName(c.cfg.AddressEnv, defaultSubjectAddressEnv) + "=" + address,
	}, c.session.SubjectEnv()...)
	env = append(env, c.cfg.Env...)

	out, err := c.session.Services.Start(StartSpec{
		Name: name, Binary: c.cfg.Binary, Profile: profile, Address: address,
		Directory: c.cfg.Directory, Request: manifest.SubjectRequest, Env: env,
	})
	if err != nil {
		return commandError(c.toolName, err)
	}
	baseURL := out["base_url"].(string)
	c.session.RecordSubject(name, baseURL)

	return core.Result{
		Signal: SignalSubjectStarted, CommandName: c.toolName,
		Output: jsonOutput(map[string]interface{}{
			"subject": name, "profile": profile, "base_url": baseURL, "env": env,
		}),
	}
}

// subjectAddress resolves where the subject binds: the manifest's pinned
// address for a subject that ships fixed ports, or a freshly reserved one.
func subjectAddress(manifest ScenarioManifest) (string, error) {
	if manifest.SubjectAddress != "" {
		return manifest.SubjectAddress, nil
	}
	return FreeAddress()
}

// awaitSubject health-checks the subject that this scenario started.
func (c command) awaitSubject() core.Result {
	_, baseURL := c.session.Subject()
	if baseURL == "" {
		return commandError(c.toolName, fmt.Errorf("%s: no subject started", c.toolName))
	}
	_, manifest, _ := c.session.Current()
	path := manifest.SubjectHealthPath
	if path == "" {
		path = c.cfg.URL
	}
	if path == "" {
		path = defaultSubjectHealthPath
	}
	target := baseURL + path
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		// Health on a different listener than the driven one.
		target = path
	}

	output, healthy := c.session.Services.AwaitHealthy(
		target,
		parseDuration(c.cfg.Timeout, defaultHealthTimeout),
		parseDuration(c.cfg.Interval, defaultHealthInterval),
	)
	signal := SignalHealthy
	if !healthy {
		signal = SignalHealthTimeout
	}
	return core.Result{Signal: signal, CommandName: c.toolName, Output: jsonOutput(output)}
}

// runScenarioValidators runs the current scenario's validators concurrently
// against its single subject instance.
func (c command) runScenarioValidators(ctx context.Context) core.Result {
	scenario, _, ok := c.session.Current()
	if !ok {
		return commandError(c.toolName, fmt.Errorf("%s: no current scenario", c.toolName))
	}
	if len(scenario.Validators) == 0 {
		return commandError(c.toolName, fmt.Errorf("%s: scenario %q declares no validator", c.toolName, scenario.Name))
	}
	_, subjectURL := c.session.Subject()

	specs := make([]ValidatorSpec, 0, len(scenario.Validators))
	for _, profile := range scenario.Validators {
		env := append([]string{"SUBJECT_URL=" + subjectURL}, c.session.SubjectEnv()...)
		specs = append(specs, ValidatorSpec{
			Name:      validatorName(profile),
			Profile:   profile,
			Directory: c.cfg.Directory,
			Env:       append(env, c.cfg.Env...),
		})
	}

	outcomes := RunValidators(ctx, c.cfg.Binary, specs, parseDuration(c.cfg.Timeout, defaultRunTimeout))
	c.session.RecordValidators(outcomes)

	signal := SignalValidatorsCompleted
	if failure, failed := FirstFailure(outcomes); failed && (failure.TimedOut || failure.Error != "") {
		signal = SignalValidatorsIncomplete
	}
	return core.Result{
		Signal: signal, CommandName: c.toolName,
		Output: jsonOutput(map[string]interface{}{"validators": outcomes, "passed": AllPassed(outcomes)}),
	}
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

// teardownScenario stops every child started for the current scenario. It runs
// on the success path and on every failure edge, so no process outlives a
// scenario (srd018 R1.5).
func (c command) teardownScenario() core.Result {
	grace := parseDuration(c.cfg.Grace, defaultStopGrace)
	stopped := make([]map[string]interface{}, 0)

	if name, _ := c.session.Subject(); name != "" {
		stopped = append(stopped, c.session.Services.Stop(name, grace))
	}
	for _, twin := range c.session.Twins() {
		stopped = append(stopped, c.session.Services.Stop(twin.Service, grace))
	}

	return core.Result{
		Signal: SignalScenarioTornDown, CommandName: c.toolName,
		Output: jsonOutput(map[string]interface{}{"stopped": stopped, "running": c.session.Services.Running()}),
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

func twinServiceName(scenario Scenario, fixture string) string {
	base := strings.TrimSuffix(filepath.Base(fixture), filepath.Ext(fixture))
	return fmt.Sprintf("twin-%s-%s-%s", scenario.Subject, scenario.Name, base)
}

func subjectServiceName(scenario Scenario) string {
	return fmt.Sprintf("subject-%s-%s", scenario.Subject, scenario.Name)
}

// fixtureBase names a fixture by its file base without extension, the key the
// manifest's fixture_env and fixture_address maps use.
func fixtureBase(fixturePath string) string {
	return strings.TrimSuffix(filepath.Base(fixturePath), filepath.Ext(fixturePath))
}

// validatorName labels a validator by its scenario-relative directory, so a
// verdict names which validator failed rather than a bare path.
func validatorName(profilePath string) string {
	dir := filepath.Dir(profilePath)
	return filepath.Base(dir)
}

// Copyright (c) 2026 Nokia. All rights reserved.

package service

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

// scenarioTree builds a fixture tree of subjects and scenarios and returns its
// root. Each scenario gets a validator profile; fixtures are optional.
func scenarioTree(t *testing.T, subjects map[string]map[string][]string) string {
	t.Helper()
	root := t.TempDir()
	for subject, scenarios := range subjects {
		require.NoError(t, os.MkdirAll(filepath.Join(root, subject), 0o755))
		require.NoError(t, os.WriteFile(
			filepath.Join(root, subject, profileFileName), []byte("name: "+subject+"\n"), 0o644))
		for scenario, fixtures := range scenarios {
			dir := filepath.Join(root, subject, testsDirName, scenario)
			require.NoError(t, os.MkdirAll(dir, 0o755))
			require.NoError(t, os.WriteFile(filepath.Join(dir, machineFileName), []byte("{}"), 0o644))
			require.NoError(t, os.WriteFile(filepath.Join(dir, profileFileName), []byte("{}"), 0o644))
			for _, fixture := range fixtures {
				mocks := filepath.Join(dir, mocksDirName)
				require.NoError(t, os.MkdirAll(mocks, 0o755))
				require.NoError(t, os.WriteFile(filepath.Join(mocks, fixture), []byte("routes: []\n"), 0o644))
			}
		}
	}
	return root
}

// TestScenarioSession_IteratesEachScenarioOnce covers the cursor contract: the
// session walks every discovered scenario exactly once, then reports done.
func TestScenarioSession_IteratesEachScenarioOnce(t *testing.T) {
	t.Parallel()

	root := scenarioTree(t, map[string]map[string][]string{
		"alpha": {"happy": {"dep.yaml"}, "failure": nil},
		"beta":  {"only": nil},
	})

	session := NewScenarioSession(NewState())
	count, err := session.Seed([]string{root})
	require.NoError(t, err)
	require.Equal(t, 3, count)

	var seen []string
	for {
		scenario, ok, err := session.Next()
		require.NoError(t, err)
		if !ok {
			break
		}
		seen = append(seen, scenario.Subject+"/"+scenario.Name)
	}
	require.Equal(t, []string{"alpha/failure", "alpha/happy", "beta/only"}, seen,
		"every scenario is visited exactly once, in deterministic order")

	// Exhausted means exhausted: a further Next stays done.
	_, ok, err := session.Next()
	require.NoError(t, err)
	require.False(t, ok)
}

// TestScenarioSession_SubjectProfileAndManifest covers subject resolution: the
// subject directory's own profile by default, and the manifest's override when
// a scenario declares one.
func TestScenarioSession_SubjectProfileAndManifest(t *testing.T) {
	t.Parallel()

	root := scenarioTree(t, map[string]map[string][]string{"alpha": {"happy": nil}})
	session := NewScenarioSession(NewState())
	_, err := session.Seed([]string{root})
	require.NoError(t, err)
	_, ok, err := session.Next()
	require.NoError(t, err)
	require.True(t, ok)

	profile, err := session.SubjectProfile()
	require.NoError(t, err)
	require.Equal(t, filepath.Join(root, "alpha", profileFileName), profile)

	// A manifest overrides the subject profile and carries health/request.
	manifest := "subject_profile: custom/profile.yaml\nsubject_health_path: /ready\nsubject_request: req.yaml\n"
	require.NoError(t, os.WriteFile(
		filepath.Join(root, "alpha", testsDirName, "happy", scenarioManifestName), []byte(manifest), 0o644))

	session2 := NewScenarioSession(NewState())
	_, err = session2.Seed([]string{root})
	require.NoError(t, err)
	scenario, ok, err := session2.Next()
	require.NoError(t, err)
	require.True(t, ok)

	profile, err = session2.SubjectProfile()
	require.NoError(t, err)
	require.Equal(t, filepath.Join(scenario.Dir, "custom", "profile.yaml"), profile)

	_, loaded, _ := session2.Current()
	require.Equal(t, "/ready", loaded.SubjectHealthPath)
	require.Equal(t, "req.yaml", loaded.SubjectRequest)
}

// TestFixtureEnvVar covers the convention that lets a fixture name line up with
// the variable a subject declares, plus the manifest override.
func TestFixtureEnvVar(t *testing.T) {
	t.Parallel()

	require.Equal(t, "CHROMA_URL", fixtureEnvVar("/x/mocks/chroma.yaml", nil))
	require.Equal(t, "RAG_SERVER_URL", fixtureEnvVar("/x/mocks/rag-server.yaml", nil))
	require.Equal(t, "DEP_URL", fixtureEnvVar("mocks/dep.yml", nil))
	require.Equal(t, "CUSTOM", fixtureEnvVar("/x/mocks/chroma.yaml", map[string]string{"chroma": "CUSTOM"}))
}

// TestScenarioSession_VerdictNamesItsCause covers the verdict contract: a
// scenario passes only when every validator passed, and a failure names which
// validator failed and why.
func TestScenarioSession_VerdictNamesItsCause(t *testing.T) {
	t.Parallel()

	root := scenarioTree(t, map[string]map[string][]string{"alpha": {"a": nil, "b": nil, "c": nil}})
	session := NewScenarioSession(NewState())
	_, err := session.Seed([]string{root})
	require.NoError(t, err)

	// Scenario a: every validator passed.
	_, _, err = session.Next()
	require.NoError(t, err)
	session.RecordValidators([]ValidatorOutcome{{Name: "v1", Passed: true}, {Name: "v2", Passed: true}})
	passed := session.CollectVerdict("")
	require.True(t, passed.Passed)
	require.Equal(t, "a", passed.Scenario)

	// Scenario b: one validator exited non-zero.
	_, _, err = session.Next()
	require.NoError(t, err)
	session.RecordValidators([]ValidatorOutcome{
		{Name: "ok", Passed: true},
		{Name: "broken", Passed: false, ExitCode: 2},
	})
	failed := session.CollectVerdict("")
	require.False(t, failed.Passed)
	require.Contains(t, failed.Reason, "broken")
	require.Contains(t, failed.Reason, "exited 2")

	// Scenario c: a step before the validators failed, so the reason is forced.
	_, _, err = session.Next()
	require.NoError(t, err)
	forced := session.CollectVerdict("subject never became healthy")
	require.False(t, forced.Passed)
	require.Equal(t, "subject never became healthy", forced.Reason)

	report := session.Report()
	require.Equal(t, 3, report["scenarios"])
	require.Equal(t, false, report["passed"])
	first, ok := report["first_failure"].(ScenarioVerdict)
	require.True(t, ok)
	require.Equal(t, "b", first.Scenario, "the report names the first failing scenario")
}

// TestScenarioSession_ReportPassesOnlyWhenAllPass covers the aggregate.
func TestScenarioSession_ReportPassesOnlyWhenAllPass(t *testing.T) {
	t.Parallel()

	root := scenarioTree(t, map[string]map[string][]string{"alpha": {"a": nil, "b": nil}})
	session := NewScenarioSession(NewState())
	_, err := session.Seed([]string{root})
	require.NoError(t, err)
	for i := 0; i < 2; i++ {
		_, _, err = session.Next()
		require.NoError(t, err)
		session.RecordValidators([]ValidatorOutcome{{Name: "v", Passed: true}})
		session.CollectVerdict("")
	}
	report := session.Report()
	require.Equal(t, true, report["passed"])
	require.NotContains(t, report, "first_failure")

	// An empty run does not pass: nothing was proven.
	empty := NewScenarioSession(NewState()).Report()
	require.Equal(t, false, empty["passed"])
}

// TestScenarioSteps_ThreadsTwinURLIntoSubject is the core of this issue: a
// twin binds a port at runtime and the subject observes that address in its
// environment. Without this, the per-scenario pipeline cannot be expressed as
// machine steps at all.
func TestScenarioSteps_ThreadsTwinURLIntoSubject(t *testing.T) {
	root := scenarioTree(t, map[string]map[string][]string{"alpha": {"happy": {"dep.yaml"}}})

	// The subject is the test binary in serve mode, echoing the environment
	// variable the twin's URL is published as.
	subjectDir := filepath.Join(root, "alpha")
	require.NoError(t, os.WriteFile(
		filepath.Join(subjectDir, profileFileName), []byte("name: alpha\n"), 0o644))

	state := NewState()
	session := NewScenarioSession(state)
	defer state.StopAll(2 * time.Second)

	_, err := session.Seed([]string{root})
	require.NoError(t, err)
	_, ok, err := session.Next()
	require.NoError(t, err)
	require.True(t, ok)

	// Start the twin: the test binary serving health, standing in for the
	// dependency. Its address is chosen at runtime.
	twinResult := Builder{
		ToolName: "start_twins", Init: InitStartTwins, State: state, Session: session,
		Config: ToolConfig{
			Profile: "twin-profile", Binary: os.Args[0],
			AddressEnv: envChildAddr,
			Env:        []string{envChildMode + "=serve"},
		},
	}.Build(core.Result{}).Execute()
	require.Equal(t, SignalTwinsStarted, twinResult.Signal, twinResult.Output)

	twins := session.Twins()
	require.Len(t, twins, 1)
	require.Equal(t, "DEP_URL", twins[0].EnvVar, "fixture dep.yaml publishes DEP_URL")
	require.NotEmpty(t, twins[0].BaseURL)

	// The subject's environment carries the twin's runtime URL.
	env := session.SubjectEnv()
	require.Contains(t, env, "DEP_URL="+twins[0].BaseURL)

	// Start the subject and confirm it actually observed that value: the child
	// echoes SERVICE_TEST_ECHO, which we set from the twin's URL.
	subjectResult := Builder{
		ToolName: "start_subject", Init: InitStartSubject, State: state, Session: session,
		Config: ToolConfig{
			Binary: os.Args[0], AddressEnv: envChildAddr,
			Env: []string{envChildMode + "=serve", envChildEcho + "=" + twins[0].BaseURL},
		},
	}.Build(core.Result{}).Execute()
	require.Equal(t, SignalSubjectStarted, subjectResult.Signal, subjectResult.Output)

	var started map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(subjectResult.Output), &started))
	require.Equal(t, filepath.Join(subjectDir, profileFileName), started["profile"],
		"the subject is the agent under test, resolved from the scenario")

	// The subject reports the twin URL it was given, proving the thread.
	health := Builder{
		ToolName: "await_subject", Init: InitAwaitSubject, State: state, Session: session,
		Config: ToolConfig{URL: "/healthz", Timeout: "20s", Interval: "20ms"},
	}.Build(core.Result{}).Execute()
	require.Equal(t, SignalHealthy, health.Signal, health.Output)

	_, subjectURL := session.Subject()
	body := httpGetBody(t, subjectURL+"/healthz")
	require.Contains(t, body, twins[0].BaseURL,
		"the subject observed the twin's runtime address in its environment")

	// Teardown stops both children.
	teardown := Builder{
		ToolName: "teardown", Init: InitTeardownScenario, State: state, Session: session,
		Config: ToolConfig{Grace: "2s"},
	}.Build(core.Result{}).Execute()
	require.Equal(t, SignalScenarioTornDown, teardown.Signal)
	require.Empty(t, state.Running(), "no child outlives the scenario")
}

// TestScenarioSteps_TeardownRunsOnFailurePath covers srd018 R1.5: a scenario
// whose health step fails still leaves nothing running.
func TestScenarioSteps_TeardownRunsOnFailurePath(t *testing.T) {
	root := scenarioTree(t, map[string]map[string][]string{"alpha": {"happy": {"dep.yaml"}}})

	state := NewState()
	session := NewScenarioSession(state)
	defer state.StopAll(2 * time.Second)

	_, err := session.Seed([]string{root})
	require.NoError(t, err)
	_, _, err = session.Next()
	require.NoError(t, err)

	twinResult := Builder{
		ToolName: "start_twins", Init: InitStartTwins, State: state, Session: session,
		Config: ToolConfig{
			Profile: "twin", Binary: os.Args[0], AddressEnv: envChildAddr,
			Env: []string{envChildMode + "=serve"},
		},
	}.Build(core.Result{}).Execute()
	require.Equal(t, SignalTwinsStarted, twinResult.Signal)
	require.Len(t, state.Running(), 1)

	// The subject starts but never serves the health path, so the wait times out.
	subjectResult := Builder{
		ToolName: "start_subject", Init: InitStartSubject, State: state, Session: session,
		Config: ToolConfig{
			Binary: os.Args[0], AddressEnv: envChildAddr,
			Env: []string{envChildMode + "=hang"},
		},
	}.Build(core.Result{}).Execute()
	require.Equal(t, SignalSubjectStarted, subjectResult.Signal)

	health := Builder{
		ToolName: "await_subject", Init: InitAwaitSubject, State: state, Session: session,
		Config: ToolConfig{URL: "/healthz", Timeout: "300ms", Interval: "20ms"},
	}.Build(core.Result{}).Execute()
	require.Equal(t, SignalHealthTimeout, health.Signal, "a dead subject must not look healthy")

	// The machine routes a failed health check to a forced verdict then teardown.
	verdict := Builder{
		ToolName: "collect", Init: InitCollectVerdict, State: state, Session: session,
		Config: ToolConfig{Reason: "subject never became healthy"},
	}.Build(core.Result{}).Execute()
	require.Equal(t, SignalScenarioFailed, verdict.Signal)

	teardown := Builder{
		ToolName: "teardown", Init: InitTeardownScenario, State: state, Session: session,
		Config: ToolConfig{Grace: "2s"},
	}.Build(core.Result{}).Execute()
	require.Equal(t, SignalScenarioTornDown, teardown.Signal)
	require.Empty(t, state.Running(), "a failed scenario still leaves nothing running")

	report := Builder{
		ToolName: "report", Init: InitReportSession, State: state, Session: session,
	}.Build(core.Result{}).Execute()
	require.Equal(t, SignalSessionFailed, report.Signal)
	require.Contains(t, report.Output, "subject never became healthy")
}

// TestScenarioSteps_SessionWordSignals covers the signals the assembler machine
// routes on, including the empty-discovery and exhausted-cursor paths.
func TestScenarioSteps_SessionWordSignals(t *testing.T) {
	t.Parallel()

	empty := t.TempDir()
	state := NewState()
	session := NewScenarioSession(state)

	none := Builder{
		ToolName: "init", Init: InitInitScenarioSession, State: state, Session: session,
		Config: ToolConfig{Roots: []string{empty}},
	}.Build(core.Result{}).Execute()
	require.Equal(t, SignalNoScenarios, none.Signal, "an empty tree is not a silent pass")

	root := scenarioTree(t, map[string]map[string][]string{"alpha": {"only": nil}})
	session2 := NewScenarioSession(NewState())
	seeded := Builder{
		ToolName: "init", Init: InitInitScenarioSession, State: state, Session: session2,
		Config: ToolConfig{Roots: []string{root}},
	}.Build(core.Result{}).Execute()
	require.Equal(t, SignalSessionSeeded, seeded.Signal)

	ready := Builder{ToolName: "next", Init: InitNextScenario, State: state, Session: session2}.
		Build(core.Result{}).Execute()
	require.Equal(t, SignalScenarioReady, ready.Signal)

	session2.RecordValidators([]ValidatorOutcome{{Name: "v", Passed: true}})
	session2.CollectVerdict("")

	done := Builder{ToolName: "next", Init: InitNextScenario, State: state, Session: session2}.
		Build(core.Result{}).Execute()
	require.Equal(t, SignalAllScenariosDone, done.Signal)

	report := Builder{ToolName: "report", Init: InitReportSession, State: state, Session: session2}.
		Build(core.Result{}).Execute()
	require.Equal(t, SignalSessionPassed, report.Signal)
}

// TestScenarioSteps_RejectIncompleteDeclarations covers build-time validation
// of the new words.
func TestScenarioSteps_RejectIncompleteDeclarations(t *testing.T) {
	t.Parallel()

	session := NewScenarioSession(NewState())

	// A step word with no current scenario is an error, not a panic.
	for _, init := range []string{InitStartTwins, InitStartSubject, InitRunScenarioTests} {
		result := Builder{
			ToolName: init, Init: init, State: NewState(), Session: session,
			Config: ToolConfig{Profile: "p"},
		}.Build(core.Result{}).Execute()
		require.Equal(t, core.CommandError, result.Signal, init)
		require.True(t,
			strings.Contains(result.Output, "no current scenario") ||
				strings.Contains(result.Output, "no subject started"),
			"%s: %s", init, result.Output)
	}

	// start_scenario_twins needs the twin profile declared.
	err := validateToolConfig("t", InitStartTwins, ToolConfig{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "requires the twin profile")

	// init_scenario_session needs roots.
	err = validateToolConfig("i", InitInitScenarioSession, ToolConfig{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "requires at least one root")
}

// TestRunValidators_FailedTerminalWithZeroExit locks in the contract that
// makes the rig able to fail at all: the agent binary exits 0 whether its
// machine reached a success terminal or a failure terminal, so a validator is
// judged on its reported terminal status, not its exit code alone.
func TestRunValidators_FailedTerminalWithZeroExit(t *testing.T) {
	t.Parallel()

	outcomes := RunValidators(context.Background(), os.Args[0], []ValidatorSpec{
		{Name: "reports-failed", Profile: "p", Env: []string{envChildMode + "=exit0failed"}},
		{Name: "reports-ok", Profile: "p", Env: []string{envChildMode + "=exit0"}},
		{Name: "silent", Profile: "p", Env: []string{envChildMode + "=exit0silent"}},
	}, 30*time.Second)

	byName := map[string]ValidatorOutcome{}
	for _, outcome := range outcomes {
		byName[outcome.Name] = outcome
	}

	// Exit 0 but a failed terminal is a failure.
	require.Equal(t, 0, byName["reports-failed"].ExitCode)
	require.Equal(t, "failed", byName["reports-failed"].Terminal)
	require.False(t, byName["reports-failed"].Passed,
		"a validator whose machine reached Failed must not pass because it exited 0")

	require.True(t, byName["reports-ok"].Passed)
	require.Equal(t, "succeeded", byName["reports-ok"].Terminal)

	// A validator that reported no terminal state proved nothing.
	require.False(t, byName["silent"].Passed)
	require.Contains(t, byName["silent"].Error, "no terminal state")
}

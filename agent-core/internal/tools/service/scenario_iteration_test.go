// Copyright (c) 2026 Nokia. All rights reserved.

package service

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
)

func TestRecordScenarioValidators_PreservesJoinOrder(t *testing.T) {
	t.Parallel()
	root := scenarioTree(t, map[string]map[string][]string{"alpha": {"only": nil}})
	session := NewScenarioSession(NewState())
	_, err := session.Seed([]string{root})
	require.NoError(t, err)
	_, _, err = session.Next()
	require.NoError(t, err)

	items := []map[string]interface{}{
		{"result": map[string]string{"output": jsonOutput(ValidatorOutcome{Name: "first", Passed: true})}},
		{"result": map[string]string{"output": jsonOutput(ValidatorOutcome{Name: "second", Passed: false, ExitCode: 2})}},
	}
	cmd := Builder{
		ToolName: "record", Init: InitRecordValidators, State: NewState(), Session: session,
		Config: ToolConfig{Outcomes: "$from(validators_joined).items"},
	}.Build(core.Result{})
	aware, ok := cmd.(core.CommandStateAware)
	require.True(t, ok)
	aware.SetCommandState(labeledStateView{
		label: "validators_joined", output: jsonOutput(map[string]interface{}{"items": items}),
	})
	result := cmd.Execute()
	require.Equal(t, SignalValidatorsRecorded, result.Signal)
	verdict := session.CollectVerdict("")
	require.Equal(t, []string{"first", "second"}, []string{
		verdict.Validators[0].Name, verdict.Validators[1].Name,
	})
	require.False(t, verdict.Passed)
}

func TestRunScenarioValidatorDoesNotEmitIrreversibleReceipt(t *testing.T) {
	t.Parallel()
	root := scenarioTree(t, map[string]map[string][]string{"alpha": {"only": nil}})
	session := NewScenarioSession(NewState())
	_, err := session.Seed([]string{root})
	require.NoError(t, err)
	_, _, err = session.Next()
	require.NoError(t, err)
	session.RecordSubject("subject", "http://127.0.0.1:1")
	profile := filepath.Join(root, "alpha", testsDirName, "only", profileFileName)
	require.NoError(t, os.WriteFile(profile, []byte("name: chatbot-turn-critic\n"), 0o644))

	factory := factoryFor(InitRunScenarioValidator, FactoryDeps{State: NewState(), Session: session})
	built, err := factory(catalog.ToolDef{Name: "run_validator", Config: map[string]interface{}{
		"binary": os.Args[0], "validator": "$from(validator).profile",
		"otlp_endpoint": "127.0.0.1:4317",
		"env":           []string{envChildMode + "=expect-otlp"}, "timeout": "30s",
	}}, nil)
	require.NoError(t, err)
	cmd := built.Build(core.Result{})
	aware, ok := cmd.(core.CommandStateAware)
	require.True(t, ok)
	aware.SetCommandState(labeledStateView{
		label: "validator", output: jsonOutput(map[string]string{"profile": profile}),
	})
	result := cmd.Execute()
	require.Equal(t, SignalValidatorCompleted, result.Signal)
	require.Empty(t, result.Receipt)
	require.Contains(t, result.Output, `"name":"chatbot-turn-critic"`)
	require.Contains(t, result.Output, `"passed":true`)
}

func TestValidatorNameUsesDeclaredCriticIdentity(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(t.TempDir(), "tests", "degraded-rag")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	profile := filepath.Join(dir, "profile.yaml")
	require.NoError(t, os.WriteFile(profile, []byte("name: chatbot-turn-critic\n"), 0o644))

	require.Equal(t, "chatbot-turn-critic", validatorName(profile))
	require.Equal(t, "degraded-rag", validatorName(filepath.Join(dir, "missing-profile.yaml")))
}

// TestRunScenarioValidator_JudgesOnExitCode locks in the contract that makes
// the rig able to fail: a machine that reached a failure terminal exits
// non-zero, while the reported terminal remains naming detail.
func TestRunScenarioValidator_JudgesOnExitCode(t *testing.T) {
	t.Parallel()
	specs := []ValidatorSpec{
		{Name: "reports-failed", Profile: "p", Env: []string{envChildMode + "=exit0failed"}},
		{Name: "reports-ok", Profile: "p", Env: []string{envChildMode + "=exit0"}},
		{Name: "silent", Profile: "p", Env: []string{envChildMode + "=exit0silent"}},
	}

	byName := map[string]ValidatorOutcome{}
	for _, spec := range specs {
		outcome := runOneValidator(t.Context(), os.Args[0], spec, 30*time.Second)
		byName[outcome.Name] = outcome
	}

	require.NotEqual(t, 0, byName["reports-failed"].ExitCode)
	require.Equal(t, "failed", byName["reports-failed"].Terminal)
	require.False(t, byName["reports-failed"].Passed)
	require.True(t, byName["reports-ok"].Passed)
	require.Equal(t, "succeeded", byName["reports-ok"].Terminal)
	require.True(t, byName["silent"].Passed)
}

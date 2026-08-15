// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package profileaudit

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestInspectProfileResolvesIncludesOverridesSelectionAndEnvironment(t *testing.T) {
	root := t.TempDir()
	t.Setenv("PROFILE_AUDIT_TIMEOUT", "20s")
	write(t, root, "machine.yaml", oneActionMachine("1m", "$tool"))
	write(t, root, "tools.yaml", "tools: [wait, included_only]\n")
	write(t, root, "base.yaml", declarations(
		tool("wait", "custom_await", "2m", "external")+
			tool("included_only", "custom_await", "10s", "external"),
	))
	write(t, root, "declarations.yaml", `
includes: [base.yaml]
tools:
  - name: wait
    type: builtin
    init: custom_await
    category: boundary
    visibility: external
    config: {timeout: "${PROFILE_AUDIT_TIMEOUT:-40s}"}
`)
	profile := writeProfile(t, root, "profile.yaml", "machine.yaml", "tools.yaml", "declarations.yaml", "")

	report, err := InspectProfile(profile)
	require.NoError(t, err)
	require.Empty(t, report.Diagnostics)
	require.Len(t, report.Operations, 2)
	require.Equal(t, []string{"included_only", "wait"}, []string{
		report.Operations[0].Action, report.Operations[1].Action,
	})
	require.Equal(t, 10*time.Second, report.Operations[0].Duration)
	require.Equal(t, 20*time.Second, report.Operations[1].Duration,
		"profile-local declaration must override the included 2m authority")
}

func TestResolveReferencePrefersDeclaringPackageOverCWD(t *testing.T) {
	root := t.TempDir()
	packageRoot := filepath.Join(root, "package")
	base := filepath.Join(packageRoot, "agents", "planner")
	expected := filepath.Join(packageRoot, "agents", "executor", "profile.yaml")
	conflictRoot := filepath.Join(root, "cwd")
	conflict := filepath.Join(conflictRoot, "agents", "executor", "profile.yaml")
	require.NoError(t, os.MkdirAll(filepath.Dir(expected), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Dir(conflict), 0o755))
	write(t, filepath.Dir(expected), filepath.Base(expected), "name: packaged\n")
	write(t, filepath.Dir(conflict), filepath.Base(conflict), "name: cwd\n")

	previous, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(conflictRoot))
	t.Cleanup(func() { require.NoError(t, os.Chdir(previous)) })

	require.Equal(t, canonical(expected), resolveReference(base, "agents/executor/profile.yaml"))
}

func TestInspectReportsMalformedEqualAndLargerDurationsDeterministically(t *testing.T) {
	root := t.TempDir()
	write(t, root, "machine.yaml", `
name: bad-timeouts
initial_state: S0
budget: {max_iterations: 10, command_timeout: 1m}
states:
  - S0
  - S1
  - S2
  - S3
  - S4
  - {name: Done, run_status: succeeded}
terminal_states: [Done]
signals: [Seed, A, B, C, D]
transitions:
  - {state: S0, signal: Seed, next: S1, action: malformed}
  - {state: S1, signal: A, next: S2, action: equal}
  - {state: S2, signal: B, next: S3, action: larger}
  - {state: S3, signal: C, next: S4, action: zero}
  - {state: S4, signal: D, next: Done}
`)
	write(t, root, "tools.yaml", "tools: [larger, malformed, equal, zero]\n")
	write(t, root, "declarations.yaml", declarations(
		tool("larger", "custom_await", "2m", "internal")+
			tool("malformed", "custom_await", "not-a-duration", "internal")+
			tool("equal", "custom_await", "1m", "internal")+
			tool("zero", "custom_await", "0s", "internal"),
	))
	profile := writeProfile(t, root, "profile.yaml", "machine.yaml", "tools.yaml", "declarations.yaml", "")

	report, err := Inspect(profile)
	require.NoError(t, err)
	require.Len(t, report.Diagnostics, 4)
	require.Equal(t, []string{"equal", "larger", "malformed", "zero"}, []string{
		report.Diagnostics[0].Action,
		report.Diagnostics[1].Action,
		report.Diagnostics[2].Action,
		report.Diagnostics[3].Action,
	})
	require.Contains(t, report.Diagnostics[0].String(), "operation 1m0s command_timeout 1m0s")
	require.Contains(t, report.Diagnostics[2].String(), `operation "not-a-duration"`)

	var validation *ValidationError
	require.ErrorAs(t, Validate(profile), &validation)
	require.Equal(t, report.Diagnostics, validation.Diagnostics)
}

func TestInspectIgnoresLaunchShutdownTimeoutButChecksSelectedStop(t *testing.T) {
	root := t.TempDir()
	write(t, root, "machine.yaml", `
name: collector-lifecycle
initial_state: Idle
budget: {max_iterations: 3, command_timeout: 10s}
states: [Idle, Running, {name: Done, run_status: succeeded}]
terminal_states: [Done]
signals: [Seed, ReceiverLaunched]
transitions:
  - {state: Idle, signal: Seed, next: Running, action: launch}
  - {state: Running, signal: ReceiverLaunched, next: Done, action: stop}
`)
	write(t, root, "tools.yaml", "tools: [launch, stop]\n")
	write(t, root, "declarations.yaml", declarations(`
  - name: launch
    type: builtin
    init: otlp_receiver_launch
    category: boundary
    visibility: internal
    config: {receiver: ingress, shutdown_timeout: 30s}
  - name: stop
    type: builtin
    init: otlp_receiver_stop
    category: boundary
    visibility: internal
    config: {receiver: ingress, shutdown_timeout: 30s}
`))
	profile := writeProfile(t, root, "profile.yaml", "machine.yaml", "tools.yaml", "declarations.yaml", "")

	report, err := Inspect(profile)
	require.NoError(t, err)
	require.Len(t, report.Operations, 1)
	require.Equal(t, "stop", report.Operations[0].Action)
	require.Equal(t, "ToolDef config.shutdown_timeout", report.Operations[0].Authority)
	require.Len(t, report.Diagnostics, 1)
	require.Equal(t, "stop", report.Diagnostics[0].Action)
}

func TestInspectFollowsMachineRequestProfileAndMachineOverride(t *testing.T) {
	root := t.TempDir()
	write(t, root, "machine.yaml", oneActionMachine("1m", "launch"))
	write(t, root, "request-machine.yaml", oneActionMachine("10s", "request_wait"))
	write(t, root, "tools.yaml", "tools: [launch]\n")
	write(t, root, "declarations.yaml", declarations(`
  - name: launch
    type: builtin
    init: rest_server_launch
    category: boundary
    visibility: internal
    config: {rest_ref: api}
`+tool("request_wait", "custom_await", "10s", "internal")))
	write(t, root, "rest.yaml", `
rest:
  version: v1
  limits:
    local: {timeout: 30s}
  servers:
    api:
      address: 127.0.0.1:19000
      limits_ref: local
      endpoints:
        run:
          method: POST
          path: /run
          binding: machine_request
          machine_request:
            profile: profile.yaml
            machine: request-machine.yaml
            timeout: 1m
            request:
              body: {input: $.input}
            response:
              terminal_states:
                Done: {status: 200}
`)
	profile := writeProfile(t, root, "profile.yaml", "machine.yaml", "tools.yaml", "declarations.yaml", "rest_definitions: [rest.yaml]\n")

	report, err := Inspect(profile)
	require.NoError(t, err)
	require.Len(t, report.Diagnostics, 2)
	require.Equal(t, "launch", report.Diagnostics[0].Action)
	require.Contains(t, report.Diagnostics[0].Authority, "machine_request.timeout")
	require.Equal(t, "request_wait", report.Diagnostics[1].Action)
	require.Equal(t, canonical(filepath.Join(root, "request-machine.yaml")), report.Diagnostics[1].Machine)
	again, err := Inspect(profile)
	require.NoError(t, err)
	require.Equal(t, report.Diagnostics, again.Diagnostics)
}

func TestInspectFollowsCompatibilityChildAndEvaluatorPointWrappers(t *testing.T) {
	root := t.TempDir()
	child := filepath.Join(root, "child")
	require.NoError(t, os.Mkdir(child, 0o755))
	write(t, child, "machine.yaml", oneActionMachine("15s", "child_wait"))
	write(t, child, "tools.yaml", "tools: [child_wait]\n")
	write(t, child, "declarations.yaml", declarations(tool("child_wait", "custom_await", "15s", "internal")))
	writeProfile(t, child, "profile.yaml", "machine.yaml", "tools.yaml", "declarations.yaml", "")

	write(t, root, "machine.yaml", `
name: wrappers
initial_state: S0
budget: {max_iterations: 5, command_timeout: 10m}
states: [S0, S1, {name: Done, run_status: succeeded}]
terminal_states: [Done]
signals: [Seed, ChildDone, PointDone]
transitions:
  - {state: S0, signal: Seed, next: S1, action: invoke_executor}
  - {state: S1, signal: ChildDone, next: Done, action: evaluate_point}
`)
	write(t, root, "point.yaml", oneActionMachine("20s", "point_wait"))
	write(t, root, "point-tools.yaml", "tools: [point_wait]\n")
	write(t, root, "point-declarations.yaml", declarations(tool("point_wait", "custom_await", "20s", "internal")))
	write(t, root, "tools.yaml", "tools: [invoke_executor, evaluate_point]\n")
	write(t, root, "declarations.yaml", declarations(`
  - name: invoke_executor
    type: builtin
    init: self_invoke
    category: boundary
    visibility: internal
    config: {profile: child/profile.yaml}
  - name: evaluate_point
    type: builtin
    init: run_point
    category: boundary
    visibility: internal
    config:
      point_machine: point.yaml
      point_tools: point-tools.yaml
      point_tool_declarations: [point-declarations.yaml]
      agent_name: point
      max_iterations: 5
      success_state: Done
`))
	profile := writeProfile(t, root, "profile.yaml", "machine.yaml", "tools.yaml", "declarations.yaml", "")

	report, err := Inspect(profile)
	require.NoError(t, err)
	require.Len(t, report.Diagnostics, 3)
	require.Equal(t, []string{"child_wait", "invoke_executor", "point_wait"}, []string{
		report.Diagnostics[0].Action, report.Diagnostics[1].Action, report.Diagnostics[2].Action,
	})
	require.Equal(t, "self_invoke execute.Config timeout", report.Diagnostics[1].Authority)
	var childAuthority Operation
	for _, operation := range report.Operations {
		if operation.Action == "invoke_executor" {
			childAuthority = operation
			break
		}
	}
	require.Equal(t, 10*time.Minute, childAuthority.Duration)
	require.Equal(t, "self_invoke execute.Config timeout", childAuthority.Authority)
}

func TestInspectFailsClosedForUnsupportedBlockingBoundary(t *testing.T) {
	root := t.TempDir()
	write(t, root, "machine.yaml", oneActionMachine("1m", "mystery"))
	write(t, root, "tools.yaml", "tools: [mystery]\n")
	write(t, root, "declarations.yaml", declarations(`
  - name: mystery
    type: builtin
    init: await_forever
    category: boundary
    visibility: internal
    config: {}
`))
	profile := writeProfile(t, root, "profile.yaml", "machine.yaml", "tools.yaml", "declarations.yaml", "")

	report, err := Inspect(profile)
	require.NoError(t, err)
	require.Len(t, report.Diagnostics, 1)
	require.Contains(t, report.Diagnostics[0].Reason, "no supported finite authority")
}

func writeProfile(
	t *testing.T,
	root, name, machine, tools, declarations, extra string,
) string {
	t.Helper()
	return write(t, root, name, "name: fixture\nmachine: "+machine+"\ntools: ["+tools+"]\n"+
		"tool_declarations: ["+declarations+"]\n"+extra)
}

func oneActionMachine(commandTimeout, action string) string {
	return `
name: fixture
initial_state: Idle
budget: {max_iterations: 2, command_timeout: ` + commandTimeout + `}
states: [Idle, {name: Done, run_status: succeeded}]
terminal_states: [Done]
signals: [Seed]
transitions:
  - {state: Idle, signal: Seed, next: Done, action: ` + action + `}
`
}

func declarations(tools string) string { return "tools:\n" + tools }

func tool(name, init, timeout, visibility string) string {
	return `
  - name: ` + name + `
    type: builtin
    init: ` + init + `
    category: boundary
    visibility: ` + visibility + `
    config: {timeout: ` + timeout + `}
`
}

func write(t *testing.T, root, name, content string) string {
	t.Helper()
	path := filepath.Join(root, name)
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	return path
}

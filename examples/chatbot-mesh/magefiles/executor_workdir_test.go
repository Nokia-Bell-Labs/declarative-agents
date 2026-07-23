// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// These cover the values-file path the executor writes and helm then reads
// (GH-737). The two are configured by different mechanisms and must agree:
// write_overrides takes its path from rest.yaml, which the runtime env-expands,
// while the helm words take theirs from exec-declarations.yaml, which it does
// not -- only REST definitions are expanded (agent-core
// internal/tools/rest/loading.go). The chart holds the second to the first by
// rewriting the exec args at packaging time.
//
// When they drift, nothing reports it: write_overrides writes one path, helm
// reads another, and helm renders whatever the release already had. Worse, if
// the written path falls outside the agent's workspace the write is refused
// outright and every apply dies on its first word -- which is what happened
// whenever profiles.workMountPath was changed from /work.

// executorValuesPath returns the values-file path the apply endpoint seeds,
// with placeholders resolved to their declared defaults.
func executorValuesPath(t *testing.T) string {
	t.Helper()
	var rest struct {
		Rest struct {
			Servers map[string]struct {
				Endpoints map[string]struct {
					MachineRequest struct {
						Request struct {
							Body map[string]string `yaml:"body"`
						} `yaml:"request"`
					} `yaml:"machine_request"`
				} `yaml:"endpoints"`
			} `yaml:"servers"`
		} `yaml:"rest"`
	}
	readIntakeYAML(t, filepath.Join(agentDir(t, "executor"), "rest.yaml"), &rest)
	path := rest.Rest.Servers["executor_apply"].Endpoints["apply"].
		MachineRequest.Request.Body["path"]
	if path == "" {
		t.Fatal("the apply endpoint seeds no values-file path")
	}
	return path
}

// TestExecutorValuesPathAgreesAcrossTheProfile proves the path write_overrides
// writes is the path the helm words read. A default render is what production
// gets, so the defaults are what must agree.
func TestExecutorValuesPathAgreesAcrossTheProfile(t *testing.T) {
	written := executorValuesPath(t)

	var decls execDeclarations
	readIntakeYAML(t, filepath.Join(agentDir(t, "executor"), "exec-declarations.yaml"), &decls)
	var checked int
	for _, tool := range decls.Tools {
		for i, arg := range tool.Args {
			if arg != "-f" {
				continue
			}
			if i+1 >= len(tool.Args) {
				t.Errorf("%s has a -f with no path", tool.Name)
				continue
			}
			checked++
			if read := tool.Args[i+1]; read != written {
				t.Errorf("%s reads %q but write_overrides writes %q; helm would render the release's existing values",
					tool.Name, read, written)
			}
		}
	}
	if checked == 0 {
		t.Fatal("no helm word passes -f a values file; the apply path or the declarations changed")
	}
}

// TestExecutorWorkDirFollowsTheMount proves a non-default workMountPath moves
// both paths together through the packaging path. This is the drift the
// hardcoded /work could not survive: the chart mounted the work volume
// elsewhere, the agent's workspace moved with it, and the write was then
// refused as outside the workspace.
func TestExecutorWorkDirFollowsTheMount(t *testing.T) {
	if _, err := exec.LookPath("helm"); err != nil {
		t.Skip("helm not on PATH")
	}
	chartDir := findChartDir(t)
	staged, cleanup, err := stageSmokeChart(chartDir, filepath.Dir(chartDir))
	if err != nil {
		t.Fatalf("stage chart: %v", err)
	}
	defer cleanup()

	out, err := exec.Command("helm", "template", "relx", staged,
		"--namespace", "nsy",
		"--set", "executor.enabled=true",
		"--set", "profiles.workMountPath=/scratch",
	).CombinedOutput()
	if err != nil {
		t.Fatalf("helm template: %v\n%s", err, out)
	}
	render := string(out)

	// The helm words must read the moved path, not the baked default.
	if !strings.Contains(render, "/scratch/overrides.yaml") {
		t.Error("the rendered exec args still read /work/overrides.yaml under workMountPath=/scratch")
	}
	if strings.Contains(render, "/work/overrides.yaml") {
		t.Error("the rendered chart still carries /work/overrides.yaml under workMountPath=/scratch")
	}
	// And the agent must be told where its workspace is, or write_overrides
	// resolves against a workspace that no longer contains the path.
	if !strings.Contains(render, "EXECUTOR_WORK_DIR") {
		t.Error("the executor Deployment does not pass EXECUTOR_WORK_DIR; the write path would not follow the mount")
	}
}

// TestExecutorDefaultRenderKeepsTheWorkPath proves the parameterization did not
// move production: a default render still writes and reads /work/overrides.yaml.
func TestExecutorDefaultRenderKeepsTheWorkPath(t *testing.T) {
	if _, err := exec.LookPath("helm"); err != nil {
		t.Skip("helm not on PATH")
	}
	chartDir := findChartDir(t)
	staged, cleanup, err := stageSmokeChart(chartDir, filepath.Dir(chartDir))
	if err != nil {
		t.Fatalf("stage chart: %v", err)
	}
	defer cleanup()

	out, err := exec.Command("helm", "template", "relx", staged,
		"--namespace", "nsy", "--set", "executor.enabled=true").CombinedOutput()
	if err != nil {
		t.Fatalf("helm template: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "/work/overrides.yaml") {
		t.Error("a default render no longer uses /work/overrides.yaml; production moved")
	}
}

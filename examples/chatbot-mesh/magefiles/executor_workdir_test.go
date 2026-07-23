// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// These cover the values-file path the executor writes and helm then reads
// (GH-737). Both sides name the same environment reference, which the runtime
// expands in REST definitions and, since GH-728, in tool declarations too
// (srd013 R5.6) -- so one mounted profile parameterizes per pod and the write
// and the read cannot drift apart at packaging time.
//
// They still must be asserted together, because when they drift nothing reports
// it: write_overrides writes one path, helm reads another, and helm renders
// whatever the release already had. Worse, if the written path falls outside the
// agent's workspace the write is refused outright and every apply dies on its
// first word -- which is what happened whenever profiles.workMountPath was
// changed from /work.

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

// TestExecutorWorkDirFollowsTheMount proves a non-default workMountPath reaches
// the agent. The profile carries the reference unresolved -- expansion happens
// when the agent loads it, not when helm renders it -- so what the render must
// carry is the variable the deployment sets. This is the drift the hardcoded
// /work could not survive: the chart mounted the work volume elsewhere, the
// agent's workspace moved with it, and the write was then refused as outside
// the workspace.
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

	// The agent must be told where its workspace is, or write_overrides resolves
	// against a workspace that no longer contains the path.
	if !strings.Contains(render, `{name: EXECUTOR_WORK_DIR, value: "/scratch"}`) {
		t.Error("the executor Deployment does not pass EXECUTOR_WORK_DIR=/scratch; the paths would not follow the mount")
	}
	// Both sides must carry the reference rather than a baked path, so the one
	// variable moves them together.
	if !strings.Contains(render, "${EXECUTOR_WORK_DIR:-/work}/overrides.yaml") {
		t.Error("the mounted profile bakes a values-file path instead of the reference; the mount could move without it")
	}
}

// TestExecutorDefaultRenderKeepsTheWorkPath proves the parameterization did not
// move production: with the default workMountPath the reference resolves to
// /work, which is where the executor has always written.
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
	if !strings.Contains(string(out), `{name: EXECUTOR_WORK_DIR, value: "/work"}`) {
		t.Error("a default render no longer sets EXECUTOR_WORK_DIR=/work; production moved")
	}
	if !strings.Contains(string(out), "${EXECUTOR_WORK_DIR:-/work}/overrides.yaml") {
		t.Error("a default render no longer carries the values-file reference")
	}
}

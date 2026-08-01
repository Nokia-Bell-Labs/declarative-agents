// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// These cover the values-file path the applier writes and helm then reads. Both
// sides name the same environment reference, which the runtime expands in REST
// definitions and in tool declarations -- so one mounted profile parameterizes
// per pod and the write and the read cannot drift apart at packaging time.
//
// They still must be asserted together, because when they drift nothing reports
// it: write_overrides writes one path, helm reads another, and helm renders
// whatever the release already had. Worse, if the written path falls outside the
// agent's workspace the write is refused outright and every apply dies on its
// first word.
//
// The coding chart pins workspace.mountPath to /work (the schema rejects any
// other value), so unlike the chatbot-mesh mesh the mount does not move; what
// these prove is that both the deployment env and the mounted profile derive the
// work path from one reference that resolves to /work.

// applierValuesPath returns the values-file path the apply endpoint seeds, with
// placeholders resolved to their declared defaults.
func applierValuesPath(t *testing.T) string {
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
	readIntakeYAML(t, filepath.Join(agentDir(t, "applier"), "rest.yaml"), &rest)
	path := rest.Rest.Servers["applier_apply"].Endpoints["apply"].
		MachineRequest.Request.Body["path"]
	if path == "" {
		t.Fatal("the apply endpoint seeds no values-file path")
	}
	return path
}

// TestApplierValuesPathAgreesAcrossTheProfile proves the path write_overrides
// writes is the path the helm words read. A default render is what production
// gets, so the defaults are what must agree.
func TestApplierValuesPathAgreesAcrossTheProfile(t *testing.T) {
	written := applierValuesPath(t)

	var decls execDeclarations
	readIntakeYAML(t, filepath.Join(agentDir(t, "applier"), "exec-declarations.yaml"), &decls)
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

// TestApplierDefaultRenderKeepsTheWorkPath proves the deployment sets
// APPLIER_WORK_DIR to the pinned /work mount and the mounted profile carries the
// reference rather than a baked path, so the one variable moves the write and the
// helm read together.
func TestApplierDefaultRenderKeepsTheWorkPath(t *testing.T) {
	chart := preparedApplierChart(t)
	out, err := exec.Command("helm", "template", "relx", chart,
		"--namespace", "nsy", "--set", "applier.enabled=true").CombinedOutput()
	if err != nil {
		t.Fatalf("helm template: %v\n%s", err, out)
	}
	render := string(out)
	if !strings.Contains(render, `{name: APPLIER_WORK_DIR, value: "/work"}`) {
		t.Error("the applier Deployment no longer sets APPLIER_WORK_DIR=/work; the write path would not follow the mount")
	}
	if !strings.Contains(render, "${APPLIER_WORK_DIR:-/work}/overrides.yaml") {
		t.Error("the mounted profile bakes a values-file path instead of the reference; the write and the read could drift")
	}
}

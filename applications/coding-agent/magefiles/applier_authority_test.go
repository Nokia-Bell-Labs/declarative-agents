// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// These assert the authority boundary at the profile source (srd006 R4.2, AC4):
// the applier alone binds helm and kubectl as exec words, and the planner,
// executor, and critic serving roles carry none. The tracer proves no *invocation*
// carries deployment authority; this proves no serving role could construct one,
// because the words are not in its profile to begin with.

// deploymentCLIBinaries are the CLIs that carry deployment-plane authority. Only
// the applier may bind them.
var deploymentCLIBinaries = map[string]bool{"helm": true, "kubectl": true}

// execBinariesUnder collects the binary of every exec word declared in the YAML
// files directly under a normalized actor directory.
func execBinariesUnder(t *testing.T, dir string) map[string]string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read %s: %v", dir, err)
	}
	found := map[string]string{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", entry.Name(), err)
		}
		// Profile and tool-list files carry tools: as a list of strings; only
		// declaration files carry the list of maps that decodes here. A file that
		// is not a declaration file is skipped, not an error -- what matters is
		// that no declaration in the tree binds a deployment CLI.
		var decls execDeclarations
		if yaml.Unmarshal(data, &decls) != nil {
			continue
		}
		for _, tool := range decls.Tools {
			if tool.Binary != "" {
				found[tool.Name] = tool.Binary
			}
		}
	}
	return found
}

// TestServingRolesCarryNoDeploymentCLI proves the planner, executor, and critic
// profiles (and the common declarations they share) declare no helm or kubectl
// exec word. A role that grew one would hold deployment authority the applier is
// meant to hold alone.
func TestServingRolesCarryNoDeploymentCLI(t *testing.T) {
	for _, role := range []string{"planner", "executor", "critic", "role-server"} {
		t.Run(role, func(t *testing.T) {
			for name, binary := range execBinariesUnder(t, agentDir(t, role)) {
				if deploymentCLIBinaries[binary] {
					t.Errorf("serving role %s declares exec word %s bound to %s; only the applier may hold a deployment CLI",
						role, name, binary)
				}
			}
		})
	}
}

// TestApplierAloneBindsTheDeploymentCLIs proves the applier does bind both helm
// and kubectl, so the negative assertion above is not vacuous -- the boundary is a
// division of authority, not a chart with no deployment CLI anywhere.
func TestApplierAloneBindsTheDeploymentCLIs(t *testing.T) {
	binaries := execBinariesUnder(t, agentDir(t, "applier"))
	present := map[string]bool{}
	for _, binary := range binaries {
		present[binary] = true
	}
	for cli := range deploymentCLIBinaries {
		if !present[cli] {
			t.Errorf("the applier declares no %s exec word; it is the only role meant to bind one", cli)
		}
	}
}

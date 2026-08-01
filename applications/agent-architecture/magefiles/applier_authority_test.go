// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// These assert the authority boundary at the profile source (srd002-applier R4.1,
// AC4): the applier alone binds helm and kubectl as exec words, and the serving
// curator and collector profiles carry none. The tracer proves no *invocation*
// carries deployment authority; this proves no serving role could construct one,
// because the words are not in its profile to begin with.

// deploymentCLIBinaries are the CLIs that carry deployment-plane authority. Only
// the applier may bind them.
var deploymentCLIBinaries = map[string]bool{"helm": true, "kubectl": true}

// execBinariesUnder collects the binary of every exec word declared in the YAML
// files directly under an agent directory.
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
		// declaration files carry the list of maps that decodes here.
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

// catalogAgentDir resolves a catalog-owned serving profile directory. The curator
// and collector profiles the chart mounts are catalog-owned, not application-owned.
func catalogAgentDir(t *testing.T, rel string) string {
	t.Helper()
	resolved, err := resolveRootsFromWorkingDirectory()
	if err != nil {
		t.Fatalf("resolve roots: %v", err)
	}
	return filepath.Join(resolved.Catalog, filepath.FromSlash(rel))
}

// TestServingRolesCarryNoDeploymentCLI proves the curator and collector serving
// profiles declare no helm or kubectl exec word. A role that grew one would hold
// deployment authority the applier is meant to hold alone.
func TestServingRolesCarryNoDeploymentCLI(t *testing.T) {
	dirs := map[string]string{
		"curator":   "agents/knowledge-manager/documentation-curator",
		"collector": "agents/collector",
	}
	for role, rel := range dirs {
		t.Run(role, func(t *testing.T) {
			for name, binary := range execBinariesUnder(t, catalogAgentDir(t, rel)) {
				if deploymentCLIBinaries[binary] {
					t.Errorf("serving role %s declares exec word %s bound to %s; only the applier may hold a deployment CLI",
						role, name, binary)
				}
			}
		})
	}
}

// TestApplierAloneBindsTheDeploymentCLIs proves the applier does bind both helm and
// kubectl, so the negative assertion above is not vacuous -- the boundary is a
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

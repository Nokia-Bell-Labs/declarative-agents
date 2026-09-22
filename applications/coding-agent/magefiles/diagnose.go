// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Nokia-Bell-Labs/declarative-agents/magefiles/kindrig"
)

// rigDoctorProfileRel is the catalog family that reads a captured evidence
// directory and writes the diagnosis beside it (srd023).
const rigDoctorProfileRel = "agents/rig-doctor/profile.yaml"

// Diagnose captures a read-only snapshot of one running scenario into
// build/kind-evidence and runs the catalog rig doctor over it (eng01, srd023).
// Pass the scenario name (coding-agent-helm, or demo for the persistent demo
// cluster). It reports and never gates. This application keeps no persistent
// trace ingress, so the evidence is kubectl reads and kind logs.
func Diagnose(scenario string) error {
	root, err := os.Getwd()
	if err != nil {
		return err
	}
	revision, err := gitOutput(root, "rev-parse", "HEAD")
	if err != nil {
		revision = ""
	}
	return kindrig.Diagnose(kindrig.DiagnoseRequest{
		Scenario:        scenario,
		DemoCluster:     codingDemoCluster,
		ApplicationRoot: root,
		Revision:        revision,
		Agent:           codingRigDoctor(root),
	})
}

// codingRigDoctor resolves the catalog rig doctor with this application's own
// root and build helpers. A root that does not resolve returns a zero agent,
// which captures evidence and reports that no diagnosis ran.
func codingRigDoctor(root string) kindrig.DiagnoseAgent {
	coreRoot := codingAgentCoreRoot(root)
	catalogRoot, err := resolveCatalogRoot("rig doctor", root)
	if err != nil {
		fmt.Printf("diagnose: %v\n", err)
		return kindrig.DiagnoseAgent{}
	}
	binary, cleanup, err := buildAgent(coreRoot)
	if err != nil {
		fmt.Printf("diagnose: %v\n", err)
		return kindrig.DiagnoseAgent{}
	}
	return kindrig.DiagnoseAgent{
		Binary:   binary,
		Profile:  filepath.Join(catalogRoot, filepath.FromSlash(rigDoctorProfileRel)),
		CoreRoot: coreRoot,
		Cleanup:  cleanup,
	}
}

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
// Pass the scenario name (agent-architecture-helm, or demo for the persistent
// demo cluster). It reports and never gates. This application keeps no
// persistent trace ingress, so the evidence is kubectl reads and kind logs.
func Diagnose(scenario string) error {
	root, err := os.Getwd()
	if err != nil {
		return err
	}
	return kindrig.Diagnose(kindrig.DiagnoseRequest{
		Scenario:        scenario,
		DemoCluster:     demoCluster,
		ApplicationRoot: root,
		Revision:        mustGitRevision(root),
		Agent:           architectureRigDoctor(),
	})
}

// architectureRigDoctor resolves the catalog rig doctor through this
// application's demo.yaml roots. A root that does not resolve returns a zero
// agent, which captures evidence and reports that no diagnosis ran.
func architectureRigDoctor() kindrig.DiagnoseAgent {
	roots, err := resolveRootsFromWorkingDirectory()
	if err != nil {
		fmt.Printf("diagnose: %v\n", err)
		return kindrig.DiagnoseAgent{}
	}
	binary, cleanup, err := buildApplierBinary(roots.Core)
	if err != nil {
		fmt.Printf("diagnose: %v\n", err)
		return kindrig.DiagnoseAgent{}
	}
	return kindrig.DiagnoseAgent{
		Binary:   binary,
		Profile:  filepath.Join(roots.Catalog, filepath.FromSlash(rigDoctorProfileRel)),
		CoreRoot: roots.Core,
		Cleanup:  cleanup,
	}
}

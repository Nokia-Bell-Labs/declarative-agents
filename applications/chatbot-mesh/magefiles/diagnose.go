// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/magefiles/kindrig"
)

// rigDoctorProfileRel is the catalog family that reads a captured evidence
// directory and writes the diagnosis beside it (srd023).
const rigDoctorProfileRel = "agents/rig-doctor/profile.yaml"

// diagnoseDemoScenario names the persistent demo cluster, whose release lives
// in the default namespace, instead of a da-platform scenario namespace.
const (
	diagnoseDemoScenario  = "demo"
	diagnoseDemoNamespace = "default"
)

// diagnoseTarget is the running cluster and namespace one diagnosis reads.
type diagnoseTarget struct {
	cluster   string
	namespace string
}

// resolveDiagnoseTarget maps a scenario argument to its cluster and namespace:
// the demo to its own cluster, every other scenario to its da-<scenario>
// namespace on da-platform. Cluster-mutating scenarios own clusters that are
// deleted on release, so only these two remain running to diagnose.
func resolveDiagnoseTarget(scenario string) (diagnoseTarget, error) {
	if scenario == diagnoseDemoScenario {
		return diagnoseTarget{cluster: chatbotDemoCluster, namespace: diagnoseDemoNamespace}, nil
	}
	namespace, err := kindrig.ScenarioNamespaceName(scenario)
	if err != nil {
		return diagnoseTarget{}, err
	}
	return diagnoseTarget{cluster: kindrig.PlatformClusterName, namespace: namespace}, nil
}

func diagnoseEvidenceDirectory(root, scenario string, now time.Time) string {
	return filepath.Join(root, "build", "kind-evidence",
		"diagnose-"+scenario+"-"+now.UTC().Format("20060102T150405Z"))
}

func diagnoseTraceSpool(root string) string {
	return filepath.Join(root, observabilityStateDir, observabilitySpoolDir,
		"traces", "collector.ndjson")
}

// Diagnose captures a read-only snapshot of one running scenario -- events,
// describe output, rollout counts, pod logs, and the trace spool tail, indexed
// by manifest.yaml -- into build/kind-evidence, then runs the catalog
// rig-doctor over it (eng01, srd023). Pass the scenario name (helm-smoke, or
// demo for the persistent demo cluster). It reports and never fails on what it
// finds: capture problems are recorded in the manifest and a diagnosis that
// does not come out leaves the evidence behind. Only an unusable argument or a
// missing cluster is an error.
func Diagnose(scenario string) error {
	target, err := resolveDiagnoseTarget(scenario)
	if err != nil {
		return err
	}
	if !kindrig.Exists(kindrig.CaptureRun, target.cluster) {
		return fmt.Errorf("diagnose %s: cluster %s is not running", scenario, target.cluster)
	}
	root, err := os.Getwd()
	if err != nil {
		return err
	}
	commands, cleanup, err := kindrig.ClusterCommands(kindrig.CaptureRun, target.cluster)
	if err != nil {
		return err
	}
	defer cleanup()
	dir := diagnoseEvidenceDirectory(root, scenario, time.Now())
	evidence := kindrig.FailureEvidence{
		Directory:  dir,
		Namespaces: []string{target.namespace},
		Run: boundedHelmEvidenceRunnerWith(
			helmDiagnosticRunner(commands.RunContext), helmEvidenceCommandTimeout),
		Revision:   gitCommit(root),
		TraceSpool: diagnoseTraceSpool(root),
	}
	if err := evidence.Capture(commands.KindRun, target.cluster); err != nil {
		fmt.Printf("diagnose: capture recorded errors (see %s):\n%v\n",
			kindrig.EvidenceManifestFile, err)
	}
	fmt.Printf("diagnose: evidence for %s/%s in %s\n", target.cluster, target.namespace, dir)
	if err := runRigDoctor(root, dir); err != nil {
		fmt.Printf("diagnose: %v\n", err)
		fmt.Printf("diagnose: the evidence stands on its own; read %s\n", dir)
	}
	return nil
}

// runRigDoctor runs the catalog rig-doctor over one evidence directory. The
// agent exits 2 when a machine reaches a failure terminal, which for this
// family means NoEvidence or a provider that could not be reached; both are
// reported, neither fails the target (srd023 R4).
func runRigDoctor(root, evidenceDir string) error {
	coreRoot := demoCoreRoot(root)
	if !agentCoreAvailable(coreRoot) {
		return fmt.Errorf("skipping diagnosis: agent-core checkout not found at %s", coreRoot)
	}
	catalogRoot, err := resolveCatalogRoot("rig doctor", root)
	if err != nil {
		return fmt.Errorf("skipping diagnosis: %w", err)
	}
	binary, err := buildAgent(coreRoot)
	if err != nil {
		return err
	}
	profile := filepath.Join(catalogRoot, filepath.FromSlash(rigDoctorProfileRel))
	cmd := exec.Command(binary, "--profile", profile,
		"--directory", evidenceDir, "--core-root", coreRoot)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	runErr := cmd.Run()
	var exitErr *exec.ExitError
	if runErr != nil && !errors.As(runErr, &exitErr) {
		return fmt.Errorf("run rig-doctor: %w", runErr)
	}
	diagnosis := filepath.Join(evidenceDir, "diagnosis.yaml")
	if _, err := os.Stat(diagnosis); err != nil {
		return fmt.Errorf("the rig-doctor wrote no diagnosis")
	}
	fmt.Printf("diagnose: diagnosis in %s\n", diagnosis)
	return nil
}

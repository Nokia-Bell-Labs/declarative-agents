// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package kindrig

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// DiagnoseDemoScenario is the argument that selects an application's own demo
// cluster, whose release lives in the default namespace, instead of a scenario
// namespace on the shared platform.
const DiagnoseDemoScenario = "demo"

const (
	diagnoseDemoNamespace = "default"
	// diagnoseCommandTimeout bounds each kubectl read the capture issues, so a
	// wedged API server costs one timeout per file rather than the whole run.
	diagnoseCommandTimeout = 10 * time.Second
	diagnosisFile          = "diagnosis.yaml"
)

// DiagnoseTarget is the running cluster and namespace one diagnosis reads.
type DiagnoseTarget struct {
	Cluster   string
	Namespace string
}

// DiagnoseAgent locates the agent the rig runs over captured evidence. An
// application resolves these paths with its own root and build helpers; a zero
// value captures evidence and runs nothing.
type DiagnoseAgent struct {
	Binary   string
	Profile  string
	CoreRoot string
	// Cleanup releases whatever holds the binary, once the run is over.
	Cleanup func()
}

// DiagnoseRequest is one application's on-demand diagnosis.
type DiagnoseRequest struct {
	// Scenario is the mage argument: DiagnoseDemoScenario or a scenario whose
	// namespace is da-<scenario> on the shared platform.
	Scenario string
	// DemoCluster is the application's own persistent demo cluster.
	DemoCluster string
	// Target, when set, is the already resolved application cluster and
	// namespace. apprig uses it because stable application namespaces are
	// app-<identity>, not scenario namespaces. Legacy scenario/demo callers
	// leave it nil and retain ResolveDiagnoseTarget behavior.
	Target *DiagnoseTarget
	// ApplicationRoot is where build/kind-evidence lives.
	ApplicationRoot string
	// Revision is recorded in the manifest.
	Revision string
	// TraceSpool is an optional spool whose newest spans are captured beside
	// the kubectl evidence. An application without a persistent ingress leaves
	// it empty.
	TraceSpool string
	// Agent runs over the captured evidence. A zero value captures only.
	Agent DiagnoseAgent
	// CommandTimeout overrides the per-command bound on capture reads.
	CommandTimeout time.Duration
	// Now overrides the evidence directory's timestamp. Tests set it.
	Now func() time.Time
}

// ResolveDiagnoseTarget maps a scenario argument to the cluster and namespace
// that hold its state: the demo to its own cluster, every other scenario to
// its da-<scenario> namespace on the shared platform. Cluster-mutating
// scenarios own clusters that are deleted on release, so only these two remain
// running to diagnose.
func ResolveDiagnoseTarget(scenario, demoCluster string) (DiagnoseTarget, error) {
	if scenario == DiagnoseDemoScenario {
		if demoCluster == "" {
			return DiagnoseTarget{}, errors.New("diagnose demo: this application declares no demo cluster")
		}
		return DiagnoseTarget{Cluster: demoCluster, Namespace: diagnoseDemoNamespace}, nil
	}
	namespace, err := ScenarioNamespaceName(scenario)
	if err != nil {
		return DiagnoseTarget{}, err
	}
	return DiagnoseTarget{Cluster: PlatformClusterName, Namespace: namespace}, nil
}

// DiagnoseEvidenceDirectory names the directory one diagnosis writes into.
func DiagnoseEvidenceDirectory(applicationRoot, scenario string, now time.Time) string {
	return filepath.Join(applicationRoot, "build", "kind-evidence",
		"diagnose-"+scenario+"-"+now.UTC().Format("20060102T150405Z"))
}

// Diagnose captures a read-only snapshot of one running scenario -- events,
// describe output, rollout counts, pod logs, and any trace spool tail, indexed
// by manifest.yaml -- then runs the rig doctor over it (eng01, srd023).
//
// It reports and never gates on what it finds: capture problems are recorded
// in the manifest, and a diagnosis that does not come out leaves the evidence
// behind. Only an unusable scenario argument or a cluster that is not running
// is an error, because neither leaves anything to report on.
func Diagnose(request DiagnoseRequest) error {
	var target DiagnoseTarget
	if request.Target != nil {
		target = *request.Target
		if strings.TrimSpace(target.Cluster) == "" || strings.TrimSpace(target.Namespace) == "" {
			return errors.New("diagnose: explicit target requires cluster and namespace")
		}
	} else {
		var err error
		target, err = ResolveDiagnoseTarget(request.Scenario, request.DemoCluster)
		if err != nil {
			return err
		}
	}
	if !Exists(CaptureRun, target.Cluster) {
		return fmt.Errorf("diagnose %s: cluster %s is not running", request.Scenario, target.Cluster)
	}
	commands, cleanup, err := ClusterCommands(CaptureRun, target.Cluster)
	if err != nil {
		return err
	}
	defer cleanup()

	now := time.Now
	if request.Now != nil {
		now = request.Now
	}
	directory := DiagnoseEvidenceDirectory(request.ApplicationRoot, request.Scenario, now())
	timeout := request.CommandTimeout
	if timeout <= 0 {
		timeout = diagnoseCommandTimeout
	}
	evidence := FailureEvidence{
		Directory:  directory,
		Namespaces: []string{target.Namespace},
		Run:        boundedRunner(commands.RunContext, timeout),
		Revision:   request.Revision,
		TraceSpool: request.TraceSpool,
	}
	if err := evidence.Capture(commands.KindRun, target.Cluster); err != nil {
		fmt.Printf("diagnose: capture recorded errors (see %s):\n%v\n", EvidenceManifestFile, err)
	}
	fmt.Printf("diagnose: evidence for %s/%s in %s\n", target.Cluster, target.Namespace, directory)
	if request.Agent.Cleanup != nil {
		defer request.Agent.Cleanup()
	}
	if err := request.Agent.diagnose(directory); err != nil {
		fmt.Printf("diagnose: %v\n", err)
		fmt.Printf("diagnose: the evidence stands on its own; read %s\n", directory)
	}
	return nil
}

// boundedRunner adapts a context-aware cluster runner to the evidence
// capture's runner, giving every command the same bound.
func boundedRunner(
	run func(context.Context, string, ...string) ([]byte, error),
	timeout time.Duration,
) CommandRunner {
	return func(name string, args ...string) ([]byte, error) {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		return run(ctx, name, args...)
	}
}

// diagnose runs the agent over one evidence directory. The agent exits 2 when
// its machine reaches a failure terminal, which for the rig doctor means no
// evidence or a provider it could not reach; both are reported rather than
// raised, since the evidence is still worth reading (srd023 R4).
func (a DiagnoseAgent) diagnose(evidenceDir string) error {
	if a.Binary == "" || a.Profile == "" {
		return fmt.Errorf("skipping diagnosis: this application resolved no rig doctor")
	}
	cmd := exec.Command(a.Binary, "--profile", a.Profile,
		"--directory", evidenceDir, "--core-root", a.CoreRoot)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	runErr := cmd.Run()
	var exitErr *exec.ExitError
	if runErr != nil && !errors.As(runErr, &exitErr) {
		return fmt.Errorf("run rig doctor: %w", runErr)
	}
	diagnosis := filepath.Join(evidenceDir, diagnosisFile)
	if _, err := os.Stat(diagnosis); err != nil {
		return errors.New("the rig doctor wrote no diagnosis")
	}
	fmt.Printf("diagnose: diagnosis in %s\n", diagnosis)
	return nil
}

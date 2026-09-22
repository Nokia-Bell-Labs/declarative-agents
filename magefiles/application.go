// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/magefiles/apprig"
	"github.com/Nokia-Bell-Labs/declarative-agents/magefiles/kindrig"
	"github.com/magefile/mage/mg"
)

// App groups the manifest-driven application lifecycle targets. The root
// targets run the checked-in conformance fixture; first-party and downstream
// modules construct the same apprig.Runner with their typed preparation and
// verification callbacks.
type App mg.Namespace

func (App) Up() error {
	runner, err := rootApplicationRunner()
	if err != nil {
		return err
	}
	return runner.Up()
}

func (App) Status() error {
	runner, err := rootApplicationRunner()
	if err != nil {
		return err
	}
	report, err := runner.Status()
	if err != nil {
		return err
	}
	return printApplicationStatus(report)
}

func (App) Down() error {
	runner, err := rootApplicationRunner()
	if err != nil {
		return err
	}
	report, err := runner.Down()
	if err != nil {
		return err
	}
	return printApplicationStatus(report)
}

func (App) Diagnose() error {
	runner, err := rootApplicationRunner()
	if err != nil {
		return err
	}
	return runner.Diagnose()
}

func (App) Purge() error {
	runner, err := rootApplicationRunner()
	if err != nil {
		return err
	}
	return runner.PurgeData()
}

// Integration groups root-owned persistent-platform proofs.
type Integration mg.Namespace

// ApplicationLifecycle exercises the reusable runner against da-platform with
// a separate fixture chart. The applier and rig-doctor remain the transaction
// and diagnosis authorities; this target observes their live results.
func (Integration) ApplicationLifecycle() (result error) {
	root, err := absoluteRepositoryRoot()
	if err != nil {
		return err
	}
	platform, err := kindrig.AcquirePlatform(kindrig.PlatformOptions{
		EvidenceDirectory: filepath.Join(root, "build", "kind-evidence", "application-lifecycle"),
	})
	if err != nil {
		return err
	}
	defer func() { platform.Stop(result != nil) }()

	runner, err := fixtureApplicationRunner(root)
	if err != nil {
		return err
	}
	// Remove residue from an interrupted proof through the same ordered path.
	_, _ = runner.Down()
	if err := runner.Up(); err != nil {
		return err
	}
	report, err := runner.Status()
	if err != nil {
		return err
	}
	if report.Workloads.State != apprig.StateOK || report.QueryEndpoint.State != apprig.StateOK {
		return fmt.Errorf("running fixture status = %+v", report)
	}
	if err := runner.Diagnose(); err != nil {
		return err
	}
	down, err := runner.Down()
	if err != nil {
		return err
	}
	if down.QueryEndpoint.State != apprig.StateAbsent || down.Bucket.State != apprig.StateOK {
		return fmt.Errorf("down did not separate retained bucket and absent query: %+v", down)
	}

	// A later up uses the same resolved bucket identity and reaches Deployed
	// again; the lifecycle never invokes purge.
	if err := runner.Up(); err != nil {
		return err
	}
	if _, err := runner.Down(); err != nil {
		return err
	}

	// Application-specific verification failure must compensate through
	// canonical undeploy before namespace cleanup.
	failing, err := fixtureApplicationRunner(root)
	if err != nil {
		return err
	}
	failing.Verify = func(apprig.Resolved) error {
		return errors.New("injected application verification failure")
	}
	if err := failing.Up(); err == nil {
		return errors.New("injected verification failure did not fail app:up")
	}
	after, err := failing.Status()
	if err != nil {
		return err
	}
	if after.QueryEndpoint.State != apprig.StateAbsent {
		return fmt.Errorf("verification compensation left query endpoint live: %+v", after)
	}
	fmt.Println("integration:applicationLifecycle PASS - canonical deploy/diagnose/down, retained identity, re-up, and verify compensation")
	return nil
}

func rootApplicationRunner() (apprig.Runner, error) {
	root, err := absoluteRepositoryRoot()
	if err != nil {
		return apprig.Runner{}, err
	}
	return fixtureApplicationRunner(root)
}

func fixtureApplicationRunner(root string) (apprig.Runner, error) {
	fixture := filepath.Join(root, "magefiles", "apprig", "testdata", "lifecycle-fixture")
	revision := strings.TrimSpace(string(commandOutput(root, "git", "rev-parse", "HEAD")))
	runner := apprig.Runner{
		ManifestPath: filepath.Join(fixture, "agents", "application.yaml"),
		Binding: apprig.PlatformBinding{
			Cluster: "da-platform", ApplicationRoot: fixture,
			ChartName: "apprig-fixture", ChartPath: filepath.Join(fixture, "helm"),
			ValuesPath: filepath.Join(fixture, "helm", "values.yaml"), Timeout: "3m",
		},
		CatalogRoot: filepath.Join(root, "applications", "catalog"),
		Revision:    revision,
	}
	runner.DeployAgent = func() (kindrig.DeployAgent, error) {
		return buildLifecycleDeployAgent(root, "agents/applier/deploy-profile.yaml")
	}
	runner.UndeployAgent = func() (kindrig.DeployAgent, error) {
		return buildLifecycleDeployAgent(root, "agents/applier/undeploy-profile.yaml")
	}
	// The fixture has no model provider by design. A nil diagnosis factory
	// selects kindrig's explicit capture-only path: evidence is still indexed
	// and useful, while a real application's typed callback supplies the
	// canonical rig-doctor agent when its model dependency is available.
	runner.Verify = func(resolved apprig.Resolved) error {
		status := fixtureKubernetesStatus(resolved, "deployment",
			resolved.Release+"-fixture", true)
		if status.State != apprig.StateOK {
			return fmt.Errorf("fixture verification: %s", status.Detail)
		}
		return nil
	}
	runner.Probes = fixtureStatusProbes
	return runner, nil
}

func fixtureStatusProbes(resolved apprig.Resolved) apprig.StatusProbes {
	return apprig.StatusProbes{
		Platform: func() apprig.ComponentStatus {
			return fixtureKubernetesStatus(resolved, "--raw", "/readyz", false)
		},
		Workloads: func() apprig.ComponentStatus {
			return fixtureKubernetesStatus(resolved, "deployment", resolved.Release+"-fixture", true)
		},
		Ingress: func() apprig.ComponentStatus {
			return apprig.ComponentStatus{State: apprig.StateAbsent, Detail: "fixture declares no ingress"}
		},
		Collector: func() apprig.ComponentStatus {
			return fixtureKubernetesStatus(resolved, "deployment", resolved.Release+"-fixture", true)
		},
		WAL: func() apprig.ComponentStatus {
			return apprig.ComponentStatus{State: apprig.StateUnknown, Detail: "minimal fixture has no WAL"}
		},
		Bucket: func() apprig.ComponentStatus {
			return apprig.ComponentStatus{State: apprig.StateOK, Detail: resolved.BucketURL + " retained; lifecycle has no delete path"}
		},
		QueryEndpoint: func() apprig.ComponentStatus {
			return fixtureKubernetesStatus(resolved, "service", resolved.Release+"-fixture-query", false)
		},
	}
}

func fixtureKubernetesStatus(resolved apprig.Resolved, resource, name string, rollout bool) apprig.ComponentStatus {
	commands, cleanup, err := kindrig.ClusterCommands(kindrig.CaptureRun, kindrig.PlatformClusterName)
	if err != nil {
		return apprig.ComponentStatus{State: apprig.StateUnknown, Detail: err.Error()}
	}
	defer cleanup()
	var output []byte
	if resource == "--raw" {
		output, err = commands.Run("kubectl", "get", "--raw="+name)
	} else if rollout {
		output, err = commands.Run("kubectl", "rollout", "status", resource+"/"+name,
			"-n", resolved.Namespace, "--timeout=5s")
	} else {
		output, err = commands.Run("kubectl", "get", resource+"/"+name, "-n", resolved.Namespace)
	}
	if err != nil {
		if resource != "--raw" && strings.Contains(string(output), "NotFound") {
			return apprig.ComponentStatus{State: apprig.StateAbsent, Detail: strings.TrimSpace(string(output))}
		}
		return apprig.ComponentStatus{State: apprig.StateDegraded, Detail: strings.TrimSpace(string(output))}
	}
	return apprig.ComponentStatus{State: apprig.StateOK, Detail: strings.TrimSpace(string(output))}
}

func buildLifecycleDeployAgent(root, profile string) (kindrig.DeployAgent, error) {
	binary, cleanup, err := buildLifecycleAgent(root)
	if err != nil {
		return kindrig.DeployAgent{}, err
	}
	return kindrig.DeployAgent{
		Binary: binary, Cleanup: cleanup,
		Profile:  filepath.Join(root, "applications", "catalog", filepath.FromSlash(profile)),
		CoreRoot: filepath.Join(root, "agent-core"),
	}, nil
}

func buildLifecycleAgent(root string) (string, func(), error) {
	directory, err := os.MkdirTemp("", "apprig-agent-*")
	if err != nil {
		return "", nil, err
	}
	binary := filepath.Join(directory, "agent")
	command := exec.Command("go", "build", "-tags", "production", "-o", binary, "./cmd/agent")
	command.Dir = filepath.Join(root, "agent-core")
	if output, err := command.CombinedOutput(); err != nil {
		_ = os.RemoveAll(directory)
		return "", nil, fmt.Errorf("build lifecycle agent: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return binary, func() { _ = os.RemoveAll(directory) }, nil
}

func absoluteRepositoryRoot() (string, error) {
	root, err := findRepositoryRoot()
	if err != nil {
		return "", err
	}
	return filepath.Abs(root)
}

func commandOutput(directory, name string, args ...string) []byte {
	command := exec.Command(name, args...)
	command.Dir = directory
	output, _ := command.Output()
	return output
}

func printApplicationStatus(report apprig.StatusReport) error {
	encoded, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(encoded))
	return nil
}

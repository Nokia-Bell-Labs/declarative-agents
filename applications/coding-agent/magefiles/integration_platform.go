// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/magefiles/kindrig"
)

// codingWorkspaceVolume is the cluster-scoped hostPath volume the kind
// workspace fixture binds. Namespace cleanup does not reach it, so the
// scenario deletes it explicitly before and after each run.
const codingWorkspaceVolume = "coding-agent-kind-workspace"

const codingScenarioCommandTimeout = 3 * time.Minute

// codingScenario is one helm scenario running as a namespaced release on the
// shared da-platform (GH-2215). A platform started by the scenario itself is
// owned and deleted at release; one found running (a release run, the
// integration:all aggregate, or platform:up) is reused and left in place.
type codingScenario struct {
	platform          *kindrig.Platform
	namespace         kindrig.ScenarioNamespace
	environment       codingSmokeEnvironment
	cleanupKubeconfig func()
}

func acquireCodingScenario(evidenceDir string) (*codingScenario, error) {
	platform, err := kindrig.AcquirePlatform(kindrig.PlatformOptions{
		EvidenceDirectory: filepath.Join(evidenceDir, "platform"),
	})
	if err != nil {
		return nil, &codingHelmInfrastructureError{Step: "platform acquisition", Cause: err}
	}
	kubeconfig, cleanupKubeconfig, err := codingKindKubeconfig(platform.Cluster.Name)
	if err != nil {
		platform.Stop(true)
		return nil, &codingHelmInfrastructureError{Step: "kind kubeconfig", Cause: err}
	}
	environment := codingSmokeEnvironment{kubeconfig: kubeconfig}
	namespace, err := kindrig.PrepareScenarioNamespace(
		codingCommandRunner(environment), codingHelmScenario, codingHelmRelease)
	if err != nil {
		cleanupKubeconfig()
		platform.Stop(true)
		return nil, &codingHelmInfrastructureError{Step: "scenario namespace", Cause: err}
	}
	return &codingScenario{
		platform:          platform,
		namespace:         namespace,
		environment:       environment,
		cleanupKubeconfig: cleanupKubeconfig,
	}, nil
}

// release captures namespace evidence when the scenario failed, tears the
// namespace and the workspace volume down, and releases the platform. The
// returned error reports residue the next scenario would otherwise inherit.
func (s *codingScenario) release(failed bool, evidenceDir string) error {
	run := codingCommandRunner(s.environment)
	if failed {
		evidence := kindrig.FailureEvidence{
			Directory:  evidenceDir,
			Namespaces: []string{s.namespace.Name},
			Run:        run,
		}
		if err := evidence.Capture(kindrig.DefaultRun, s.platform.Cluster.Name); err != nil {
			fmt.Printf("coding-agent: capture scenario evidence failed: %v\n", err)
		}
	}
	releaseErr := s.namespace.Release()
	if output, err := run("kubectl", "delete", "persistentvolume", codingWorkspaceVolume,
		"--ignore-not-found=true", "--wait=true", "--timeout=60s"); err != nil {
		releaseErr = errors.Join(releaseErr,
			fmt.Errorf("delete workspace volume %s: %w: %s", codingWorkspaceVolume, err, output))
	}
	s.cleanupKubeconfig()
	s.platform.Stop(failed)
	if releaseErr != nil {
		return fmt.Errorf("release scenario %s: %w", s.namespace.Name, releaseErr)
	}
	return nil
}

// acquireCodingIntegrationPlatform holds one platform across integration:all,
// so its scenarios reuse it instead of each starting their own. It returns a
// no-op when the kind prerequisites are missing; the targets then self-skip.
func acquireCodingIntegrationPlatform() (func(failed bool), error) {
	roots, err := resolveIntegrationRoots()
	if err != nil {
		return nil, err
	}
	if reason := codingHelmSmokeSkipReason(roots, codingSmokeEnvironment{}.run); reason != "" {
		return func(bool) {}, nil
	}
	platform, err := kindrig.AcquirePlatform(kindrig.PlatformOptions{
		EvidenceDirectory: filepath.Join(roots.Application, "build", "kind-evidence",
			kindrig.PlatformClusterName+"-"+time.Now().UTC().Format("20060102T150405Z")),
	})
	if err != nil {
		return nil, &codingHelmInfrastructureError{Step: "platform acquisition", Cause: err}
	}
	return platform.Stop, nil
}

func codingCommandRunner(environment codingSmokeEnvironment) kindrig.CommandRunner {
	return func(name string, args ...string) ([]byte, error) {
		ctx, cancel := context.WithTimeout(context.Background(), codingScenarioCommandTimeout)
		defer cancel()
		return environment.run(ctx, name, args...)
	}
}

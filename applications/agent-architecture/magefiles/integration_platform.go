// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/magefiles/kindrig"
)

const smokeScenarioCommandTimeout = 3 * time.Minute

// smokeScenario is one helm scenario running as a namespaced release on the
// shared da-platform (GH-2215). A platform started by the scenario itself is
// owned and deleted at release; one found running (a release run, the
// integration:all aggregate, or platform:up) is reused and left in place.
type smokeScenario struct {
	platform          *kindrig.Platform
	namespace         kindrig.ScenarioNamespace
	environment       smokeEnvironment
	cleanupKubeconfig func()
	evidenceDir       string
}

func acquireSmokeScenario(applicationRoot, step string) (*smokeScenario, error) {
	evidenceDir := filepath.Join(applicationRoot, "build", "kind-evidence",
		smokeNamespace+"-"+time.Now().UTC().Format("20060102T150405.000000000Z"))
	platform, err := kindrig.AcquirePlatform(kindrig.PlatformOptions{
		EvidenceDirectory: filepath.Join(evidenceDir, "platform"),
	})
	if err != nil {
		return nil, fmt.Errorf("%s platform acquisition: %w", step, err)
	}
	kubeconfig, cleanupKubeconfig, err := smokeKubeconfig(platform.Cluster.Name)
	if err != nil {
		platform.Stop(true)
		return nil, fmt.Errorf("%s kubeconfig: %w", step, err)
	}
	environment := smokeEnvironment{kubeconfig: kubeconfig}
	namespace, err := kindrig.PrepareScenarioNamespace(
		smokeCommandRunner(environment), smokeScenarioName, smokeRelease)
	if err != nil {
		cleanupKubeconfig()
		platform.Stop(true)
		return nil, fmt.Errorf("%s scenario namespace: %w", step, err)
	}
	return &smokeScenario{
		platform:          platform,
		namespace:         namespace,
		environment:       environment,
		cleanupKubeconfig: cleanupKubeconfig,
		evidenceDir:       evidenceDir,
	}, nil
}

// release captures namespace evidence when the scenario failed, tears the
// namespace down, and releases the platform. The returned error reports residue
// the next scenario would otherwise inherit.
func (s *smokeScenario) release(failed bool) error {
	if failed {
		evidence := kindrig.FailureEvidence{
			Directory:  s.evidenceDir,
			Namespaces: []string{s.namespace.Name},
			Run:        smokeCommandRunner(s.environment),
		}
		if err := evidence.Capture(kindrig.DefaultRun, s.platform.Cluster.Name); err != nil {
			fmt.Printf("agent-architecture: capture scenario evidence failed: %v\n", err)
		}
	}
	releaseErr := s.namespace.Release()
	s.cleanupKubeconfig()
	s.platform.Stop(failed)
	if releaseErr != nil {
		return fmt.Errorf("release scenario %s: %w", s.namespace.Name, releaseErr)
	}
	return nil
}

// acquireSmokeIntegrationPlatform holds one platform across integration:all,
// so its scenarios reuse it instead of each starting their own. It returns a
// no-op when the kind prerequisites are missing; the targets then self-skip.
func acquireSmokeIntegrationPlatform() (func(failed bool), error) {
	resolved, err := resolveRootsFromWorkingDirectory()
	if err != nil {
		return func(bool) {}, nil
	}
	if reason := smokeSkipReason(resolved); reason != "" {
		return func(bool) {}, nil
	}
	platform, err := kindrig.AcquirePlatform(kindrig.PlatformOptions{
		EvidenceDirectory: filepath.Join(resolved.Application, "build", "kind-evidence",
			kindrig.PlatformClusterName+"-"+time.Now().UTC().Format("20060102T150405Z")),
	})
	if err != nil {
		return nil, fmt.Errorf("integration platform acquisition: %w", err)
	}
	return platform.Stop, nil
}

func smokeCommandRunner(environment smokeEnvironment) kindrig.CommandRunner {
	return func(name string, args ...string) ([]byte, error) {
		ctx, cancel := context.WithTimeout(context.Background(), smokeScenarioCommandTimeout)
		defer cancel()
		return environment.run(ctx, name, args...)
	}
}

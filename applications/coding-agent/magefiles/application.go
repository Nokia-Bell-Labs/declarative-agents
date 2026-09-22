// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/magefiles/apprig"
	"github.com/Nokia-Bell-Labs/declarative-agents/magefiles/kindrig"
	"github.com/magefile/mage/mg"
)

const (
	codingApplicationRelease   = "coding-agent"
	codingApplicationNamespace = "app-coding-agent"
)

// App is Coding Agent's canonical lifecycle on the shared da-platform.
type App mg.Namespace

func (App) Up() error {
	runner, err := codingApplicationRunner()
	if err != nil {
		return err
	}
	return runner.Up()
}

func (App) Status() error {
	runner, err := codingApplicationRunner()
	if err != nil {
		return err
	}
	report, err := runner.Status()
	if err != nil {
		return err
	}
	return printCodingApplicationStatus(report)
}

func (App) Down() error {
	runner, err := codingApplicationRunner()
	if err != nil {
		return err
	}
	report, err := runner.Down()
	if err != nil {
		return err
	}
	return printCodingApplicationStatus(report)
}

func (App) Diagnose() error {
	runner, err := codingApplicationRunner()
	if err != nil {
		return err
	}
	return runner.Diagnose()
}

// Verify runs the application-owned planner-executor-critic assertion against
// the canonical shared-platform release. Lifecycle mechanics remain in apprig;
// coding behavior remains here.
func (App) Verify() error {
	roots, err := resolveIntegrationRoots()
	if err != nil {
		return err
	}
	kubeconfig, cleanup, err := codingKindKubeconfig(kindrig.PlatformClusterName)
	if err != nil {
		return err
	}
	defer cleanup()
	environment := codingSmokeEnvironment{kubeconfig: kubeconfig}
	if err := seedCodingWorkspaceForNamespace(
		environment, roots.Application, codingApplicationNamespace); err != nil {
		return err
	}
	forwards, err := startCodingForwards(
		environment, codingApplicationNamespace, codingApplicationRelease, false)
	if err != nil {
		return err
	}
	defer forwards.stop()
	if err := verifyCodingHealthEndpoints(forwards.queryURL); err != nil {
		return err
	}
	if err := submitCodingHelmRequest(); err != nil {
		return err
	}
	if err := verifyCodingWorkspaceAndVerdictForNamespace(
		environment, codingApplicationNamespace, codingApplicationRelease); err != nil {
		return err
	}
	return verifyCodingTrace(forwards.queryURL)
}

func (App) Purge() error {
	runner, err := codingApplicationRunner()
	if err != nil {
		return err
	}
	return runner.PurgeData()
}

func codingApplicationRunner() (apprig.Runner, error) {
	roots, err := resolveIntegrationRoots()
	if err != nil {
		return apprig.Runner{}, err
	}
	images, err := resolveCodingHelmImages(roots.Application)
	if err != nil {
		return apprig.Runner{}, err
	}
	runner := apprig.Runner{
		ManifestPath: filepath.Join(roots.Application, "agents", "application.yaml"),
		Binding: apprig.PlatformBinding{
			Cluster: kindrig.PlatformClusterName, NamespacePrefix: "app-",
			IngressHostSuffix: "localhost", BucketURLTemplate: "gs://%s-telemetry",
			ChartName: "coding-agent", ChartPath: filepath.Join(roots.Application, "helm"),
			ValuesPath: filepath.Join(roots.Application, "helm", "ci", "kind-values.yaml"),
			Timeout:    codingHelmInstallTimeout.String(), ApplicationRoot: roots.Application,
		},
		CatalogRoot: roots.Profiles,
		Revision:    images.Revision,
	}
	runner.Prepare = func(resolved apprig.Resolved) (preparation apprig.Preparation, result error) {
		if _, err := kindrig.EnsureFakeGCSApplicationBucket(kindrig.ApplicationBucketRequest{
			Cluster: kindrig.PlatformClusterName, BucketURL: resolved.BucketURL,
		}); err != nil {
			return preparation, err
		}
		namespace := kindrig.ApplicationNamespaceRequest{
			Cluster: kindrig.PlatformClusterName, Namespace: resolved.Namespace,
		}
		created, err := kindrig.EnsureApplicationNamespace(namespace)
		if err != nil {
			return preparation, err
		}
		defer func() {
			if result != nil && created {
				result = errors.Join(result,
					kindrig.DeleteApplicationNamespace(namespace),
					cleanupCodingApplicationWorkspace())
			}
		}()
		kubeconfig, cleanup, err := codingKindKubeconfig(kindrig.PlatformClusterName)
		if err != nil {
			return preparation, err
		}
		defer cleanup()
		environment := codingSmokeEnvironment{kubeconfig: kubeconfig}
		if err := prepareCodingHelmClusterForNamespace(
			environment, kindrig.PlatformClusterName, resolved.Namespace, roots, images, created); err != nil {
			return preparation, err
		}
		if err := Package(); err != nil {
			return preparation, err
		}
		chart, err := codingDeployChartForRelease(roots, resolved.Release)
		if err != nil {
			return preparation, err
		}
		return apprig.Preparation{
			ChartPath: chart, ValuesPath: runner.Binding.ValuesPath,
			Overrides: codingApplicationOverrides(resolved, images),
		}, nil
	}
	runner.DeployAgent = func() (kindrig.DeployAgent, error) {
		return codingApplicationDeployAgent(roots, applierDeployProfileRel)
	}
	runner.UndeployAgent = func() (kindrig.DeployAgent, error) {
		return codingApplicationDeployAgent(roots, applierUndeployProfileRel)
	}
	runner.Verify = func(resolved apprig.Resolved) error {
		kubeconfig, cleanup, err := codingKindKubeconfig(kindrig.PlatformClusterName)
		if err != nil {
			return err
		}
		defer cleanup()
		environment := codingSmokeEnvironment{kubeconfig: kubeconfig}
		for _, component := range []string{"planner", "executor", "critic", "collector"} {
			if err := runCodingSmokeCommand(environment, codingHelmReadyTimeout,
				"kubectl", "rollout", "status",
				"deployment/"+resolved.Release+"-coding-agent-"+component,
				"-n", resolved.Namespace, "--timeout=90s"); err != nil {
				return err
			}
		}
		return nil
	}
	runner.Probes = codingApplicationProbes
	runner.AfterDown = func(apprig.Resolved) error {
		return cleanupCodingApplicationWorkspace()
	}
	return runner, nil
}

func codingApplicationDeployAgent(roots integrationRoots, profile string) (kindrig.DeployAgent, error) {
	binary, cleanup, err := buildAgent(roots.Core)
	if err != nil {
		return kindrig.DeployAgent{}, err
	}
	return kindrig.DeployAgent{
		Binary: binary, Cleanup: cleanup, CoreRoot: roots.Core,
		Profile: filepath.Join(roots.Profiles, filepath.FromSlash(profile)),
	}, nil
}

func codingApplicationOverrides(resolved apprig.Resolved, images codingHelmImages) string {
	repository, tag := splitCodingImageRef(images.Agent)
	collectorRepository, collectorTag := splitCodingImageRef(codingHelmCollectorImage)
	bucket := strings.TrimPrefix(resolved.BucketURL, "gs://")
	return fmt.Sprintf(`image:
  repository: %q
  tag: %q
collector:
  image:
    repository: %q
    tag: %q
  storage:
    backend: object
    bucketName: %q
    endpoint: %q
    prefix: %q
    application: %q
    namespace: %q
`, repository, tag, collectorRepository, collectorTag,
		bucket, kindrig.FakeGCSEndpoint, resolved.ObjectPrefix,
		resolved.Application, resolved.Namespace)
}

func codingApplicationProbes(resolved apprig.Resolved) apprig.StatusProbes {
	return apprig.StatusProbes{
		Platform: func() apprig.ComponentStatus {
			return codingApplicationResourceStatus(resolved, "--raw", "/readyz", false)
		},
		Workloads: func() apprig.ComponentStatus {
			return codingApplicationResourceStatus(
				resolved, "deployment", resolved.Release+"-coding-agent-planner", true)
		},
		Ingress: func() apprig.ComponentStatus {
			return codingApplicationResourceStatus(
				resolved, "ingress", resolved.Release+"-coding-agent", false)
		},
		Collector: func() apprig.ComponentStatus {
			return codingApplicationResourceStatus(resolved, "service", resolved.CollectorService, false)
		},
		WAL: func() apprig.ComponentStatus {
			return codingApplicationResourceStatus(
				resolved, "persistentvolumeclaim", resolved.CollectorService+"-wal", false)
		},
		Bucket: func() apprig.ComponentStatus {
			exists, err := kindrig.FakeGCSApplicationBucketStatus(kindrig.ApplicationBucketRequest{
				Cluster: kindrig.PlatformClusterName, BucketURL: resolved.BucketURL,
			})
			switch {
			case err != nil:
				return apprig.ComponentStatus{State: apprig.StateDegraded, Detail: err.Error()}
			case !exists:
				return apprig.ComponentStatus{State: apprig.StateAbsent, Detail: resolved.BucketURL}
			default:
				return apprig.ComponentStatus{State: apprig.StateOK, Detail: resolved.BucketURL}
			}
		},
		QueryEndpoint: func() apprig.ComponentStatus {
			return codingApplicationResourceStatus(resolved, "service", resolved.CollectorService, false)
		},
	}
}

func codingApplicationResourceStatus(
	resolved apprig.Resolved, resource, name string, rollout bool,
) apprig.ComponentStatus {
	kubeconfig, cleanup, err := codingKindKubeconfig(kindrig.PlatformClusterName)
	if err != nil {
		return apprig.ComponentStatus{State: apprig.StateUnknown, Detail: err.Error()}
	}
	defer cleanup()
	environment := codingSmokeEnvironment{kubeconfig: kubeconfig}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	var output []byte
	if resource == "--raw" {
		output, err = environment.run(ctx, "kubectl", "get", "--raw="+name)
	} else if rollout {
		output, err = environment.run(ctx, "kubectl", "rollout", "status",
			resource+"/"+name, "-n", resolved.Namespace, "--timeout=10s")
	} else {
		output, err = environment.run(ctx, "kubectl", "get",
			resource+"/"+name, "-n", resolved.Namespace)
	}
	if err != nil {
		if strings.Contains(string(output), "NotFound") {
			return apprig.ComponentStatus{State: apprig.StateAbsent, Detail: strings.TrimSpace(string(output))}
		}
		return apprig.ComponentStatus{State: apprig.StateDegraded, Detail: strings.TrimSpace(string(output))}
	}
	return apprig.ComponentStatus{State: apprig.StateOK, Detail: strings.TrimSpace(string(output))}
}

func cleanupCodingApplicationWorkspace() error {
	kubeconfig, cleanup, err := codingKindKubeconfig(kindrig.PlatformClusterName)
	if err != nil {
		return err
	}
	defer cleanup()
	environment := codingSmokeEnvironment{kubeconfig: kubeconfig}
	return runCodingSmokeCommand(environment, 60*time.Second,
		"kubectl", "delete", "persistentvolume", codingWorkspaceVolume,
		"--ignore-not-found=true", "--wait=true", "--timeout=45s")
}

func printCodingApplicationStatus(report apprig.StatusReport) error {
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}

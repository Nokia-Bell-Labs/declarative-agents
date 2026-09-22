// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/magefiles/apprig"
	"github.com/Nokia-Bell-Labs/declarative-agents/magefiles/kindrig"
	"github.com/magefile/mage/mg"
)

const (
	architectureApplicationRelease   = "agent-architecture"
	architectureApplicationNamespace = "app-agent-architecture"
)

// App is the canonical persistent-platform lifecycle. Demo remains a
// lightweight development compatibility mode and no longer owns the deployed
// application contract.
type App mg.Namespace

func (App) Up() error {
	runner, err := architectureApplicationRunner()
	if err != nil {
		return err
	}
	return runner.Up()
}

func (App) Status() error {
	runner, err := architectureApplicationRunner()
	if err != nil {
		return err
	}
	report, err := runner.Status()
	if err != nil {
		return err
	}
	return printArchitectureApplicationStatus(report)
}

func (App) Down() error {
	runner, err := architectureApplicationRunner()
	if err != nil {
		return err
	}
	report, err := runner.Down()
	if err != nil {
		return err
	}
	return printArchitectureApplicationStatus(report)
}

func (App) Diagnose() error {
	runner, err := architectureApplicationRunner()
	if err != nil {
		return err
	}
	return runner.Diagnose()
}

// Exit runs the catalog-owned lifecycle-exit client through a temporary
// context-bound port-forward to the curator Service. The control server keeps
// its loopback-only unauthenticated exit policy; shared ingress therefore
// exposes health but refuses destructive remote requests.
func (App) Exit() error {
	roots, err := resolveRootsFromWorkingDirectory()
	if err != nil {
		return err
	}
	kubeconfig, cleanupKubeconfig, err := smokeKubeconfig(kindrig.PlatformClusterName)
	if err != nil {
		return err
	}
	defer cleanupKubeconfig()
	localPort, err := freeLocalPort()
	if err != nil {
		return err
	}
	environment := smokeEnvironment{kubeconfig: kubeconfig}
	service := architectureApplicationRelease + "-agent-architecture-curator"
	forward, err := forwardNamespacedService(
		environment, architectureApplicationNamespace, service, localPort+":18082")
	if err != nil {
		return err
	}
	defer forward.stop()
	controlURL := "http://127.0.0.1:" + localPort
	if err := waitHTTP200(controlURL+"/api/lifecycle/health", 20*time.Second); err != nil {
		return err
	}
	binary, cleanupBinary, err := buildApplierBinary(roots.Core)
	if err != nil {
		return err
	}
	defer cleanupBinary()
	command := exec.Command(binary,
		"--profile", filepath.Join(roots.Catalog, "agents", "lifecycle-exit", "profile.yaml"),
		"--directory", roots.Catalog, "--core-root", roots.Core)
	command.Env = append(os.Environ(),
		"LIFECYCLE_EXIT_HOST=127.0.0.1", "LIFECYCLE_EXIT_PORT="+localPort)
	command.Stdout, command.Stderr = os.Stdout, os.Stderr
	return command.Run()
}

func (App) Purge() error {
	runner, err := architectureApplicationRunner()
	if err != nil {
		return err
	}
	return runner.PurgeData()
}

func architectureApplicationRunner() (apprig.Runner, error) {
	roots, err := resolveRootsFromWorkingDirectory()
	if err != nil {
		return apprig.Runner{}, err
	}
	runner := apprig.Runner{
		ManifestPath: filepath.Join(roots.Application, "agents", "application.yaml"),
		Binding: apprig.PlatformBinding{
			Cluster: kindrig.PlatformClusterName, NamespacePrefix: "app-",
			IngressHostSuffix: "localhost", BucketURLTemplate: "gs://%s-telemetry",
			ChartName: "agent-architecture", ChartPath: filepath.Join(roots.Application, "helm"),
			ValuesPath: filepath.Join(roots.Application, "helm", "ci", "kind-values.yaml"),
			Timeout:    smokeInstallTimeout.String(), ApplicationRoot: roots.Application,
		},
		CatalogRoot: roots.Catalog,
		Revision:    mustGitRevision(roots.Application),
	}
	runner.Prepare = func(resolved apprig.Resolved) (preparation apprig.Preparation, result error) {
		bucket := kindrig.ApplicationBucketRequest{
			Cluster: kindrig.PlatformClusterName, BucketURL: resolved.BucketURL,
		}
		if _, err := kindrig.EnsureFakeGCSApplicationBucket(bucket); err != nil {
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
				result = errors.Join(result, kindrig.DeleteApplicationNamespace(namespace))
			}
		}()
		kubeconfig, cleanup, err := smokeKubeconfig(kindrig.PlatformClusterName)
		if err != nil {
			return preparation, err
		}
		defer cleanup()
		environment := smokeEnvironment{kubeconfig: kubeconfig}
		if err := prepareSmokeCluster(environment, kindrig.PlatformClusterName, roots); err != nil {
			return preparation, err
		}
		destination := filepath.Join(
			kindrig.DeployRenderDirectory(roots.Application, resolved.Release), "chart")
		chart, err := packageHelmChart(
			filepath.Join(roots.Application, "helm"), roots.Catalog, destination)
		if err != nil {
			return preparation, err
		}
		if err := clearCuratorUIShards(environment, resolved.Namespace, resolved.Release); err != nil {
			return preparation, err
		}
		shards, err := provisionCuratorUIShards(
			environment, roots.Catalog, resolved.Namespace, resolved.Release)
		if err != nil {
			return preparation, err
		}
		return apprig.Preparation{
			ChartPath: chart, ValuesPath: runner.Binding.ValuesPath,
			Overrides:     architectureApplicationOverrides(resolved, shards),
			OwnsNamespace: created,
		}, nil
	}
	runner.DeployAgent = func() (kindrig.DeployAgent, error) {
		return architectureApplicationDeployAgent(roots, applierDeployProfileRel)
	}
	runner.UndeployAgent = func() (kindrig.DeployAgent, error) {
		return architectureApplicationDeployAgent(roots, applierUndeployProfileRel)
	}
	// No model provider is part of the local platform contract. Diagnose still
	// runs kindrig's canonical evidence capture; applications that configure a
	// provider supply the rig-doctor factory through this typed hook.
	runner.Verify = func(resolved apprig.Resolved) error {
		status := architectureApplicationResourceStatus(
			resolved, "deployment", resolved.CollectorService, true)
		if status.State != apprig.StateOK {
			return fmt.Errorf("collector rollout: %s", status.Detail)
		}
		return nil
	}
	runner.Probes = architectureApplicationProbes
	return runner, nil
}

func architectureApplicationDeployAgent(roots roots, profile string) (kindrig.DeployAgent, error) {
	binary, cleanup, err := buildApplierBinary(roots.Core)
	if err != nil {
		return kindrig.DeployAgent{}, err
	}
	return kindrig.DeployAgent{
		Binary: binary, Cleanup: cleanup, CoreRoot: roots.Core,
		Profile: filepath.Join(roots.Catalog, filepath.FromSlash(profile)),
	}, nil
}

func architectureApplicationOverrides(resolved apprig.Resolved, shards []string) string {
	repository, tag := splitImageRef(smokeCollectorImage)
	bucket := strings.TrimPrefix(resolved.BucketURL, "gs://")
	var document strings.Builder
	fmt.Fprintf(&document, `image:
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
`, repository, tag, repository, tag,
		bucket, kindrig.FakeGCSEndpoint, resolved.ObjectPrefix,
		resolved.Application, resolved.Namespace)
	if len(shards) == 0 {
		document.WriteString("curatorUI:\n  shards: []\n")
		return document.String()
	}
	document.WriteString("curatorUI:\n  shards:\n")
	for _, shard := range shards {
		fmt.Fprintf(&document, "    - %q\n", shard)
	}
	return document.String()
}

func architectureApplicationProbes(resolved apprig.Resolved) apprig.StatusProbes {
	return apprig.StatusProbes{
		Platform: func() apprig.ComponentStatus {
			return architectureApplicationResourceStatus(resolved, "--raw", "/readyz", false)
		},
		Workloads: func() apprig.ComponentStatus {
			return architectureApplicationResourceStatus(resolved, "deployment", resolved.CollectorService, true)
		},
		Ingress: func() apprig.ComponentStatus {
			return architectureApplicationResourceStatus(
				resolved, "ingress", resolved.Release+"-agent-architecture", false)
		},
		Collector: func() apprig.ComponentStatus {
			return architectureApplicationResourceStatus(resolved, "service", resolved.CollectorService, false)
		},
		WAL: func() apprig.ComponentStatus {
			return architectureApplicationResourceStatus(
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
			return architectureApplicationResourceStatus(resolved, "service", resolved.CollectorService, false)
		},
	}
}

func architectureApplicationResourceStatus(
	resolved apprig.Resolved, resource, name string, rollout bool,
) apprig.ComponentStatus {
	kubeconfig, cleanup, err := smokeKubeconfig(kindrig.PlatformClusterName)
	if err != nil {
		return apprig.ComponentStatus{State: apprig.StateUnknown, Detail: err.Error()}
	}
	defer cleanup()
	environment := smokeEnvironment{kubeconfig: kubeconfig}
	var output []byte
	if resource == "--raw" {
		output, err = environment.run(context.Background(), "kubectl", "get", "--raw="+name)
	} else if rollout {
		output, err = environment.run(context.Background(), "kubectl", "rollout", "status",
			resource+"/"+name, "-n", resolved.Namespace, "--timeout=10s")
	} else {
		output, err = environment.run(context.Background(), "kubectl", "get",
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

func printArchitectureApplicationStatus(report apprig.StatusReport) error {
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}

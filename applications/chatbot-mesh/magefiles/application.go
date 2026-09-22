// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/magefiles/apprig"
	"github.com/Nokia-Bell-Labs/declarative-agents/magefiles/kindrig"
	"github.com/magefile/mage/mg"
)

const (
	chatbotApplicationRelease   = "chatbot-mesh"
	chatbotApplicationNamespace = "app-chatbot-mesh"
	chatbotApplicationHost      = "chatbot-mesh.localhost"
	chatbotObserverHost         = "observer.chatbot-mesh.localhost"
	chatbotCollectorHost        = "telemetry.chatbot-mesh.localhost"
)

// App is Chatbot Mesh's canonical lifecycle on shared da-platform.
type App mg.Namespace

func (App) Up() error {
	runner, err := chatbotApplicationRunner()
	if err != nil {
		return err
	}
	return runner.Up()
}

func (App) Status() error {
	runner, err := chatbotApplicationRunner()
	if err != nil {
		return err
	}
	report, err := runner.Status()
	if err != nil {
		return err
	}
	return printChatbotApplicationStatus(report)
}

func (App) Down() error {
	runner, err := chatbotApplicationRunner()
	if err != nil {
		return err
	}
	report, err := runner.Down()
	if err != nil {
		return err
	}
	return printChatbotApplicationStatus(report)
}

func (App) Diagnose() error {
	runner, err := chatbotApplicationRunner()
	if err != nil {
		return err
	}
	return runner.Diagnose()
}

// Seed preserves the application-owned corpus behavior by forwarding only the
// app-local Chroma Service and invoking the existing canonical ingest wrapper.
func (App) Seed() error {
	root, err := chatbotApplicationRoot()
	if err != nil {
		return err
	}
	commands, cleanup, err := kindrig.ClusterCommands(kindrig.CaptureRun, kindrig.PlatformClusterName)
	if err != nil {
		return err
	}
	defer cleanup()
	if output, err := commands.Run("kubectl", "config", "set-context",
		"--current", "--namespace", chatbotApplicationNamespace); err != nil {
		return fmt.Errorf("bind application namespace: %w: %s", err, output)
	}
	stop, err := kubectlPortForwardWithCommands(commands,
		"service/"+chatbotApplicationRelease+"-chatbot-mesh-rag0-chroma", 8000)
	if err != nil {
		return err
	}
	defer stop()
	if err := waitHTTPStatus(chromaHeartbeatURL, http.StatusOK, 30*time.Second); err != nil {
		return err
	}
	previous, err := os.Getwd()
	if err != nil {
		return err
	}
	if err := os.Chdir(root); err != nil {
		return err
	}
	defer func() { _ = os.Chdir(previous) }()
	return Seed()
}

// Verify exercises the browser route and a real served chat turn; retrieval,
// ranking, prompting, and answer policy remain application-owned.
func (App) Verify() error {
	if err := waitHTTPStatus(
		"http://"+chatbotApplicationHost+"/", http.StatusOK, 30*time.Second); err != nil {
		return err
	}
	if err := waitHTTPStatus(
		"http://"+chatbotObserverHost+"/", http.StatusOK, 30*time.Second); err != nil {
		return err
	}
	return assertSmokeChatServed("http://" + chatbotApplicationHost + "/api/v1/chat")
}

func (App) Purge(confirmation string) error {
	runner, err := chatbotApplicationRunner()
	if err != nil {
		return err
	}
	return runner.PurgeData(confirmation)
}

func chatbotApplicationRunner() (apprig.Runner, error) {
	root, err := chatbotApplicationRoot()
	if err != nil {
		return apprig.Runner{}, err
	}
	catalogRoot, err := resolveCatalogRoot("chatbot-mesh application", root)
	if err != nil {
		return apprig.Runner{}, err
	}
	images, err := resolveChatbotIntegrationImages(root)
	if err != nil {
		return apprig.Runner{}, err
	}
	runner := apprig.Runner{
		ManifestPath: filepath.Join(root, "agents", "application.yaml"),
		Binding: apprig.PlatformBinding{
			Cluster: kindrig.PlatformClusterName, NamespacePrefix: "app-",
			IngressHostSuffix: "localhost", BucketURLTemplate: "gs://%s-telemetry",
			ChartName: "chatbot-mesh", ChartPath: filepath.Join(root, "helm"),
			ValuesPath: filepath.Join(root, "helm", "ci", chatbotDemoValuesFile),
			Timeout:    helmLLMInstallTimeout.String(), ApplicationRoot: root,
		},
		CatalogRoot: catalogRoot,
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
				result = errors.Join(result, kindrig.DeleteApplicationNamespace(namespace))
			}
		}()

		chartDir := applicationChartDir(root)
		if err := buildSmokeRuntimeImage(demoCoreRoot(root), images.Runtime); err != nil {
			return preparation, err
		}
		staged, cleanupStaged, err := stageSmokeChart(chartDir, root)
		if err != nil {
			return preparation, err
		}
		cleanups := []func(){cleanupStaged}
		cleanupOwned := func() {
			for index := len(cleanups) - 1; index >= 0; index-- {
				cleanups[index]()
			}
		}
		success := false
		defer func() {
			if !success {
				cleanupOwned()
			}
		}()
		assets, cleanupAssets, err := externalizeUIAssets(staged, resolved.Release)
		if err != nil {
			return preparation, err
		}
		cleanups = append(cleanups, cleanupAssets)
		archive, cleanupArchive, err := packageApplierChart(staged)
		if err != nil {
			return preparation, err
		}
		cleanups = append(cleanups, cleanupArchive)
		dependencies, err := smokeDependencyImages(chartDir)
		if err != nil {
			return preparation, err
		}
		if err := ensureChatbotApplicationDependencyImages(dependencies); err != nil {
			return preparation, err
		}
		commands, cleanupCommands, err := kindrig.ClusterCommands(
			kindrig.CaptureRun, kindrig.PlatformClusterName)
		if err != nil {
			return preparation, err
		}
		defer cleanupCommands()
		if output, err := commands.Run("kubectl", "config", "set-context",
			"--current", "--namespace", resolved.Namespace); err != nil {
			return preparation, fmt.Errorf("bind application namespace: %w: %s", err, output)
		}
		if err := loadKindImageWithCommands(
			commands, kindrig.PlatformClusterName, images.Runtime); err != nil {
			return preparation, err
		}
		if err := loadIntegrationDependencyImages(
			commands, kindrig.PlatformClusterName, dependencies); err != nil {
			return preparation, err
		}
		if err := provisionExternalUIAssets(commands.Run, assets); err != nil {
			return preparation, err
		}
		overrides := chatbotApplicationOverrides(resolved, images.Runtime, assets)
		overridesPath := kindrig.DeployOverridesPath(root, resolved.Release)
		if err := os.MkdirAll(filepath.Dir(overridesPath), 0o755); err != nil {
			return preparation, err
		}
		if err := os.WriteFile(overridesPath, []byte(overrides), 0o644); err != nil {
			return preparation, err
		}
		valuesPath := filepath.Join(staged, "ci", chatbotDemoValuesFile)
		measured, err := measureHelmReleaseBudget(
			resolved.Release, staged, archive,
			[]string{"--namespace", resolved.Namespace, "--values", valuesPath, "-f", overridesPath})
		if err != nil {
			return preparation, err
		}
		fmt.Printf("app:up release budget PASS - %s\n", measured.String())
		success = true
		return apprig.Preparation{
			ChartPath: staged, ValuesPath: valuesPath,
			Overrides: overrides, Cleanup: func() error { cleanupOwned(); return nil },
			OwnsNamespace: created,
		}, nil
	}
	runner.DeployAgent = func() (kindrig.DeployAgent, error) {
		return chatbotApplicationDeployAgent(root, applierDeployProfileRel)
	}
	runner.UndeployAgent = func() (kindrig.DeployAgent, error) {
		return chatbotApplicationDeployAgent(root, applierUndeployProfileRel)
	}
	runner.Verify = func(resolved apprig.Resolved) error {
		commands, cleanup, err := kindrig.ClusterCommands(
			kindrig.CaptureRun, kindrig.PlatformClusterName)
		if err != nil {
			return err
		}
		defer cleanup()
		if output, err := commands.Run("kubectl", "wait",
			"--for=condition=Available", "deployment", "--all",
			"-n", resolved.Namespace, "--timeout=180s"); err != nil {
			return fmt.Errorf("application rollouts: %w: %s", err, output)
		}
		for _, endpoint := range []string{
			"http://" + chatbotApplicationHost + "/",
			"http://" + chatbotObserverHost + "/",
			"http://" + chatbotCollectorHost + "/query/traces?page_size=1",
		} {
			if err := waitHTTPStatus(endpoint, http.StatusOK, 30*time.Second); err != nil {
				return err
			}
		}
		return nil
	}
	runner.Probes = chatbotApplicationProbes
	runner.Purge = func(resolved apprig.Resolved, confirmation string) error {
		agent, err := chatbotDeployAgent(
			root, "agents/application-purge/profile.yaml")
		if err != nil {
			return err
		}
		return apprig.RunApplicationPurge(
			resolved, confirmation,
			apprig.PurgeBinding{
				Endpoint:       kindrig.FakeGCSHostEndpoint,
				AuditDirectory: filepath.Join(root, "build", "purge"),
			},
			apprig.PurgeAgent{
				Binary: agent.agent.Binary, Cleanup: agent.agent.Cleanup,
				CoreRoot: agent.agent.CoreRoot, Profile: agent.agent.Profile,
			},
		)
	}
	return runner, nil
}

func chatbotApplicationDeployAgent(root, profile string) (kindrig.DeployAgent, error) {
	agent, err := chatbotDeployAgent(root, profile)
	if err != nil {
		return kindrig.DeployAgent{}, err
	}
	return agent.agent, nil
}

func ensureChatbotApplicationDependencyImages(images []string) error {
	for _, image := range images {
		source := image
		if image == helmLLMOllamaImage {
			source = helmLLMOllamaSourceImage
		}
		if _, err := inspectHostImageID(runHelmSmokeCommand, source); err == nil {
			continue
		}
		output, err := runHelmSmokeCommand(
			"docker", "pull", "--platform", "linux/"+runtime.GOARCH, source)
		if err != nil {
			return fmt.Errorf("pull app:up dependency %s: %w: %s",
				source, err, strings.TrimSpace(string(output)))
		}
	}
	return nil
}

func chatbotApplicationOverrides(
	resolved apprig.Resolved, image string, assets []externalUIAsset,
) string {
	repository, tag := splitImageRef(image)
	bucket := strings.TrimPrefix(resolved.BucketURL, "gs://")
	var document strings.Builder
	fmt.Fprintf(&document, "image:\n  repository: %q\n  tag: %q\n  pullPolicy: %q\n",
		repository, tag, "Never")
	document.WriteString(`chroma:
  image:
    digest: ""
    pullPolicy: Never
dolt:
  image:
    digest: ""
    pullPolicy: Never
observer:
  proxy:
    image:
      digest: ""
      pullPolicy: Never
`)
	fmt.Fprintf(&document, `ingress:
  enabled: true
  className: %q
  host: %q
  observerHost: %q
  collectorHost: %q
`, chatbotDemoIngressClass, chatbotApplicationHost, chatbotObserverHost, chatbotCollectorHost)
	fmt.Fprintf(&document, `collector:
  externalOTLPEndpoint: ""
  storage:
    backend: object
    bucketName: %q
    endpoint: %q
    prefix: %q
    application: %q
    namespace: %q
`, bucket, kindrig.FakeGCSEndpoint, resolved.ObjectPrefix,
		resolved.Application, resolved.Namespace)
	for _, asset := range assets {
		if asset.Component == "collector" {
			fmt.Fprintf(&document, "  uiArchiveConfigMap: %q\n  uiArchiveChecksum: %q\n",
				asset.ConfigMapName, asset.Checksum)
		}
	}
	for _, asset := range assets {
		if asset.Component == "collector" {
			continue
		}
		fmt.Fprintf(&document, "%s:\n  uiArchiveConfigMap: %q\n  uiArchiveChecksum: %q\n",
			asset.Component, asset.ConfigMapName, asset.Checksum)
	}
	return document.String()
}

func chatbotApplicationProbes(resolved apprig.Resolved) apprig.StatusProbes {
	return apprig.StatusProbes{
		Platform: func() apprig.ComponentStatus {
			return chatbotApplicationResourceStatus(resolved, "--raw", "/readyz")
		},
		Workloads: func() apprig.ComponentStatus {
			return chatbotApplicationResourceStatus(
				resolved, "deployment", resolved.Release+"-chatbot-mesh-chatbot")
		},
		Ingress: func() apprig.ComponentStatus {
			return chatbotApplicationResourceStatus(
				resolved, "ingress", resolved.Release+"-chatbot-mesh-chatbot")
		},
		Collector: func() apprig.ComponentStatus {
			return chatbotApplicationResourceStatus(resolved, "service", resolved.CollectorService)
		},
		WAL: func() apprig.ComponentStatus {
			return chatbotApplicationResourceStatus(
				resolved, "persistentvolumeclaim", resolved.CollectorService+"-wal")
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
			return chatbotApplicationResourceStatus(resolved, "service", resolved.CollectorService)
		},
	}
}

func chatbotApplicationResourceStatus(
	resolved apprig.Resolved, resource, name string,
) apprig.ComponentStatus {
	commands, cleanup, err := kindrig.ClusterCommands(kindrig.CaptureRun, kindrig.PlatformClusterName)
	if err != nil {
		return apprig.ComponentStatus{State: apprig.StateUnknown, Detail: err.Error()}
	}
	defer cleanup()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	var output []byte
	if resource == "--raw" {
		output, err = commands.RunContext(ctx, "kubectl", "get", "--raw="+name)
	} else {
		output, err = commands.RunContext(ctx, "kubectl", "get",
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

func chatbotApplicationRoot() (string, error) {
	root, err := os.Getwd()
	if err != nil {
		return "", err
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return "", err
	}
	if filepath.Base(root) == "magefiles" {
		root = filepath.Dir(root)
	}
	if _, err := os.Stat(filepath.Join(root, "agents", "application.yaml")); err != nil {
		return "", fmt.Errorf("chatbot-mesh application root %s: %w", root, err)
	}
	return root, nil
}

func printChatbotApplicationStatus(report apprig.StatusReport) error {
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}

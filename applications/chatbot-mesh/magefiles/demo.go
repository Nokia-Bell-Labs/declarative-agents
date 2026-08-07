// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/magefiles/kindrig"
	"github.com/magefile/mage/mg"
)

const (
	chatbotDemoCluster      = "da-chatbot-mesh-demo"
	chatbotDemoRelease      = "demo"
	chatbotDemoHost         = "chatbot.localhost"
	chatbotDemoIngressClass = "traefik"
	chatbotDemoValuesFile   = "kind-values.yaml"
)

type Demo mg.Namespace

// Doctor checks the shared ENG01 toolchain and host resources without mutation.
func Doctor() error {
	return kindrig.Doctor()
}

// Up creates or reuses the persistent demo cluster and deploys chatbot-mesh.
func (Demo) Up() error {
	if err := Doctor(); err != nil {
		return fmt.Errorf("demo requested but preflight failed: %w", err)
	}
	root, err := os.Getwd()
	if err != nil {
		return err
	}
	coreRoot := demoCoreRoot(root)
	chartDir := applicationChartDir(root)
	models, err := demoRequiredModels(chartDir)
	if err != nil {
		return err
	}
	if reason := chromaOllamaSkipReasonForModels(models); reason != "" {
		return fmt.Errorf("demo requires host Ollama with the chart models installed: %s", reason)
	}
	images, err := resolveChatbotIntegrationImages(root)
	if err != nil {
		return err
	}
	if err := buildSmokeRuntimeImage(coreRoot, images.Runtime); err != nil {
		return err
	}
	staged, cleanup, err := stageSmokeChart(chartDir, root)
	if err != nil {
		return err
	}
	defer cleanup()
	// The shipped UIs travel out of the release, as they do on every other kind
	// path. Left inside, the release Secret projects past the 1 MiB Kubernetes
	// limit and the install dies creating it (GH-1475).
	assets, cleanupAssets, err := externalizeUIAssets(staged, chatbotDemoRelease)
	if err != nil {
		return err
	}
	defer cleanupAssets()
	chartArchive, cleanupArchive, err := packageApplierChart(staged)
	if err != nil {
		return err
	}
	defer cleanupArchive()
	dependencies, err := smokeDependencyImages(chartDir)
	if err != nil {
		return err
	}
	for _, image := range dependencies {
		command := exec.Command("docker", "pull", "--platform", "linux/"+runtime.GOARCH, image)
		if output, pullErr := command.CombinedOutput(); pullErr != nil {
			return fmt.Errorf("pull demo dependency %s: %w: %s",
				image, pullErr, strings.TrimSpace(string(output)))
		}
	}
	config := filepath.Join(chartDir, "ci", "kind-demo-config.yaml")
	return kindrig.DemoUp(kindrig.DefaultRun, chatbotDemoCluster, config,
		120*time.Second, func(kindrig.Cluster) error {
			if err := loadKindImage(chatbotDemoCluster, images.Runtime); err != nil {
				return err
			}
			for _, image := range dependencies {
				if err := loadSmokeDependencyImage(chatbotDemoCluster, image); err != nil {
					return err
				}
			}
			if err := kindrig.InstallIngress(kindrig.DefaultCommandRun, chatbotDemoCluster); err != nil {
				return err
			}
			// EnsureCluster reuses a pre-existing demo cluster without switching
			// contexts, so pin the kubeconfig before any kubectl or helm mutation
			// rather than trusting the ambient current context (GH-1341).
			unbindKubeconfig, err := bindClusterKubeconfig(chatbotDemoCluster)
			if err != nil {
				return err
			}
			defer unbindKubeconfig()
			if err := provisionExternalUIAssets(assets); err != nil {
				return err
			}
			if err := installDemoRelease(staged, chartArchive, images.Runtime, assets); err != nil {
				return err
			}
			if err := waitHTTPStatus("http://"+chatbotDemoHost+"/",
				http.StatusOK, 30*time.Second); err != nil {
				return fmt.Errorf("chatbot demo ingress not ready: %w", err)
			}
			if err := waitHTTPStatus("http://observer.localhost/", http.StatusOK, 30*time.Second); err != nil {
				return fmt.Errorf("observer demo ingress not ready: %w", err)
			}
			fmt.Printf("demo: revision %s ready at http://%s/\n",
				images.Revision, chatbotDemoHost)
			fmt.Println("demo: fleet observer at http://observer.localhost/")
			return nil
		})
}

// demoValueArgs builds the demo install arguments. The browser demo reuses the
// host Ollama and its cached models, keeping it within a laptop budget and
// independent of in-pod model-registry trust; integration:helmLLMTier separately
// proves the optional self-contained in-cluster tier (GH-1321). The external UI
// asset values keep the release Secret inside its budget (GH-1475).
func demoValueArgs(staged, image string, assets []externalUIAsset) []string {
	repository, tag := splitImageRef(image)
	args := []string{
		"--values", filepath.Join(staged, "ci", chatbotDemoValuesFile),
		"--set", "image.repository=" + repository,
		"--set-string", "image.tag=" + tag,
		"--set", "image.pullPolicy=Never",
		"--set", "ingress.enabled=true",
		"--set", "ingress.className=" + chatbotDemoIngressClass,
		"--set", "ingress.host=" + chatbotDemoHost,
	}
	return append(args, externalUIAssetValueArgs(assets)...)
}

// installDemoRelease measures the projected release Secret before contacting the
// cluster, then upgrades or installs. Without the gate an over-budget release
// reaches the API server and fails there as an opaque Secret size error, after
// the cluster and images are already built (GH-1475).
func installDemoRelease(staged, chartArchive, image string, assets []externalUIAsset) error {
	valueArgs := demoValueArgs(staged, image, assets)
	measured, err := measureHelmReleaseBudget(chatbotDemoRelease, staged, chartArchive, valueArgs)
	if err != nil {
		return err
	}
	fmt.Printf("demo: release budget PASS - %s\n", measured.String())
	args := append([]string{"upgrade", "--install", chatbotDemoRelease, staged}, valueArgs...)
	args = append(args, "--wait", "--timeout", helmLLMInstallTimeout.String())
	if output, err := exec.Command("helm", args...).CombinedOutput(); err != nil {
		return fmt.Errorf("helm demo install: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

// Down deletes only the chatbot-mesh demo cluster.
func (Demo) Down() error {
	if err := Doctor(); err != nil {
		return fmt.Errorf("demo teardown requested but preflight failed: %w", err)
	}
	return kindrig.DemoDown(kindrig.DefaultRun, chatbotDemoCluster)
}

func demoRequiredModels(chartDir string) ([]string, error) {
	var values struct {
		Ollama struct {
			Models struct {
				Embedding string   `yaml:"embedding"`
				Chat      []string `yaml:"chat"`
				Tier      string   `yaml:"tier"`
			} `yaml:"models"`
		} `yaml:"ollama"`
	}
	if err := readIntegrationYAML(filepath.Join(chartDir, "values.yaml"),
		"chart values", &values); err != nil {
		return nil, err
	}
	candidates := append([]string{values.Ollama.Models.Embedding},
		values.Ollama.Models.Chat...)
	candidates = append(candidates, values.Ollama.Models.Tier)
	seen := map[string]bool{}
	models := make([]string, 0, len(candidates))
	for _, model := range candidates {
		if model != "" && !seen[model] {
			seen[model] = true
			models = append(models, model)
		}
	}
	if len(models) == 0 {
		return nil, fmt.Errorf("chart values declare no Ollama models for the demo")
	}
	return models, nil
}

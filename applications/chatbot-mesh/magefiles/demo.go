// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"fmt"
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
	chatbotDemoCluster = "da-chatbot-mesh-demo"
	chatbotDemoRelease = "demo"
	chatbotDemoHost    = "chatbot.localhost"
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
			if err := kindrig.InstallIngress(kindrig.DefaultCommandRun); err != nil {
				return err
			}
			repository, tag := splitImageRef(images.Runtime)
			args := []string{
				"upgrade", "--install", chatbotDemoRelease, staged,
				"--values", filepath.Join(staged, "ci", "kind-llm-values.yaml"),
				"--set", "image.repository=" + repository,
				"--set", "image.tag=" + tag,
				"--set", "image.pullPolicy=Never",
				"--set", "ingress.enabled=true",
				"--set", "ingress.className=nginx",
				"--set", "ingress.host=" + chatbotDemoHost,
				"--wait", "--timeout", helmLLMInstallTimeout.String(),
			}
			if output, err := exec.Command("helm", args...).CombinedOutput(); err != nil {
				return fmt.Errorf("helm demo install: %w: %s",
					err, strings.TrimSpace(string(output)))
			}
			fmt.Printf("demo: revision %s ready at http://%s/ and http://%s/api/lifecycle/health\n",
				images.Revision, chatbotDemoHost, chatbotDemoHost)
			fmt.Println("demo: fleet observer at http://observer.localhost/")
			return nil
		})
}

// Down deletes only the chatbot-mesh demo cluster.
func (Demo) Down() error {
	if err := Doctor(); err != nil {
		return fmt.Errorf("demo teardown requested but preflight failed: %w", err)
	}
	return kindrig.DemoDown(kindrig.DefaultRun, chatbotDemoCluster)
}

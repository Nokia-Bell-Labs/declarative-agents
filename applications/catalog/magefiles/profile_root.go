// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/catalog/catalogroot"
	"gopkg.in/yaml.v3"
)

const catalogDemoConfigFile = "demo.yaml"

type catalogDemoConfig struct {
	CoreRoot  string `yaml:"core_root"`
	CoreImage string `yaml:"core_image"`
}

// catalogOwnerRoot resolves the command's startup directory once and verifies
// that catalog Mage targets are being run from their owner root.
func catalogOwnerRoot(owner string) (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("%s: resolve catalog owner root: %w", owner, err)
	}
	resolution, err := catalogroot.Resolve(owner, cwd, cwd)
	if err != nil {
		return "", fmt.Errorf("%s; run the command from applications/catalog", err)
	}
	return resolution.Path, nil
}

// resolveAgentCoreRoot returns one absolute Agent Core checkout path. An
// explicit demo.yaml core_root is interpreted against the catalog owner root;
// otherwise the monorepo's ../../agent-core path is used.
func resolveAgentCoreRoot(catalogRoot string) (string, error) {
	config, err := loadCatalogDemoConfig(catalogRoot)
	if err != nil {
		return "", err
	}
	candidate := strings.TrimSpace(config.CoreRoot)
	source := "demo.yaml core_root"
	if candidate == "" {
		candidate = filepath.Join(catalogRoot, "..", "..", "agent-core")
		source = "repository discovery"
	} else if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(catalogRoot, candidate)
	}
	candidate, err = filepath.Abs(filepath.Clean(candidate))
	if err != nil {
		return "", fmt.Errorf("resolve %s path %q: %w", source, candidate, err)
	}
	return candidate, nil
}

func resolveAgentCoreImage(catalogRoot string) (string, error) {
	config, err := loadCatalogDemoConfig(catalogRoot)
	if err != nil {
		return "", err
	}
	if image := strings.TrimSpace(config.CoreImage); image != "" {
		return image, nil
	}
	return defaultAgentCoreImage, nil
}

func loadCatalogDemoConfig(catalogRoot string) (catalogDemoConfig, error) {
	configPath := filepath.Join(catalogRoot, catalogDemoConfigFile)
	data, err := os.ReadFile(configPath)
	if os.IsNotExist(err) {
		return catalogDemoConfig{}, nil
	}
	if err != nil {
		return catalogDemoConfig{}, fmt.Errorf("read %s: %w", configPath, err)
	}
	var config catalogDemoConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return catalogDemoConfig{}, fmt.Errorf("parse %s: %w", configPath, err)
	}
	return config, nil
}

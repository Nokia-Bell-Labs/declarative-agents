// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const demoConfigFile = "demo.yaml"

// demoConfig carries the optional, declarative overrides the coding-agent
// magefiles read from demo.yaml. Every field is optional: an absent file or an
// unset field falls back to the built-in default. Overriding a value means
// editing this declaration, never an environment variable. (Catalog-root
// resolution stays with the shared catalogroot package, tracked in GH-1250.)
type demoConfig struct {
	CoreRoot       string `yaml:"core_root"`
	HelmDist       string `yaml:"helm_dist"`
	Image          string `yaml:"image"`
	ProfilesOutput string `yaml:"profiles_output"`
	OllamaURL      string `yaml:"ollama_url"`
}

// loadDemoConfig reads demo.yaml from the application root. A missing file is the
// zero-configuration path and yields an empty config, not an error.
func loadDemoConfig(applicationRoot string) (demoConfig, error) {
	var config demoConfig
	data, err := os.ReadFile(filepath.Join(applicationRoot, demoConfigFile))
	if err != nil {
		if os.IsNotExist(err) {
			return config, nil
		}
		return config, fmt.Errorf("read %s: %w", demoConfigFile, err)
	}
	if err := yaml.Unmarshal(data, &config); err != nil {
		return config, fmt.Errorf("parse %s: %w", demoConfigFile, err)
	}
	return config, nil
}

// loadDemoConfigOrEmpty returns the parsed demo.yaml, or an empty config when the
// file is absent or unreadable, so the resolver helpers fall back to the default
// rather than threading an error through every call site.
func loadDemoConfigOrEmpty(applicationRoot string) demoConfig {
	config, _ := loadDemoConfig(applicationRoot)
	return config
}

// demoResolvePath anchors a demo.yaml override at the application root when it is
// relative, returns the fallback when the override is empty, and cleans an
// absolute override as-is.
func demoResolvePath(applicationRoot, override, fallback string) string {
	if override == "" {
		return fallback
	}
	if filepath.IsAbs(override) {
		return filepath.Clean(override)
	}
	return filepath.Join(applicationRoot, override)
}

// demoImage resolves the runtime image reference: demo.yaml image when set,
// otherwise the built-in repository and tag.
func demoImage(applicationRoot string) string {
	if image := loadDemoConfigOrEmpty(applicationRoot).Image; image != "" {
		return image
	}
	return codingAgentImageRepository + ":" + codingAgentImageTag
}

// demoProfilesOutput resolves the profile-closure output directory: demo.yaml
// profiles_output when set, otherwise build/profiles under the application root.
func demoProfilesOutput(applicationRoot string) string {
	return demoResolvePath(applicationRoot,
		loadDemoConfigOrEmpty(applicationRoot).ProfilesOutput,
		filepath.Join(applicationRoot, filepath.FromSlash(defaultProfileOutput)))
}

// demoHelmDist resolves the chart package output directory: demo.yaml helm_dist
// when set, otherwise helm/dist under the application root.
func demoHelmDist(applicationRoot string) string {
	return demoResolvePath(applicationRoot,
		loadDemoConfigOrEmpty(applicationRoot).HelmDist,
		filepath.Join(applicationRoot, "helm", "dist"))
}

// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	agentCoreRootEnv       = "AGENT_CORE_ROOT"
	agentCoreImageEnv      = "AGENT_CORE_IMAGE"
	containerEngineEnv     = "AGENT_PROFILES_CONTAINER_ENGINE"
	defaultAgentCoreImage  = "agent-core:latest"
	containerProfilesMount = "/profiles"
	containerWorkMount     = "/work"
	containerCoreMount     = "/opt/agent-core"
)

type profileConfig struct {
	Machine          string   `yaml:"machine"`
	Tools            []string `yaml:"tools"`
	ToolDeclarations []string `yaml:"tool_declarations"`
	ToolConfigDirs   []string `yaml:"tool_config_dirs"`
	RestDefinitions  []string `yaml:"rest_definitions"`
	RestConfigDirs   []string `yaml:"rest_config_dirs"`
}

type lookPathFunc func(string) (string, error)
type commandRunner func(name string, args ...string) error

// Validate checks profile paths against an external agent-core checkout.
func Validate() error {
	root, err := os.Getwd()
	if err != nil {
		return err
	}
	coreRoot := envOrDefault(agentCoreRootEnv, filepath.Join(filepath.Dir(root), "agent-core"))
	return validateProfiles(root, coreRoot)
}

// ContainerSmoke runs one profile from /profiles with an agent-core image.
func ContainerSmoke() error {
	root, err := os.Getwd()
	if err != nil {
		return err
	}
	coreRoot := envOrDefault(agentCoreRootEnv, filepath.Join(filepath.Dir(root), "agent-core"))
	engine, err := profileContainerEngine(os.Getenv(containerEngineEnv), exec.LookPath)
	if err != nil {
		return err
	}
	return runContainerSmoke(engine, defaultRun, root, coreRoot, envOrDefault(agentCoreImageEnv, defaultAgentCoreImage))
}

func validateProfiles(root, coreRoot string) error {
	profiles, err := discoverProfiles(filepath.Join(root, "agents"))
	if err != nil {
		return err
	}
	if len(profiles) == 0 {
		return fmt.Errorf("no profile-shaped YAML files found under agents")
	}
	for _, profile := range profiles {
		if err := validateProfile(profile, coreRoot); err != nil {
			return err
		}
	}
	fmt.Printf("validated %d profiles against %s\n", len(profiles), coreRoot)
	return nil
}

func discoverProfiles(root string) ([]string, error) {
	var profiles []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() && isProfileFile(entry.Name()) {
			profiles = append(profiles, path)
		}
		return nil
	})
	return profiles, err
}

func isProfileFile(name string) bool {
	if name == "profile.yaml" {
		return true
	}
	if strings.HasPrefix(name, "profile-") && strings.HasSuffix(name, ".yaml") {
		return true
	}
	return strings.HasSuffix(name, "-profile.yaml")
}

func validateProfile(path, coreRoot string) error {
	profile, err := readProfile(path)
	if err != nil {
		return err
	}
	base := filepath.Dir(path)
	for _, ref := range profileRefs(profile) {
		if err := validateProfileRef(base, coreRoot, ref); err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
	}
	return nil
}

func readProfile(path string) (profileConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return profileConfig{}, fmt.Errorf("read profile %s: %w", path, err)
	}
	var profile profileConfig
	if err := yaml.Unmarshal(data, &profile); err != nil {
		return profileConfig{}, fmt.Errorf("parse profile %s: %w", path, err)
	}
	if profile.Machine == "" {
		return profileConfig{}, fmt.Errorf("profile %s: machine is required", path)
	}
	if len(profile.Tools) == 0 {
		return profileConfig{}, fmt.Errorf("profile %s: tools is required", path)
	}
	return profile, nil
}

func profileRefs(profile profileConfig) []string {
	refs := []string{profile.Machine}
	refs = append(refs, profile.Tools...)
	refs = append(refs, profile.ToolDeclarations...)
	refs = append(refs, profile.ToolConfigDirs...)
	refs = append(refs, profile.RestDefinitions...)
	refs = append(refs, profile.RestConfigDirs...)
	return refs
}

func validateProfileRef(base, coreRoot, ref string) error {
	path, err := resolveProfileRef(base, coreRoot, ref)
	if err != nil {
		return err
	}
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("missing referenced path %s: %w", ref, err)
	}
	return nil
}

func resolveProfileRef(base, coreRoot, ref string) (string, error) {
	clean := filepath.Clean(ref)
	if strings.HasPrefix(filepath.ToSlash(clean), containerCoreMount+"/agents/") {
		return "", fmt.Errorf("profile reference must not require copied core agent assets: %s", ref)
	}
	if strings.HasPrefix(filepath.ToSlash(clean), containerCoreMount+"/") {
		rel := strings.TrimPrefix(filepath.ToSlash(clean), containerCoreMount+"/")
		return filepath.Join(coreRoot, filepath.FromSlash(rel)), nil
	}
	if filepath.IsAbs(clean) {
		return clean, nil
	}
	return filepath.Join(base, clean), nil
}

func profileContainerEngine(override string, lookPath lookPathFunc) (string, error) {
	if engine := strings.TrimSpace(override); engine != "" {
		return engine, nil
	}
	if _, err := lookPath("docker"); err == nil {
		return "docker", nil
	}
	if _, err := lookPath("podman"); err == nil {
		return "podman", nil
	}
	return "", fmt.Errorf("no container engine found; set %s to docker or podman", containerEngineEnv)
}

func runContainerSmoke(engine string, run commandRunner, root, coreRoot, image string) error {
	if err := run(engine, "run", "--rm", "--entrypoint", "sh", image, "-c", "test ! -e /opt/agent-core/agents"); err != nil {
		return fmt.Errorf("check image excludes bundled agent assets: %w", err)
	}
	args := []string{
		"run", "--rm",
		"-v", root + ":" + containerProfilesMount + ":ro",
		"-v", filepath.Join(coreRoot, "tools") + ":" + filepath.Join(containerCoreMount, "tools") + ":ro",
		"-v", root + ":" + containerWorkMount + ":ro",
		"-w", containerWorkMount,
		image,
		"--profile", containerProfilesMount + "/agents/jurist/profile.yaml",
		"--directory", containerWorkMount,
	}
	if err := run(engine, args...); err != nil {
		return fmt.Errorf("run mounted jurist profile: %w", err)
	}
	return nil
}

func defaultRun(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func envOrDefault(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

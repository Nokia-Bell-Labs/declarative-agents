// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	agentCoreRootEnv       = "AGENT_CORE_ROOT"
	agentCoreImageEnv      = "AGENT_CORE_IMAGE"
	dockerEngine           = "docker"
	defaultAgentCoreImage  = "agent-core:latest"
	containerProfilesMount = "/profiles"
	containerWorkMount     = "/work"
	containerCoreMount     = "/opt/agent-core"
	juristProfileDir       = "agents/jurist"
	juristCharterDemoDir   = "testdata/integration/jurist-charter-demo"
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

// Validate checks bundle-owned path policy, then delegates all runtime
// configuration semantics to agent-core's canonical --validate-config path.
func Validate() error {
	root, err := catalogOwnerRoot("catalog validate")
	if err != nil {
		return err
	}
	coreRoot, err := resolveAgentCoreRoot(root)
	if err != nil {
		return err
	}
	if err := validatePortableProfileRefs(root, coreRoot); err != nil {
		return err
	}
	if err := bootSmoke(root, coreRoot); err != nil {
		return err
	}
	return validateJuristCharterDemo(root, coreRoot)
}

// ContainerSmoke runs one profile from /profiles with an agent-core image.
func ContainerSmoke() error {
	root, err := catalogOwnerRoot("catalog containerSmoke")
	if err != nil {
		return err
	}
	coreRoot, err := resolveAgentCoreRoot(root)
	if err != nil {
		return err
	}
	if err := requireDocker(exec.LookPath); err != nil {
		return err
	}
	return runContainerSmoke(defaultRun, root, coreRoot, envOrDefault(agentCoreImageEnv, defaultAgentCoreImage))
}

func validatePortableProfileRefs(root, coreRoot string) error {
	profiles, err := discoverAuditProfiles(root)
	if err != nil {
		return err
	}
	for _, profile := range profiles {
		if err := validatePortableProfileRef(profile, root, coreRoot); err != nil {
			return err
		}
	}
	fmt.Printf("validated portable references for %d profiles against %s\n", len(profiles), coreRoot)
	return nil
}

func validateJuristCharterDemo(profilesRoot, coreRoot string) error {
	tmpDir, err := os.MkdirTemp("", "catalog-jurist-charter-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()
	profilePath, err := writeJuristCharterDemoProfileFiles(profilesRoot, coreRoot, tmpDir)
	if err != nil {
		return err
	}
	binary, err := buildIntegrationAgent(coreRoot)
	if err != nil {
		return err
	}
	fixtureDir := filepath.Join(profilesRoot, juristCharterDemoDir)
	cmd := exec.Command(binary, "--profile", profilePath, "--directory", fixtureDir, "--core-root", coreRoot)
	cmd.Dir = profilesRoot
	output, runErr := commandWithOutput(cmd)
	if err := assertJuristCharterDemoFindings(output.String()); err != nil {
		if runErr != nil {
			return fmt.Errorf("%w: %v\n%s", err, runErr, output.String())
		}
		return fmt.Errorf("%w\n%s", err, output.String())
	}
	fmt.Println("validated jurist charter demo findings")
	return nil
}

func writeJuristCharterDemoProfileFiles(profilesRoot, coreRoot, tmpDir string) (string, error) {
	profilePath := filepath.Join(tmpDir, "profile.yaml")
	toolDeclPath := filepath.Join(tmpDir, "load-corpus-demo.yaml")
	suitePath := filepath.Join(profilesRoot, juristProfileDir, "suites", "demo-charter.yaml")
	profile := fmt.Sprintf(`name: jurist-demo
machine: %q
tools:
  - %q
tool_config_dirs:
  - %q
tool_declarations:
  - %q
  - %q
`, filepath.Join(profilesRoot, juristProfileDir, "machine.yaml"),
		filepath.Join(profilesRoot, juristProfileDir, "tools.yaml"),
		filepath.Join(coreRoot, "tools", "builtin", "spec-validation"),
		toolDeclPath,
		filepath.Join(profilesRoot, juristProfileDir, "ripgrep.yaml"))
	if err := os.WriteFile(profilePath, []byte(profile), 0o644); err != nil {
		return "", fmt.Errorf("write jurist demo profile: %w", err)
	}
	toolDecl := fmt.Sprintf(`includes:
  - %q
tools:
  - name: load_corpus
    type: builtin
    init: load_corpus
    visibility: internal
    config:
      suite_paths:
        - %q
    emits:
      - ToolDone
      - CommandError
`, filepath.Join(coreRoot, "tools", "builtin", "load-corpus.yaml"), suitePath)
	if err := os.WriteFile(toolDeclPath, []byte(toolDecl), 0o644); err != nil {
		return "", fmt.Errorf("write jurist demo tool declaration: %w", err)
	}
	return profilePath, nil
}

func assertJuristCharterDemoFindings(output string) error {
	required := []string{
		"jurist-demo-charter/no-internal-vocabulary (grep_check)",
		"jurist-demo-charter/citations-resolve (ref_check)",
		"jurist-demo-charter/artifacts-exist (consistency_check)",
		"terminal state: failed",
	}
	for _, want := range required {
		if !strings.Contains(output, want) {
			return fmt.Errorf("jurist charter demo output missing %q", want)
		}
	}
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

func validatePortableProfileRef(profilePath, profilesRoot, coreRoot string) error {
	profile, err := readProfileRefs(profilePath)
	if err != nil {
		return err
	}
	base := filepath.Dir(profilePath)
	for _, ref := range profileRefs(profile) {
		if err := validateProfileRef(profilesRoot, base, coreRoot, ref); err != nil {
			return fmt.Errorf("%s: %w", profilePath, err)
		}
	}
	return nil
}

func readProfileRefs(path string) (profileConfig, error) {
	var profile profileConfig
	if err := readYAML(path, &profile); err != nil {
		return profileConfig{}, err
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

func validateProfileRef(profilesRoot, base, coreRoot, ref string) error {
	resolved, boundary, err := resolveProfileRef(profilesRoot, base, coreRoot, ref)
	if err != nil {
		return err
	}
	if err := requireLexicalPathWithin(boundary, resolved); err != nil {
		return fmt.Errorf("non-portable reference %s: %w", ref, err)
	}
	info, err := os.Lstat(resolved)
	if err != nil {
		return fmt.Errorf("missing referenced path %s: %w", ref, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("non-portable reference %s: symlinks are not portable", ref)
	}
	if err := requirePathWithin(boundary, resolved); err != nil {
		return fmt.Errorf("non-portable reference %s: %w", ref, err)
	}
	return nil
}

func readYAML(path string, out any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	if err := yaml.Unmarshal(data, out); err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}
	return nil
}

func resolveProfileRef(profilesRoot, base, coreRoot, ref string) (string, string, error) {
	trimmed := strings.TrimSpace(ref)
	if trimmed == "" {
		return "", "", fmt.Errorf("profile reference must not be empty")
	}
	portable := strings.ReplaceAll(trimmed, `\`, "/")
	clean := path.Clean(portable)
	if isWindowsDriveRef(clean) || strings.HasPrefix(clean, "//") {
		return "", "", fmt.Errorf("profile reference must not use an absolute host path: %s", ref)
	}
	if clean == containerCoreMount+"/agents" || strings.HasPrefix(clean, containerCoreMount+"/agents/") {
		return "", "", fmt.Errorf("profile reference must not require copied core agent assets: %s", ref)
	}
	coreTools := containerCoreMount + "/tools"
	if clean == coreTools || strings.HasPrefix(clean, coreTools+"/") {
		rel := strings.TrimPrefix(clean, containerCoreMount+"/")
		return filepath.Join(coreRoot, filepath.FromSlash(rel)), coreRoot, nil
	}
	if strings.HasPrefix(clean, containerCoreMount+"/") || path.IsAbs(clean) {
		return "", "", fmt.Errorf("profile reference must not use an absolute host path: %s", ref)
	}
	if clean == "agents" || strings.HasPrefix(clean, "agents/") {
		return filepath.Join(profilesRoot, filepath.FromSlash(clean)), profilesRoot, nil
	}
	return filepath.Join(base, filepath.FromSlash(clean)), profilesRoot, nil
}

func isWindowsDriveRef(ref string) bool {
	return len(ref) >= 2 &&
		((ref[0] >= 'A' && ref[0] <= 'Z') || (ref[0] >= 'a' && ref[0] <= 'z')) &&
		ref[1] == ':'
}

func requireLexicalPathWithin(root, candidate string) error {
	rel, err := filepath.Rel(filepath.Clean(root), filepath.Clean(candidate))
	if err != nil {
		return fmt.Errorf("compare referenced path with allowed root: %w", err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return fmt.Errorf("resolved path escapes allowed root %s", root)
	}
	return nil
}

func requirePathWithin(root, candidate string) error {
	rootReal, err := filepath.EvalSymlinks(root)
	if err != nil {
		return fmt.Errorf("resolve allowed root %s: %w", root, err)
	}
	candidateReal, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		return fmt.Errorf("resolve referenced path %s: %w", candidate, err)
	}
	rel, err := filepath.Rel(rootReal, candidateReal)
	if err != nil {
		return fmt.Errorf("compare referenced path with allowed root: %w", err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return fmt.Errorf("resolved path escapes allowed root %s", root)
	}
	return nil
}

func requireDocker(lookPath lookPathFunc) error {
	if _, err := lookPath(dockerEngine); err != nil {
		return fmt.Errorf("docker not found on PATH; install Docker to run the container smoke test")
	}
	return nil
}

func runContainerSmoke(run commandRunner, root, coreRoot, image string) error {
	if err := run(dockerEngine, "run", "--rm", "--entrypoint", "sh", image, "-c", "test ! -e /opt/agent-core/agents && command -v rg >/dev/null"); err != nil {
		return fmt.Errorf("check image layout and shared find/jurist rg dependency: %w", err)
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
	if err := run(dockerEngine, args...); err != nil {
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

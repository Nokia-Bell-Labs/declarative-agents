// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	knowledgeManagerProfile = "agents/knowledge-manager/documentation-curator/profile.yaml"
	agentBinaryEnv          = "AGENT_BINARY"
	agentCoreHomeEnv        = "AGENT_CORE_HOME"
	agentCoreRootEnv        = "AGENT_CORE_ROOT"
	agentProfilesRootEnv    = "AGENT_PROFILES_ROOT"
)

var buildAgentBinaryFunc = buildAgentBinary

// START OMIT
func main() {
	StartAgent(knowledgeManagerProfile)
}

// END OMIT

func StartAgent(profile string) {
	profilesRoot := resolveProfilesRoot()
	coreRoot := resolveAgentCoreRoot()
	profilePath := filepath.Join(profilesRoot, profile)
	if coreRoot != "" {
		profilePath = prepareDemoProfile(profilePath, profilesRoot, coreRoot)
	}
	workspace := envOrDefault("AGENT_WORKSPACE", envOrDefaultRoot(coreRoot, profilesRoot))
	cmd := agentCommand(profilePath, workspace, profilesRoot, coreRoot)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		panic(err)
	}
}

func agentCommand(profile, workspace, profilesRoot, coreRoot string) *exec.Cmd {
	if agentBinary := strings.TrimSpace(os.Getenv(agentBinaryEnv)); agentBinary != "" {
		cmd := exec.Command(agentBinary, "--profile", profile, "--directory", workspace)
		cmd.Dir = profilesRoot
		return withAgentCoreHome(cmd, coreRoot)
	}
	if coreRoot != "" {
		cmd := exec.Command(buildAgentBinaryFunc(coreRoot), "--profile", profile, "--directory", workspace)
		cmd.Dir = profilesRoot
		return withAgentCoreHome(cmd, coreRoot)
	}
	cmd := exec.Command("agent", "--profile", profile, "--directory", workspace)
	cmd.Dir = profilesRoot
	return cmd
}

func prepareDemoProfile(profilePath, profilesRoot, coreRoot string) string {
	profileDir := filepath.Dir(profilePath)
	tmpDir, err := os.MkdirTemp("", "agent-profiles-demo-*")
	if err != nil {
		panic(err)
	}
	tmpProfile := filepath.Join(tmpDir, "profile.yaml")
	writeFile(tmpProfile, fmt.Sprintf(`name: documentation-curator
machine: %q
tools:
  - %q
tool_declarations:
  - %q
  - %q
  - %q
  - %q
rest_definitions:
  - %q
`, filepath.Join(profileDir, "machine.yaml"),
		filepath.Join(profileDir, "tools.yaml"),
		filepath.Join(tmpDir, "builtin.yaml"),
		filepath.Join(profileDir, "declarations.yaml"),
		filepath.Join(profileDir, "request-declarations.yaml"),
		filepath.Join(coreRoot, "tools", "builtin", "lifecycle", "exit-agent.yaml"),
		filepath.Join(tmpDir, "rest.yaml")))
	writeFile(filepath.Join(tmpDir, "builtin.yaml"), demoBuiltinConfig(profileDir, coreRoot, tmpProfile))
	copyFile(filepath.Join(profileDir, "rest.yaml"), filepath.Join(tmpDir, "rest.yaml"))
	copyFile(filepath.Join(profileDir, "openapi.yaml"), filepath.Join(tmpDir, "openapi.yaml"))
	copyFile(filepath.Join(profileDir, "request-machine.yaml"), filepath.Join(tmpDir, "request-machine.yaml"))
	copyFile(filepath.Join(profileDir, "ui", "ux.yaml"), filepath.Join(tmpDir, "ui", "ux.yaml"))
	return tmpProfile
}

func demoBuiltinConfig(profileDir, coreRoot, profilePath string) string {
	content := readFile(filepath.Join(profileDir, "builtin.yaml"))
	replacements := map[string]string{
		"docs_dir: docs":       "docs_dir: " + fmt.Sprintf("%q", filepath.Join(coreRoot, "docs")),
		"configs_dir: configs": "configs_dir: " + fmt.Sprintf("%q", filepath.Join(coreRoot, "configs")),
		"source_dir: .":        "source_dir: " + fmt.Sprintf("%q", coreRoot),
		"profile_path: agents/knowledge-manager/documentation-curator/profile.yaml": "profile_path: " + fmt.Sprintf("%q", profilePath),
		"timeout: 30s": "timeout: 24h",
	}
	for old, newValue := range replacements {
		content = strings.ReplaceAll(content, old, newValue)
	}
	return content
}

func readFile(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		panic(err)
	}
	return string(data)
}

func copyFile(src, dst string) {
	writeFile(dst, readFile(src))
}

func writeFile(path, content string) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		panic(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		panic(err)
	}
}

func buildAgentBinary(coreRoot string) string {
	binary := filepath.Join(os.TempDir(), "agent-profiles-demo-agent")
	cmd := exec.Command("go", "build", "-tags", "production", "-o", binary, "./cmd/agent")
	cmd.Dir = coreRoot
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		panic(err)
	}
	return binary
}

func withAgentCoreHome(cmd *exec.Cmd, coreRoot string) *exec.Cmd {
	if coreRoot != "" && strings.TrimSpace(os.Getenv(agentCoreHomeEnv)) == "" {
		cmd.Env = withEnv(os.Environ(), agentCoreHomeEnv, coreRoot)
	}
	return cmd
}

func withEnv(env []string, name, value string) []string {
	prefix := name + "="
	for i, entry := range env {
		if strings.HasPrefix(entry, prefix) {
			env[i] = prefix + value
			return env
		}
	}
	return append(env, prefix+value)
}

func resolveAgentCoreRoot() string {
	if coreRoot := strings.TrimSpace(os.Getenv(agentCoreRootEnv)); coreRoot != "" {
		return coreRoot
	}
	profilesRoot := resolveProfilesRoot()
	candidate := filepath.Join(filepath.Dir(profilesRoot), "agent-core")
	if pathExists(filepath.Join(candidate, "cmd", "agent", "main.go")) {
		return candidate
	}
	return ""
}

func resolveProfilesRoot() string {
	if profilesRoot := strings.TrimSpace(os.Getenv(agentProfilesRootEnv)); profilesRoot != "" {
		return profilesRoot
	}
	if wd, err := os.Getwd(); err == nil {
		if root := findProfilesRoot(wd); root != "" {
			return root
		}
	}
	return "/profiles"
}

func findProfilesRoot(start string) string {
	dir := filepath.Clean(start)
	for {
		if pathExists(filepath.Join(dir, knowledgeManagerProfile)) {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

func envOrDefault(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

func envOrDefaultRoot(preferred, fallback string) string {
	if strings.TrimSpace(preferred) != "" {
		return preferred
	}
	return fallback
}

func pathExists(path string) bool {
	if _, err := os.Stat(path); err == nil {
		return true
	} else if !os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "warning: cannot inspect %s: %v\n", path, err)
	}
	return false
}

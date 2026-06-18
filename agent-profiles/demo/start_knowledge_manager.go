// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"flag"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const knowledgeManagerProfile = "agents/knowledge-manager/documentation-curator/profile.yaml"

var buildAgentBinaryFunc = buildAgentBinary

// demoConfig holds resolved paths for the knowledge-manager demo launcher.
type demoConfig struct {
	ProfilesRoot string
	CoreRoot     string
	Workspace    string
	AgentBinary  string
}

// START OMIT
func main() {
	flagProfiles := flag.String("profiles-root", "", "path to agent-profiles repository root (default: walk upward from the working directory)")
	flagCore := flag.String("core-root", "", "path to agent-core checkout for --core-root and builtin overlay (default: sibling ../agent-core of profiles root)")
	flagWorkspace := flag.String("workspace", "", "workspace directory for cmd/agent --directory (default: core-root if set, otherwise profiles root)")
	flagAgent := flag.String("agent", "", "path to agent binary (default: build from core-root when set, otherwise agent on PATH)")
	flag.Parse()

	cfg, err := ResolveDemoConfig(*flagProfiles, *flagCore, *flagWorkspace, *flagAgent)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	StartAgent(knowledgeManagerProfile, cfg)
}

// END OMIT

// ResolveDemoConfig resolves launcher paths from explicit flags or repository layout.
func ResolveDemoConfig(profilesFlag, coreFlag, workspaceFlag, agentFlag string) (demoConfig, error) {
	profilesRoot, err := profilesRootFromFlagOrWalk(profilesFlag)
	if err != nil {
		return demoConfig{}, err
	}
	coreRoot := coreRootFromFlagOrSibling(profilesRoot, coreFlag)
	workspace := strings.TrimSpace(workspaceFlag)
	if workspace == "" {
		workspace = workspaceDefault(coreRoot, profilesRoot)
	}
	return demoConfig{
		ProfilesRoot: profilesRoot,
		CoreRoot:     coreRoot,
		Workspace:    workspace,
		AgentBinary:  strings.TrimSpace(agentFlag),
	}, nil
}

func profilesRootFromFlagOrWalk(profilesFlag string) (string, error) {
	if root := strings.TrimSpace(profilesFlag); root != "" {
		return filepath.Clean(root), nil
	}
	wd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("get working directory: %w", err)
	}
	if root := findProfilesRoot(wd); root != "" {
		return root, nil
	}
	return "", fmt.Errorf(
		"agent-profiles root not found; run from the repository checkout or pass -profiles-root with the path that contains %s",
		knowledgeManagerProfile,
	)
}

func coreRootFromFlagOrSibling(profilesRoot, coreFlag string) string {
	if root := strings.TrimSpace(coreFlag); root != "" {
		return filepath.Clean(root)
	}
	candidate := filepath.Join(filepath.Dir(profilesRoot), "agent-core")
	if pathExists(filepath.Join(candidate, "cmd", "agent", "main.go")) {
		return candidate
	}
	return ""
}

func workspaceDefault(coreRoot, profilesRoot string) string {
	if strings.TrimSpace(coreRoot) != "" {
		return coreRoot
	}
	return profilesRoot
}

// StartAgent runs cmd/agent for the given profile path segment under cfg.ProfilesRoot.
func StartAgent(profile string, cfg demoConfig) {
	profilePath := filepath.Join(cfg.ProfilesRoot, profile)
	if cfg.CoreRoot != "" {
		profilePath = prepareDemoProfile(profilePath, cfg.ProfilesRoot, cfg.CoreRoot)
	}
	cmd := agentCommand(profilePath, cfg.Workspace, cfg)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		panic(err)
	}
}

func agentCommand(profile, workspace string, cfg demoConfig) *exec.Cmd {
	if bin := cfg.AgentBinary; bin != "" {
		return execAgent(bin, profile, workspace, cfg.CoreRoot, cfg.ProfilesRoot)
	}
	if cfg.CoreRoot != "" {
		bin := buildAgentBinaryFunc(cfg.CoreRoot)
		return execAgent(bin, profile, workspace, cfg.CoreRoot, cfg.ProfilesRoot)
	}
	return execAgent("agent", profile, workspace, "", cfg.ProfilesRoot)
}

func execAgent(bin, profile, workspace, coreRoot, profilesRoot string) *exec.Cmd {
	args := []string{"--profile", profile, "--directory", workspace}
	if coreRoot != "" {
		args = append(args, "--core-root", coreRoot)
	}
	cmd := exec.Command(bin, args...)
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
	copyMonitorAssetsIntoDemoProfile(profileDir, tmpDir)
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

const monitorDistYAMLPath = "agents/knowledge-manager/documentation-curator/ui/monitor/dist"

// copyMonitorAssetsIntoDemoProfile copies bundled monitor UX into the temp profile
// and rewrites static_assets roots in rest.yaml to absolute paths under tmpDir.
func copyMonitorAssetsIntoDemoProfile(profileDir, tmpDir string) {
	srcDist := filepath.Join(profileDir, "ui", "monitor", "dist")
	if !pathExists(srcDist) {
		return
	}
	dstDist := filepath.Join(tmpDir, "ui", "monitor", "dist")
	if err := copyDir(srcDist, dstDist); err != nil {
		panic(err)
	}
	rewriteDemoMonitorRestRoots(tmpDir)
}

func rewriteDemoMonitorRestRoots(tmpDir string) {
	restPath := filepath.Join(tmpDir, "rest.yaml")
	content := readFile(restPath)
	distAbs := filepath.ToSlash(filepath.Join(tmpDir, "ui", "monitor", "dist"))
	if strings.Contains(content, monitorDistYAMLPath) {
		content = strings.ReplaceAll(content, monitorDistYAMLPath, distAbs)
	}
	writeFile(restPath, content)
}

func copyDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return os.MkdirAll(dst, 0o755)
		}
		out := filepath.Join(dst, rel)
		if entry.IsDir() {
			return os.MkdirAll(out, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		writeFile(out, string(data))
		return nil
	})
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

func pathExists(path string) bool {
	if _, err := os.Stat(path); err == nil {
		return true
	} else if !os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "warning: cannot inspect %s: %v\n", path, err)
	}
	return false
}

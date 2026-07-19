// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
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

type machineConfig struct {
	Name           string             `yaml:"name"`
	Purpose        string             `yaml:"purpose"`
	Configuration  map[string]any     `yaml:"configuration"`
	Invariants     []string           `yaml:"invariants"`
	Lifecycle      string             `yaml:"lifecycle"`
	States         []namedSpec        `yaml:"states"`
	Signals        []namedSpec        `yaml:"signals"`
	TerminalStates []string           `yaml:"terminal_states"`
	Transitions    []transitionConfig `yaml:"transitions"`
}

type namedSpec struct {
	Name    string
	Meaning string `yaml:"meaning"`
	Trigger string `yaml:"trigger"`
}

func (n *namedSpec) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.ScalarNode:
		n.Name = value.Value
		return nil
	case yaml.MappingNode:
		type alias namedSpec
		return value.Decode((*alias)(n))
	default:
		return fmt.Errorf("expected scalar or mapping")
	}
}

type transitionConfig struct {
	State  string `yaml:"state"`
	Signal string `yaml:"signal"`
	Next   string `yaml:"next"`
	Action string `yaml:"action"`
}

type toolSelectionFile struct {
	Tools []string `yaml:"tools"`
}

type toolDefsFile struct {
	Includes []string  `yaml:"includes"`
	Tools    []toolDef `yaml:"tools"`
}

type toolDef struct {
	Name       string   `yaml:"name"`
	Visibility string   `yaml:"visibility"`
	Emits      []string `yaml:"emits"`
}

type lookPathFunc func(string) (string, error)
type commandRunner func(name string, args ...string) error

// Validate checks profile paths and profile wiring against an external agent-core checkout.
func Validate() error {
	root, err := os.Getwd()
	if err != nil {
		return err
	}
	coreRoot := envOrDefault(agentCoreRootEnv, filepath.Join(filepath.Dir(root), "agent-core"))
	if err := validateProfiles(root, coreRoot); err != nil {
		return err
	}
	return validateJuristCharterDemo(root, coreRoot)
}

// ContainerSmoke runs one profile from /profiles with an agent-core image.
func ContainerSmoke() error {
	root, err := os.Getwd()
	if err != nil {
		return err
	}
	coreRoot := envOrDefault(agentCoreRootEnv, filepath.Join(filepath.Dir(root), "agent-core"))
	if err := requireDocker(exec.LookPath); err != nil {
		return err
	}
	return runContainerSmoke(defaultRun, root, coreRoot, envOrDefault(agentCoreImageEnv, defaultAgentCoreImage))
}

func validateProfiles(root, coreRoot string) error {
	profiles, err := discoverProfiles(filepath.Join(root, "agents"))
	if err != nil {
		return err
	}
	if len(profiles) == 0 {
		return fmt.Errorf("no profile-shaped YAML files found under agents")
	}
	// The conformance grammar fixtures (rest, control, lifecycle) live under
	// testdata/conformance rather than agents/ (they are not roles, GH-328), but
	// they are still profiles and must keep schema and path validation. The
	// directory is optional so a synthetic root with only agents/ still validates.
	conformanceDir := filepath.Join(root, "testdata", "conformance")
	if _, statErr := os.Stat(conformanceDir); statErr == nil {
		fixtures, err := discoverProfiles(conformanceDir)
		if err != nil {
			return err
		}
		profiles = append(profiles, fixtures...)
	}
	for _, profile := range profiles {
		if err := validateProfile(profile, coreRoot); err != nil {
			return err
		}
	}
	fmt.Printf("validated %d profiles (agents + testdata/conformance) against %s\n", len(profiles), coreRoot)
	return nil
}

func validateJuristCharterDemo(profilesRoot, coreRoot string) error {
	tmpDir, err := os.MkdirTemp("", "agent-profiles-jurist-charter-*")
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
`, filepath.Join(profilesRoot, juristProfileDir, "machine.yaml"),
		filepath.Join(profilesRoot, juristProfileDir, "tools.yaml"),
		filepath.Join(coreRoot, "tools", "builtin", "spec-validation"),
		toolDeclPath)
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
	if err := validateProfileWiring(base, coreRoot, profile); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}

func readProfile(path string) (profileConfig, error) {
	var profile profileConfig
	if err := readYAML(path, &profile); err != nil {
		return profileConfig{}, err
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

func validateProfileWiring(base, coreRoot string, profile profileConfig) error {
	machinePath, err := resolveProfileRef(base, coreRoot, profile.Machine)
	if err != nil {
		return err
	}
	machine, err := loadMachine(machinePath)
	if err != nil {
		return err
	}
	if err := validateMachineMetadata(machinePath, machine); err != nil {
		return err
	}
	selected, err := loadToolSelections(base, coreRoot, profile.Tools)
	if err != nil {
		return err
	}
	declared, err := loadProfileToolDefs(base, coreRoot, profile)
	if err != nil {
		return err
	}
	selectedDefs := map[string]toolDef{}
	for _, name := range selected {
		def, ok := declared[name]
		if !ok {
			return fmt.Errorf("selected tool %q is not declared", name)
		}
		selectedDefs[name] = def
	}
	if err := validateMachineActions(machine, selectedDefs); err != nil {
		return err
	}
	return validateToolEmits(machine, selectedDefs)
}

func validateMachineMetadata(path string, machine machineConfig) error {
	if machine.Name == "" {
		return fmt.Errorf("machine %s: name is required", path)
	}
	for _, state := range machine.States {
		if state.Name == "" {
			return fmt.Errorf("machine %s: state missing name", path)
		}
	}
	for _, signal := range machine.Signals {
		if signal.Name == "" {
			return fmt.Errorf("machine %s: signal missing name", path)
		}
	}
	return nil
}

func loadMachine(path string) (machineConfig, error) {
	var machine machineConfig
	if err := readYAML(path, &machine); err != nil {
		return machineConfig{}, err
	}
	return machine, nil
}

func loadToolSelections(base, coreRoot string, refs []string) ([]string, error) {
	seen := map[string]bool{}
	var selected []string
	for _, ref := range refs {
		path, err := resolveProfileRef(base, coreRoot, ref)
		if err != nil {
			return nil, err
		}
		var file toolSelectionFile
		if err := readYAML(path, &file); err != nil {
			return nil, err
		}
		for _, name := range file.Tools {
			if !seen[name] {
				seen[name] = true
				selected = append(selected, name)
			}
		}
	}
	return selected, nil
}

func loadProfileToolDefs(base, coreRoot string, profile profileConfig) (map[string]toolDef, error) {
	declared := map[string]toolDef{}
	for _, ref := range profile.ToolConfigDirs {
		dir, err := resolveProfileRef(base, coreRoot, ref)
		if err != nil {
			return nil, err
		}
		if err := loadToolDefDir(declared, dir); err != nil {
			return nil, err
		}
	}
	for _, ref := range profile.ToolDeclarations {
		path, err := resolveProfileRef(base, coreRoot, ref)
		if err != nil {
			return nil, err
		}
		if err := loadToolDefFile(declared, path, map[string]bool{}); err != nil {
			return nil, err
		}
	}
	return declared, nil
}

func loadToolDefDir(declared map[string]toolDef, dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read tool config dir %s: %w", dir, err)
	}
	var paths []string
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".yaml" {
			continue
		}
		paths = append(paths, filepath.Join(dir, entry.Name()))
	}
	sort.Strings(paths)
	for _, path := range paths {
		if err := loadToolDefFile(declared, path, map[string]bool{}); err != nil {
			return err
		}
	}
	return nil
}

func loadToolDefFile(declared map[string]toolDef, path string, seen map[string]bool) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	if seen[abs] {
		return fmt.Errorf("circular tool include %s", abs)
	}
	seen[abs] = true
	var file toolDefsFile
	if err := readYAML(abs, &file); err != nil {
		return err
	}
	for _, include := range file.Includes {
		includePath := include
		if !filepath.IsAbs(includePath) {
			includePath = filepath.Join(filepath.Dir(abs), include)
		}
		if err := loadToolDefFile(declared, includePath, seen); err != nil {
			return err
		}
	}
	for _, def := range file.Tools {
		if def.Name == "" {
			return fmt.Errorf("tool definition in %s is missing name", abs)
		}
		declared[def.Name] = def
	}
	return nil
}

func validateMachineActions(machine machineConfig, selected map[string]toolDef) error {
	for _, transition := range machine.Transitions {
		if transition.Action == "" || transition.Action == "$tool" {
			continue
		}
		if _, ok := selected[transition.Action]; !ok {
			return fmt.Errorf("machine %s action %q is not selected", machine.Name, transition.Action)
		}
	}
	return nil
}

func validateToolEmits(machine machineConfig, selected map[string]toolDef) error {
	signals := map[string]bool{}
	for _, signal := range machine.Signals {
		signals[signal.Name] = true
	}
	terminals := map[string]bool{}
	for _, state := range machine.TerminalStates {
		terminals[state] = true
	}
	transitions := map[string]bool{}
	actionTargets := map[string][]string{}
	for _, transition := range machine.Transitions {
		transitions[transition.State+"/"+transition.Signal] = true
		if transition.Action == "" {
			continue
		}
		actionTargets[transition.Action] = append(actionTargets[transition.Action], transition.Next)
	}
	for action, targets := range actionTargets {
		if action == "$tool" {
			continue
		}
		if err := validateToolDefEmits(machine.Name, action, selected[action], targets, signals, terminals, transitions); err != nil {
			return err
		}
	}
	if targets := actionTargets["$tool"]; len(targets) > 0 {
		for name, def := range selected {
			if def.Visibility == "internal" {
				continue
			}
			if err := validateToolDefEmits(machine.Name, name, def, targets, signals, terminals, transitions); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateToolDefEmits(machineName, name string, def toolDef, targets []string, signals map[string]bool, terminals map[string]bool, transitions map[string]bool) error {
	for _, emit := range def.Emits {
		if !signals[emit] {
			return fmt.Errorf("machine %s tool %q emits undeclared signal %q", machineName, name, emit)
		}
		for _, target := range targets {
			if terminals[target] {
				continue
			}
			if !transitions[target+"/"+emit] {
				return fmt.Errorf("machine %s tool %q emits %q but state %s has no transition", machineName, name, emit, target)
			}
		}
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

func requireDocker(lookPath lookPathFunc) error {
	if _, err := lookPath(dockerEngine); err != nil {
		return fmt.Errorf("docker not found on PATH; install Docker to run the container smoke test")
	}
	return nil
}

func runContainerSmoke(run commandRunner, root, coreRoot, image string) error {
	if err := run(dockerEngine, "run", "--rm", "--entrypoint", "sh", image, "-c", "test ! -e /opt/agent-core/agents"); err != nil {
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

// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/magefiles/appmanifest"
	"gopkg.in/yaml.v3"
)

const (
	applicationModule      = "github.com/Nokia-Bell-Labs/declarative-agents/applications/prose-editor"
	applicationManifest    = "agents/application.yaml"
	canonicalCorpusProfile = "agents/knowledge-manager/corpus-reader/profile.yaml"
	canonicalRelease       = "v0.20260804.0"
	demoConfigFile         = "demo.yaml"
)

var (
	requiredREADMESections = []string{
		"Purpose",
		"Status",
		"Composition",
		"Capabilities",
		"Ownership Boundaries",
		"Run or Planned Entry Points",
		"Verification",
		"Documentation",
	}
	executableLocalRoots = []string{
		"specialist-editor",
		"structure-rag",
		"voice-critic",
		"workflow-orchestrator",
	}
	tracerProfiles = []string{
		"agents/workflow-orchestrator/profile.yaml",
		"agents/specialist-editor/profile.yaml",
		"agents/voice-critic/profile.yaml",
		"agents/structure-rag/profile.yaml",
	}
)

type demoConfig struct {
	CatalogRoot string `yaml:"catalog_root"`
}

type applicationStats struct {
	Agents struct {
		Total    tracerAgentMetrics            `json:"total"`
		PerAgent map[string]tracerAgentMetrics `json:"per_agent"`
	} `json:"agents"`
	Application struct {
		Ownership           string   `json:"ownership"`
		ModuleStatus        string   `json:"module_status"`
		AgentsContributed   int      `json:"agents_contributed"`
		CanonicalReferences int      `json:"canonical_references"`
		CompositionWrappers int      `json:"composition_wrappers"`
		CanonicalProfiles   []string `json:"canonical_profiles"`
		ExecutableRoots     []string `json:"executable_roots"`
	} `json:"application"`
}

type tracerAgentMetrics struct {
	Agents      int `json:"agents,omitempty"`
	States      int `json:"states"`
	Transitions int `json:"transitions"`
	Tools       int `json:"tools"`
	YAML        struct {
		Files int `json:"files"`
		Lines int `json:"lines"`
	} `json:"yaml"`
}

// Audit validates the runnable tracer program closure and documentation corpus.
func Audit() error {
	root, err := applicationRootFromWorkingDirectory()
	if err != nil {
		return err
	}
	count, err := auditApplication(root)
	if err != nil {
		return err
	}
	if err := validateTracerProfileBoot(root); err != nil {
		return err
	}
	fmt.Printf("audit: validated Prose Editor tracer closure and %d YAML documents\n", count)
	return nil
}

// Stats reports the runnable tracer implementation and composition.
func Stats() error {
	root, err := applicationRootFromWorkingDirectory()
	if err != nil {
		return err
	}
	manifest, err := loadApplicationManifest(root)
	if err != nil {
		return err
	}
	stats, err := newStats(root, manifest)
	if err != nil {
		return err
	}
	encoded, err := json.Marshal(stats)
	if err != nil {
		return fmt.Errorf("encode Prose Editor stats: %w", err)
	}
	fmt.Println(string(encoded))
	return nil
}

func auditApplication(root string) (int, error) {
	manifest, err := loadApplicationManifest(root)
	if err != nil {
		return 0, err
	}
	if err := validateTracerManifest(manifest); err != nil {
		return 0, err
	}
	config, err := loadDemoConfig(root)
	if err != nil {
		return 0, err
	}
	catalogRoot, err := resolveCatalogRoot(root, config)
	if err != nil {
		return 0, err
	}
	if _, err := appmanifest.Resolve(manifest, appmanifest.Options{
		ApplicationRoot: root,
		CatalogRoot:     catalogRoot,
	}); err != nil {
		return 0, fmt.Errorf("resolve Prose Editor tracer closure: %w", err)
	}
	count, err := auditYAMLDocuments(root)
	if err != nil {
		return 0, err
	}
	if err := auditREADME(root); err != nil {
		return 0, err
	}
	if err := auditStatusClaims(root); err != nil {
		return 0, err
	}
	return count, nil
}

func loadApplicationManifest(root string) (appmanifest.Manifest, error) {
	config, err := loadDemoConfig(root)
	if err != nil {
		return appmanifest.Manifest{}, err
	}
	catalogRoot, err := resolveCatalogRoot(root, config)
	if err != nil {
		return appmanifest.Manifest{}, err
	}
	if err := requireRegularFile(filepath.Join(catalogRoot, filepath.FromSlash(canonicalCorpusProfile))); err != nil {
		return appmanifest.Manifest{}, fmt.Errorf("canonical corpus-reader dependency: %w", err)
	}
	return appmanifest.Load(
		filepath.Join(root, filepath.FromSlash(applicationManifest)),
		appmanifest.Options{ApplicationRoot: root, CatalogRoot: catalogRoot},
	)
}

func validateTracerManifest(manifest appmanifest.Manifest) error {
	if manifest.Application != "prose-editor" ||
		manifest.Ownership != "agent-owning" ||
		manifest.ModuleStatus != "implemented" {
		return fmt.Errorf("application manifest identity/status is not runnable Prose Editor tracer")
	}
	wantCapabilities := map[string]string{
		"runnable_module": "implemented",
		"managed_service": "not_applicable",
		"packaged":        "not_applicable",
		"helm_managed":    "not_applicable",
		"kind_demo":       "not_applicable",
		"ui":              "not_applicable",
	}
	if len(manifest.Capabilities) != len(wantCapabilities) {
		return fmt.Errorf("application manifest capabilities = %d, want %d",
			len(manifest.Capabilities), len(wantCapabilities))
	}
	for name, want := range wantCapabilities {
		if got := manifest.Capabilities[name].Status; got != want {
			return fmt.Errorf("capability %s status = %q, want %q", name, got, want)
		}
	}
	if manifest.Runtime.MountPath != "" || manifest.Runtime.ImageContainsProfiles ||
		len(manifest.Deployment.Entries) != 0 || len(manifest.UI.Assets) != 0 {
		return errors.New("local-only tracer manifest must not declare a runtime mount, deployment, or UI surface")
	}

	var local, catalog []string
	for _, root := range manifest.Roots {
		if root.Planned {
			return fmt.Errorf("tracer root %s must be executable", root.ID)
		}
		switch root.Ownership {
		case "local":
			local = append(local, root.ID)
		case "catalog":
			if root.ID != "catalog-corpus-reader" ||
				root.Source != canonicalCorpusProfile ||
				root.RuntimePath != "applications/catalog/knowledge-manager/corpus-reader/profile.yaml" ||
				root.CompatibleRelease != canonicalRelease {
				return fmt.Errorf("catalog root %s is not the canonical corpus-reader reference", root.ID)
			}
			catalog = append(catalog, root.ID)
		}
	}
	sort.Strings(local)
	if strings.Join(local, "\x00") != strings.Join(executableLocalRoots, "\x00") {
		return fmt.Errorf("executable local roots = %v, want %v", local, executableLocalRoots)
	}
	if len(catalog) != 1 {
		return fmt.Errorf("canonical roots = %v, want [catalog-corpus-reader]", catalog)
	}
	return nil
}

func auditYAMLDocuments(root string) (int, error) {
	var files []string
	err := filepath.WalkDir(filepath.Join(root, "docs"), func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() && (strings.HasSuffix(entry.Name(), ".yaml") || strings.HasSuffix(entry.Name(), ".yml")) {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("walk Prose Editor documents: %w", err)
	}
	files = append(files,
		filepath.Join(root, filepath.FromSlash(applicationManifest)),
		filepath.Join(root, demoConfigFile),
		filepath.Join(root, ".golangci.yml"),
	)
	sort.Strings(files)
	for _, path := range files {
		if err := parseSingleYAML(path); err != nil {
			return 0, err
		}
	}
	return len(files), nil
}

func parseSingleYAML(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", filepath.ToSlash(path), err)
	}
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	var document yaml.Node
	if err := decoder.Decode(&document); err == io.EOF {
		return nil
	} else if err != nil {
		return fmt.Errorf("parse %s: %w", filepath.ToSlash(path), err)
	}
	if len(document.Content) == 0 {
		return fmt.Errorf("parse %s: empty YAML document", filepath.ToSlash(path))
	}
	var trailing yaml.Node
	if err := decoder.Decode(&trailing); err == nil {
		return fmt.Errorf("parse %s: expected exactly one YAML document", filepath.ToSlash(path))
	} else if err != io.EOF {
		return fmt.Errorf("parse %s: %w", filepath.ToSlash(path), err)
	}
	return nil
}

func auditREADME(root string) error {
	data, err := os.ReadFile(filepath.Join(root, "README.md"))
	if err != nil {
		return fmt.Errorf("read README.md: %w", err)
	}
	content := string(data)
	for _, section := range requiredREADMESections {
		if !strings.Contains(content, "\n## "+section+"\n") {
			return fmt.Errorf("README.md missing required section %q", section)
		}
	}
	return nil
}

func auditStatusClaims(root string) error {
	var architecture struct {
		Overview struct {
			Status string `yaml:"status"`
		} `yaml:"overview"`
		ImplementationStatus struct {
			Overall            string `yaml:"overall"`
			RegistryStatus     string `yaml:"registry_status"`
			ExecutableEvidence string `yaml:"executable_evidence"`
		} `yaml:"implementation_status"`
	}
	if err := readYAML(filepath.Join(root, "docs", "ARCHITECTURE.yaml"), &architecture); err != nil {
		return err
	}
	if architecture.Overview.Status != "implemented_release_00_1_tracer" ||
		architecture.ImplementationStatus.Overall != "implemented_release_00_1_tracer" ||
		architecture.ImplementationStatus.RegistryStatus != "runnable" ||
		architecture.ImplementationStatus.ExecutableEvidence != "interpreter_driven_deterministic_tracer" {
		return errors.New("architecture status must claim only the runnable interpreter-driven deterministic tracer")
	}

	var roadmap struct {
		Releases []struct {
			Version          string `yaml:"version"`
			Status           string `yaml:"status"`
			CapabilityStatus struct {
				RunnableModule string `yaml:"runnable_module"`
			} `yaml:"capability_status"`
			EvidenceStatus struct {
				RootRegistration   string `yaml:"root_registration"`
				ExecutableEvidence string `yaml:"executable_evidence"`
			} `yaml:"evidence_status"`
		} `yaml:"releases"`
	}
	if err := readYAML(filepath.Join(root, "docs", "road-map.yaml"), &roadmap); err != nil {
		return err
	}
	foundRelease := false
	for _, release := range roadmap.Releases {
		if release.Version != "00.1" {
			continue
		}
		foundRelease = true
		if release.Status != "implemented" ||
			release.CapabilityStatus.RunnableModule != "implemented" ||
			release.EvidenceStatus.RootRegistration != "runnable" ||
			release.EvidenceStatus.ExecutableEvidence != "interpreter_driven_deterministic_tracer" {
			return errors.New("road-map release 00.1 must report runnable interpreter-driven tracer evidence")
		}
	}
	if !foundRelease {
		return errors.New("road-map does not contain release 00.1")
	}

	var suite struct {
		Status        string `yaml:"status"`
		RuntimeStatus string `yaml:"runtime_status"`
		TestCases     []struct {
			CurrentEvidence yaml.Node `yaml:"current_evidence"`
		} `yaml:"test_cases"`
	}
	if err := readYAML(filepath.Join(
		root, "docs", "specs", "test-suites", "test-rel00.1-prose-editor.yaml"), &suite); err != nil {
		return err
	}
	if suite.Status != "implemented" || suite.RuntimeStatus != "interpreter_driven_deterministic_tracer" {
		return errors.New("release 00.1 tracer suite must report interpreter-driven deterministic tracer evidence")
	}
	return nil
}

func newStats(root string, manifest appmanifest.Manifest) (applicationStats, error) {
	var result applicationStats
	result.Agents.PerAgent = map[string]tracerAgentMetrics{}
	result.Application.Ownership = manifest.Ownership
	result.Application.ModuleStatus = manifest.ModuleStatus
	result.Application.AgentsContributed = 3
	for _, actor := range []string{"workflow-orchestrator", "specialist-editor", "voice-critic"} {
		metrics, err := scanTracerAgent(filepath.Join(root, "agents", actor))
		if err != nil {
			return result, err
		}
		result.Agents.PerAgent[actor] = metrics
		result.Agents.Total.Agents++
		result.Agents.Total.States += metrics.States
		result.Agents.Total.Transitions += metrics.Transitions
		result.Agents.Total.Tools += metrics.Tools
		result.Agents.Total.YAML.Files += metrics.YAML.Files
		result.Agents.Total.YAML.Lines += metrics.YAML.Lines
	}
	for _, root := range manifest.Roots {
		if root.Ownership == "catalog" {
			result.Application.CanonicalReferences++
			result.Application.CanonicalProfiles = append(
				result.Application.CanonicalProfiles,
				"applications/catalog/"+root.Source,
			)
		} else {
			result.Application.ExecutableRoots = append(result.Application.ExecutableRoots, root.ID)
		}
	}
	result.Application.CompositionWrappers =
		len(result.Application.ExecutableRoots) - result.Application.AgentsContributed
	sort.Strings(result.Application.CanonicalProfiles)
	sort.Strings(result.Application.ExecutableRoots)
	return result, nil
}

func scanTracerAgent(root string) (tracerAgentMetrics, error) {
	var result tracerAgentMetrics
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || (!strings.HasSuffix(path, ".yaml") && !strings.HasSuffix(path, ".yml")) {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		result.YAML.Files++
		result.YAML.Lines += bytes.Count(data, []byte{'\n'})
		switch filepath.Base(path) {
		case "machine.yaml":
			var machine struct {
				States      []yaml.Node `yaml:"states"`
				Transitions []yaml.Node `yaml:"transitions"`
			}
			if err := yaml.Unmarshal(data, &machine); err != nil {
				return err
			}
			result.States += len(machine.States)
			result.Transitions += len(machine.Transitions)
		case "tools.yaml":
			var selection struct {
				Tools []yaml.Node `yaml:"tools"`
			}
			if err := yaml.Unmarshal(data, &selection); err != nil {
				return err
			}
			result.Tools += len(selection.Tools)
		}
		return nil
	})
	return result, err
}

func validateTracerProfileBoot(root string) error {
	coreRoot, err := filepath.Abs(filepath.Join(root, "..", "..", "agent-core"))
	if err != nil {
		return err
	}
	config, err := loadDemoConfig(root)
	if err != nil {
		return err
	}
	catalogRoot, err := resolveCatalogRoot(root, config)
	if err != nil {
		return err
	}
	manifest, err := loadApplicationManifest(root)
	if err != nil {
		return err
	}
	inventory, err := appmanifest.Resolve(manifest, appmanifest.Options{
		ApplicationRoot: root,
		CatalogRoot:     catalogRoot,
	})
	if err != nil {
		return err
	}
	temp, err := os.MkdirTemp("", "prose-editor-agent-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(temp) }()
	profilesRoot := filepath.Join(temp, "profiles")
	for _, file := range inventory.Files {
		var source string
		switch {
		case strings.HasPrefix(file.Source, "application/"):
			source = filepath.Join(root, filepath.FromSlash(strings.TrimPrefix(file.Source, "application/")))
		case strings.HasPrefix(file.Source, "catalog/"):
			source = filepath.Join(catalogRoot, filepath.FromSlash(strings.TrimPrefix(file.Source, "catalog/")))
		default:
			return fmt.Errorf("unknown closure source %s", file.Source)
		}
		data, readErr := os.ReadFile(source)
		if readErr != nil {
			return readErr
		}
		destination := filepath.Join(profilesRoot, filepath.FromSlash(file.RuntimePath))
		if mkdirErr := os.MkdirAll(filepath.Dir(destination), 0o755); mkdirErr != nil {
			return mkdirErr
		}
		if writeErr := os.WriteFile(destination, data, 0o644); writeErr != nil {
			return writeErr
		}
	}
	binary := filepath.Join(temp, "agent")
	build := exec.Command("go", "build", "-o", binary, "./cmd/agent")
	build.Dir = coreRoot
	if output, buildErr := build.CombinedOutput(); buildErr != nil {
		return fmt.Errorf("build agent-core preflight binary: %w\n%s", buildErr, output)
	}
	for _, manifestRoot := range manifest.Roots {
		if manifestRoot.Ownership != "local" {
			continue
		}
		profile := filepath.Join(profilesRoot, filepath.FromSlash(manifestRoot.RuntimePath))
		command := exec.Command(binary, "--validate-config", "--profile", profile, "--core-root", coreRoot)
		if output, runErr := command.CombinedOutput(); runErr != nil {
			return fmt.Errorf("validate tracer profile %s: %w\n%s", manifestRoot.ID, runErr, output)
		}
	}
	return nil
}

func applicationRootFromWorkingDirectory() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("resolve Prose Editor working directory: %w", err)
	}
	current, err := filepath.Abs(filepath.Clean(cwd))
	if err != nil {
		return "", fmt.Errorf("resolve Prose Editor root from %q: %w", cwd, err)
	}
	for {
		data, readErr := os.ReadFile(filepath.Join(current, "go.mod"))
		if readErr == nil && strings.Contains(string(data), "module "+applicationModule) {
			return current, nil
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}
	return "", fmt.Errorf(
		"could not find the Prose Editor root from %s; run from applications/prose-editor or a directory beneath it",
		cwd,
	)
}

func loadDemoConfig(root string) (demoConfig, error) {
	var config demoConfig
	data, err := os.ReadFile(filepath.Join(root, demoConfigFile))
	if err != nil {
		return config, fmt.Errorf("read %s: %w", demoConfigFile, err)
	}
	if err := yaml.Unmarshal(data, &config); err != nil {
		return config, fmt.Errorf("parse %s: %w", demoConfigFile, err)
	}
	return config, nil
}

func resolveCatalogRoot(root string, config demoConfig) (string, error) {
	value := strings.TrimSpace(config.CatalogRoot)
	if value == "" {
		value = filepath.Join(root, "..", "catalog")
	} else if !filepath.IsAbs(value) {
		value = filepath.Join(root, value)
	}
	catalogRoot, err := filepath.Abs(filepath.Clean(value))
	if err != nil {
		return "", fmt.Errorf("resolve catalog_root: %w", err)
	}
	info, err := os.Stat(catalogRoot)
	if err != nil || !info.IsDir() {
		return "", fmt.Errorf("catalog_root is not a directory: %s", catalogRoot)
	}
	return catalogRoot, nil
}

func requireRegularFile(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%s is not a regular file", path)
	}
	return nil
}

func readYAML(path string, out any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", filepath.ToSlash(path), err)
	}
	if err := yaml.Unmarshal(data, out); err != nil {
		return fmt.Errorf("parse %s: %w", filepath.ToSlash(path), err)
	}
	return nil
}

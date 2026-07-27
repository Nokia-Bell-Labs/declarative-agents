// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	applicationModule = "github.com/Nokia-Bell-Labs/declarative-agents/applications/knowledge-manager-demo"
	catalogRootEnv    = "AGENT_CATALOG_ROOT"
	coreRootEnv       = "AGENT_CORE_ROOT"
	canonicalProfile  = "agents/knowledge-manager/documentation-curator/profile.yaml"
)

var requiredDocuments = map[string][]string{
	"docs/VISION.yaml": {
		"id", "title", "executive_summary", "problem", "what_this_does",
		"why_we_build_this", "success_criteria", "not",
	},
	"docs/ARCHITECTURE.yaml": {
		"id", "title", "overview", "interfaces", "components", "design_decisions",
		"technology_choices", "project_structure", "implementation_status", "related_documents",
	},
	"docs/road-map.yaml": {"id", "title", "overview", "releases"},
	"docs/SPECIFICATIONS.yaml": {
		"id", "title", "overview", "roadmap_summary", "foundation_document_index",
		"srd_index", "external_requirement_references", "config_format_index",
		"semantic_model_index", "use_case_index", "test_suite_index", "coverage_gaps",
	},
	"docs/specs/use-cases/rel00.0-uc001-guided-knowledge-manager-demo.yaml": {
		"id", "title", "summary", "actor", "trigger", "preconditions", "flow",
		"touchpoints", "success_criteria", "out_of_scope", "test_suite", "status",
	},
	"docs/specs/test-suites/test-rel00.0-guided-knowledge-manager-demo.yaml": {
		"id", "title", "release", "overview", "traces", "preconditions", "test_cases",
	},
}

type roots struct {
	Application string
	Catalog     string
	Core        string
}

type commandPlan struct {
	Build *exec.Cmd
	Run   *exec.Cmd
}

type specificationIndex struct {
	Foundation []documentIndexEntry `yaml:"foundation_document_index"`
	External   []documentIndexEntry `yaml:"external_requirement_references"`
	UseCases   []useCaseIndexEntry  `yaml:"use_case_index"`
	TestSuites []suiteIndexEntry    `yaml:"test_suite_index"`
}

type documentIndexEntry struct {
	ID   string `yaml:"id"`
	Path string `yaml:"path"`
}

type useCaseIndexEntry struct {
	ID        string `yaml:"id"`
	Path      string `yaml:"path"`
	TestSuite string `yaml:"test_suite"`
}

type suiteIndexEntry struct {
	ID     string   `yaml:"id"`
	Path   string   `yaml:"path"`
	Traces []string `yaml:"traces"`
}

type useCaseDocument struct {
	ID              string `yaml:"id"`
	TestSuite       string `yaml:"test_suite"`
	SuccessCriteria []struct {
		ID string `yaml:"id"`
	} `yaml:"success_criteria"`
}

type testSuiteDocument struct {
	ID        string   `yaml:"id"`
	Traces    []string `yaml:"traces"`
	TestCases []struct {
		ID      string   `yaml:"id"`
		UseCase string   `yaml:"use_case"`
		Traces  []string `yaml:"traces"`
	} `yaml:"test_cases"`
}

type statsOutput struct {
	Application struct {
		Ownership           string `json:"ownership"`
		AgentsContributed   int    `json:"agents_contributed"`
		CanonicalReferences int    `json:"canonical_references"`
		CanonicalProfile    string `json:"canonical_profile"`
	} `json:"application"`
}

// Run builds agent-core and starts the canonical documentation-curator profile.
func Run() error {
	resolved, err := resolveRootsFromWorkingDirectory()
	if err != nil {
		return err
	}
	temp, err := os.MkdirTemp("", "knowledge-manager-demo-*")
	if err != nil {
		return fmt.Errorf("create agent build directory: %w", err)
	}
	defer os.RemoveAll(temp)

	plan := runCommandPlan(resolved, filepath.Join(temp, "agent"))
	plan.Build.Stdout, plan.Build.Stderr = os.Stdout, os.Stderr
	if err := plan.Build.Run(); err != nil {
		return fmt.Errorf("build agent-core runtime: %w", err)
	}
	plan.Run.Stdin, plan.Run.Stdout, plan.Run.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := plan.Run.Run(); err != nil {
		return fmt.Errorf("run documentation-curator: %w", err)
	}
	return nil
}

// Presentation serves knowledge-manager.slide with the module-pinned Go tool.
func Presentation() error {
	root, err := applicationRootFromWorkingDirectory()
	if err != nil {
		return err
	}
	cmd := presentationCommand(root)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("serve Knowledge Manager presentation: %w", err)
	}
	return nil
}

// Present is a short alias for Presentation.
func Present() error {
	return Presentation()
}

// Audit validates the demo's documents, traces, ownership, and portable paths.
func Audit() error {
	root, err := applicationRootFromWorkingDirectory()
	if err != nil {
		return err
	}
	if err := auditApplication(root); err != nil {
		return err
	}
	fmt.Printf("audit: validated %d Knowledge Manager demo YAML documents\n", len(requiredDocuments))
	return nil
}

// Stats emits composition ownership without an agents section.
func Stats() error {
	encoded, err := json.Marshal(newStatsOutput())
	if err != nil {
		return fmt.Errorf("encode Knowledge Manager demo stats: %w", err)
	}
	fmt.Println(string(encoded))
	return nil
}

func resolveRootsFromWorkingDirectory() (roots, error) {
	root, err := applicationRootFromWorkingDirectory()
	if err != nil {
		return roots{}, err
	}
	return resolveRoots(root, os.Getenv)
}

func applicationRootFromWorkingDirectory() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("resolve Knowledge Manager demo working directory: %w", err)
	}
	return findApplicationRoot(cwd)
}

func findApplicationRoot(start string) (string, error) {
	current, err := filepath.Abs(filepath.Clean(start))
	if err != nil {
		return "", fmt.Errorf("resolve Knowledge Manager demo root from %q: %w", start, err)
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
		"Knowledge Manager demo root not found from %s; run from applications/knowledge-manager-demo or a directory beneath it",
		start,
	)
}

func resolveRoots(application string, getenv func(string) string) (roots, error) {
	catalog, err := ownerRoot(getenv(catalogRootEnv), application, filepath.Join(application, "..", "catalog"))
	if err != nil {
		return roots{}, fmt.Errorf("resolve %s: %w", catalogRootEnv, err)
	}
	core, err := ownerRoot(getenv(coreRootEnv), application, filepath.Join(application, "..", "..", "agent-core"))
	if err != nil {
		return roots{}, fmt.Errorf("resolve %s: %w", coreRootEnv, err)
	}
	resolved := roots{Application: application, Catalog: catalog, Core: core}
	if err := requireFile(filepath.Join(catalog, filepath.FromSlash(canonicalProfile)),
		"canonical documentation-curator profile", catalogRootEnv); err != nil {
		return roots{}, err
	}
	if err := requireFile(filepath.Join(core, "go.mod"), "agent-core checkout", coreRootEnv); err != nil {
		return roots{}, err
	}
	if info, statErr := os.Stat(filepath.Join(core, "cmd", "agent")); statErr != nil || !info.IsDir() {
		return roots{}, fmt.Errorf(
			"agent-core command directory not found at %s; set %s to an agent-core checkout",
			filepath.Join(core, "cmd", "agent"), coreRootEnv,
		)
	}
	return resolved, nil
}

func ownerRoot(value, application, fallback string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		value = fallback
	} else if !filepath.IsAbs(value) {
		value = filepath.Join(application, value)
	}
	return filepath.Abs(filepath.Clean(value))
}

func requireFile(path, label, environment string) error {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return fmt.Errorf("%s not found at %s; set %s to the owning checkout", label, path, environment)
	}
	return nil
}

func runCommandPlan(resolved roots, binary string) commandPlan {
	build := exec.Command("go", "build", "-tags", "production", "-o", binary, "./cmd/agent")
	build.Dir = resolved.Core
	run := exec.Command(
		binary,
		"--profile", filepath.Join(resolved.Catalog, filepath.FromSlash(canonicalProfile)),
		"--directory", resolved.Catalog,
		"--core-root", resolved.Core,
	)
	run.Dir = resolved.Catalog
	return commandPlan{Build: build, Run: run}
}

func presentationCommand(application string) *exec.Cmd {
	cmd := exec.Command("go", "tool", "present", "-play=false", "knowledge-manager.slide")
	cmd.Dir = application
	return cmd
}

func auditApplication(root string) error {
	loaded := make(map[string]map[string]any, len(requiredDocuments))
	for path, fields := range requiredDocuments {
		var document map[string]any
		if err := readYAML(filepath.Join(root, filepath.FromSlash(path)), &document); err != nil {
			return err
		}
		var missing []string
		for _, field := range fields {
			if _, ok := document[field]; !ok {
				missing = append(missing, field)
			}
		}
		if len(missing) > 0 {
			return fmt.Errorf("%s missing required fields: %s", path, strings.Join(missing, ", "))
		}
		loaded[path] = document
	}
	if err := auditTraceability(root); err != nil {
		return err
	}
	if err := auditOwnedSources(root); err != nil {
		return err
	}
	catalog, err := ownerRoot(os.Getenv(catalogRootEnv), root, filepath.Join(root, "..", "catalog"))
	if err != nil {
		return fmt.Errorf("resolve canonical profile owner: %w", err)
	}
	if err := requireFile(filepath.Join(catalog, filepath.FromSlash(canonicalProfile)),
		"canonical documentation-curator profile", catalogRootEnv); err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(canonicalProfile))); !os.IsNotExist(err) {
		return fmt.Errorf("application must not contain a copy of canonical profile %s", canonicalProfile)
	}
	return nil
}

func auditTraceability(root string) error {
	var index specificationIndex
	if err := readYAML(filepath.Join(root, "docs", "SPECIFICATIONS.yaml"), &index); err != nil {
		return err
	}
	for _, entry := range append(append([]documentIndexEntry{}, index.Foundation...), index.External...) {
		if entry.ID == "" || entry.Path == "" {
			return fmt.Errorf("SPECIFICATIONS document index has an empty id or path")
		}
		if err := requireIndexedFile(root, entry.Path); err != nil {
			return err
		}
	}
	if len(index.UseCases) != 1 || len(index.TestSuites) != 1 {
		return fmt.Errorf("SPECIFICATIONS must index exactly one use case and one test suite")
	}
	useEntry, suiteEntry := index.UseCases[0], index.TestSuites[0]
	if err := requireIndexedFile(root, useEntry.Path); err != nil {
		return err
	}
	if err := requireIndexedFile(root, suiteEntry.Path); err != nil {
		return err
	}
	var useCase useCaseDocument
	if err := readYAML(filepath.Join(root, filepath.FromSlash(useEntry.Path)), &useCase); err != nil {
		return err
	}
	var suite testSuiteDocument
	if err := readYAML(filepath.Join(root, filepath.FromSlash(suiteEntry.Path)), &suite); err != nil {
		return err
	}
	if useEntry.ID != useCase.ID || suiteEntry.ID != suite.ID {
		return fmt.Errorf("SPECIFICATIONS ids do not match their indexed documents")
	}
	if useEntry.TestSuite != suite.ID || useCase.TestSuite != suite.ID {
		return fmt.Errorf("use case %s must name reciprocal test suite %s", useCase.ID, suite.ID)
	}
	if !slices.Contains(suiteEntry.Traces, useCase.ID) || !slices.Contains(suite.Traces, useCase.ID) {
		return fmt.Errorf("test suite %s must trace use case %s in its index and document", suite.ID, useCase.ID)
	}
	criterionTraces := make(map[string]bool, len(useCase.SuccessCriteria))
	for _, criterion := range useCase.SuccessCriteria {
		criterionTraces[useCase.ID+" "+criterion.ID] = false
	}
	for _, testCase := range suite.TestCases {
		if testCase.ID == "" || testCase.UseCase != useCase.ID {
			return fmt.Errorf("test case %q must name use case %s", testCase.ID, useCase.ID)
		}
		for _, trace := range testCase.Traces {
			if _, ok := criterionTraces[trace]; ok {
				criterionTraces[trace] = true
			}
		}
	}
	for trace, covered := range criterionTraces {
		if !covered {
			return fmt.Errorf("use-case criterion %s has no reciprocal test-case trace", trace)
		}
	}
	return nil
}

func requireIndexedFile(root, path string) error {
	info, err := os.Stat(filepath.Join(root, filepath.FromSlash(path)))
	if err != nil || info.IsDir() {
		return fmt.Errorf("indexed path does not exist: %s", path)
	}
	return nil
}

func auditOwnedSources(root string) error {
	stalePaths := []string{
		"applications/catalog/demo/",
		"../catalog/demo/",
	}
	developerPath := regexp.MustCompile(`(?:/Users/|/home/|[A-Za-z]:\\)`)
	profileReferenceSeen := false
	scanRoots := []string{"docs", "call-lifecycle-exit", "README.md", "knowledge-manager.slide"}
	for _, relative := range scanRoots {
		path := filepath.Join(root, relative)
		err := filepath.WalkDir(path, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				return nil
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			content := string(data)
			for _, stale := range stalePaths {
				if strings.Contains(content, stale) {
					return fmt.Errorf("%s contains stale pre-relocation path %q", filepath.ToSlash(path), stale)
				}
			}
			if developerPath.MatchString(content) {
				return fmt.Errorf("%s contains a developer-specific absolute path", filepath.ToSlash(path))
			}
			if strings.Contains(content, "documentation-curator/profile.yaml") {
				profileReferenceSeen = true
				if !strings.Contains(content, canonicalProfile) {
					return fmt.Errorf("%s contains a non-canonical documentation-curator profile reference", filepath.ToSlash(path))
				}
			}
			return nil
		})
		if err != nil {
			return err
		}
	}
	if !profileReferenceSeen {
		return fmt.Errorf("application does not reference canonical profile %s", canonicalProfile)
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

func newStatsOutput() statsOutput {
	var output statsOutput
	output.Application.Ownership = "composition"
	output.Application.AgentsContributed = 0
	output.Application.CanonicalReferences = 1
	output.Application.CanonicalProfile = "applications/catalog/" + canonicalProfile
	return output
}

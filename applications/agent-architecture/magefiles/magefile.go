// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/magefiles/appmanifest"
	"gopkg.in/yaml.v3"
)

const (
	applicationModule      = "github.com/Nokia-Bell-Labs/declarative-agents/applications/agent-architecture"
	demoConfigFile         = "demo.yaml"
	defaultCatalogRoot     = "../catalog"
	defaultCoreRoot        = "../../agent-core"
	canonicalProfile       = "agents/knowledge-manager/documentation-curator/profile.yaml"
	collectorProfile       = "agents/collector/profile.yaml"
	collectorHealthTimeout = 15 * time.Second
)

// demoConfig carries the optional, declarative overrides the agent-architecture
// demo reads from demo.yaml. Every field is optional: an absent file or an unset
// field falls back to the monorepo default, so mage run needs no configuration.
// Overriding a value means editing this declaration — never an environment
// variable. Tracing is a pointer so an omitted key is distinguishable from an
// explicit false.
type demoConfig struct {
	CatalogRoot string `yaml:"catalog_root"`
	CoreRoot    string `yaml:"core_root"`
	Tracing     *bool  `yaml:"tracing"`
	HelmDist    string `yaml:"helm_dist"`
	Image       string `yaml:"image"`
}

// loadDemoConfig reads demo.yaml from the application root. A missing file is the
// zero-configuration path and yields an empty config, not an error; every
// resolver treats empty fields as "use the default".
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
	"docs/specs/use-cases/rel00.0-uc001-guided-agent-architecture.yaml": {
		"id", "title", "summary", "actor", "trigger", "preconditions", "flow",
		"touchpoints", "success_criteria", "out_of_scope", "test_suite", "status",
	},
	"docs/specs/test-suites/test-rel00.0-guided-agent-architecture.yaml": {
		"id", "title", "release", "overview", "traces", "preconditions", "test_cases",
	},
	"docs/specs/use-cases/rel01.0-uc001-demo-trace-collection.yaml": {
		"id", "title", "summary", "actor", "trigger", "preconditions", "flow",
		"success_criteria", "out_of_scope", "test_suite", "status",
	},
	"docs/specs/test-suites/test-rel01.0-demo-trace-collection.yaml": {
		"id", "title", "release", "overview", "traces", "preconditions", "test_cases",
	},
}

// roots holds the checkout locations and the demo.yaml-declared settings
// resolved for one run: the ownership roots plus the tracing toggle, chart
// output directory, and runtime image reference every target reads instead of an
// environment variable.
type roots struct {
	Application string
	Catalog     string
	Core        string
	Tracing     bool
	HelmDist    string
	Image       string
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
		CompositionWrappers int    `json:"composition_wrappers"`
		DeploymentEntries   int    `json:"deployment_entries"`
		UIAssets            int    `json:"ui_assets"`
		PackageAssets       int    `json:"package_assets"`
	} `json:"application"`
}

// Run builds agent-core, optionally starts the collector agent for trace
// collection, and starts the canonical documentation-curator profile.
// Set tracing to false in demo.yaml to disable trace collection.
func Run() error {
	resolved, err := resolveRootsFromWorkingDirectory()
	if err != nil {
		return err
	}
	temp, err := os.MkdirTemp("", "agent-architecture-*")
	if err != nil {
		return fmt.Errorf("create agent build directory: %w", err)
	}
	defer func() { _ = os.RemoveAll(temp) }()

	binary := filepath.Join(temp, "agent")
	plan := runCommandPlan(resolved, binary)
	plan.Build.Stdout, plan.Build.Stderr = os.Stdout, os.Stderr
	if err := plan.Build.Run(); err != nil {
		return fmt.Errorf("build agent-core runtime: %w", err)
	}

	tracing := resolved.Tracing
	var collectorCleanup func()
	if tracing {
		var endpoints collectorEndpoints
		endpoints, collectorCleanup, err = startCollectorAgent(resolved, binary)
		if err != nil {
			return err
		}
		plan.Run.Args = append(plan.Run.Args,
			"--otel-otlp-endpoint", endpoints.ReceiverAddress,
			"--otel-service-name", "knowledge-manager-curator")
		fmt.Printf("collector query: %s/query/traces\n", endpoints.QueryAddress)
	}

	plan.Run.Stdin, plan.Run.Stdout, plan.Run.Stderr = os.Stdin, os.Stdout, os.Stderr
	runErr := plan.Run.Run()

	if collectorCleanup != nil {
		collectorCleanup()
	}

	if runErr != nil {
		return fmt.Errorf("run documentation-curator: %w", runErr)
	}
	return nil
}

// Presentation serves agent-architecture.slide with the module-pinned Go tool.
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
	fmt.Printf("audit: validated %d Agent Architecture YAML documents\n", len(requiredDocuments))
	return nil
}

// Stats emits composition ownership without an agents section.
func Stats() error {
	resolved, err := resolveRootsFromWorkingDirectory()
	if err != nil {
		return err
	}
	manifest, err := appmanifest.Load(
		filepath.Join(resolved.Application, "agents", "application.yaml"),
		appmanifest.Options{ApplicationRoot: resolved.Application, CatalogRoot: resolved.Catalog})
	if err != nil {
		return err
	}
	encoded, err := json.Marshal(newStatsOutput(manifest))
	if err != nil {
		return fmt.Errorf("encode Agent Architecture stats: %w", err)
	}
	fmt.Println(string(encoded))
	return nil
}

func resolveRootsFromWorkingDirectory() (roots, error) {
	root, err := applicationRootFromWorkingDirectory()
	if err != nil {
		return roots{}, err
	}
	config, err := loadDemoConfig(root)
	if err != nil {
		return roots{}, err
	}
	return resolveRoots(root, config)
}

func applicationRootFromWorkingDirectory() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("resolve Agent Architecture working directory: %w", err)
	}
	return findApplicationRoot(cwd)
}

func findApplicationRoot(start string) (string, error) {
	current, err := filepath.Abs(filepath.Clean(start))
	if err != nil {
		return "", fmt.Errorf("resolve Agent Architecture root from %q: %w", start, err)
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
		"could not find the Agent Architecture root from %s; run from applications/agent-architecture or a directory beneath it",
		start,
	)
}

func resolveRoots(application string, config demoConfig) (roots, error) {
	catalog, err := ownerRoot(config.CatalogRoot, application, filepath.Join(application, filepath.FromSlash(defaultCatalogRoot)))
	if err != nil {
		return roots{}, fmt.Errorf("resolve catalog_root: %w", err)
	}
	core, err := ownerRoot(config.CoreRoot, application, filepath.Join(application, filepath.FromSlash(defaultCoreRoot)))
	if err != nil {
		return roots{}, fmt.Errorf("resolve core_root: %w", err)
	}
	resolved := roots{
		Application: application,
		Catalog:     catalog,
		Core:        core,
		Tracing:     tracingEnabled(config),
		HelmDist:    helmDistDirectory(application, config),
		Image:       imageReference(config),
	}
	if err := requireFile(filepath.Join(catalog, filepath.FromSlash(canonicalProfile)),
		"canonical documentation-curator profile", "catalog_root"); err != nil {
		return roots{}, err
	}
	if err := requireFile(filepath.Join(catalog, filepath.FromSlash(collectorProfile)),
		"canonical collector profile", "catalog_root"); err != nil {
		return roots{}, err
	}
	if err := requireFile(filepath.Join(core, "go.mod"), "agent-core checkout", "core_root"); err != nil {
		return roots{}, err
	}
	if info, statErr := os.Stat(filepath.Join(core, "cmd", "agent")); statErr != nil || !info.IsDir() {
		return roots{}, fmt.Errorf(
			"agent-core command directory not found at %s; set core_root in %s to an agent-core checkout",
			filepath.Join(core, "cmd", "agent"), demoConfigFile,
		)
	}
	return resolved, nil
}

// helmDistDirectory resolves the chart package output directory: the demo.yaml
// helm_dist override when set (relative values are anchored at the application
// root), otherwise the default helm/dist under the application.
func helmDistDirectory(application string, config demoConfig) string {
	if config.HelmDist == "" {
		return filepath.Join(application, "helm", "dist")
	}
	if filepath.IsAbs(config.HelmDist) {
		return filepath.Clean(config.HelmDist)
	}
	return filepath.Join(application, config.HelmDist)
}

// imageReference resolves the runtime image reference: the demo.yaml image
// override when set, otherwise the built-in repository and tag.
func imageReference(config demoConfig) string {
	if config.Image == "" {
		return agentArchitectureImageRepository + ":" + agentArchitectureImageTag
	}
	return config.Image
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

func requireFile(path, label, configKey string) error {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return fmt.Errorf("%s not found at %s; set %s in %s to the owning checkout", label, path, configKey, demoConfigFile)
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
	cmd := exec.Command("go", "tool", "present", "-play=false", "agent-architecture.slide")
	cmd.Dir = application
	return cmd
}

// tracingEnabled reports whether the collector should start. Tracing is on
// unless demo.yaml sets tracing to false; an omitted key keeps the default.
func tracingEnabled(config demoConfig) bool {
	return config.Tracing == nil || *config.Tracing
}

func collectorCommand(
	resolved roots,
	binary, spoolDir string,
	endpoints collectorEndpoints,
) *exec.Cmd {
	cmd := exec.Command(
		binary,
		"--profile", filepath.Join(resolved.Catalog, filepath.FromSlash(collectorProfile)),
		"--directory", resolved.Catalog,
		"--core-root", resolved.Core,
	)
	cmd.Dir = resolved.Catalog
	cmd.Env = append(os.Environ(),
		"COLLECTOR_MODE=spool",
		"COLLECTOR_BIND_HOST=127.0.0.1",
		"COLLECTOR_RECEIVER_ADDRESS="+endpoints.ReceiverAddress,
		"COLLECTOR_CONTROL_PORT="+endpoints.ControlPort,
		"COLLECTOR_MONITOR_PORT="+endpoints.MonitorPort,
		"COLLECTOR_QUERY_PORT="+endpoints.QueryPort,
		"COLLECTOR_SPOOL_PATH="+spoolDir,
	)
	return cmd
}

func startCollectorAgent(
	resolved roots,
	binary string,
) (collectorEndpoints, func(), error) {
	spoolDir, err := os.MkdirTemp("", "collector-spool-*")
	if err != nil {
		return collectorEndpoints{}, nil,
			fmt.Errorf("create collector spool directory: %w", err)
	}
	reservation, err := reserveCollectorEndpoints()
	if err != nil {
		_ = os.RemoveAll(spoolDir)
		return collectorEndpoints{}, nil, err
	}
	endpoints := reservation.endpoints
	// COLLECTOR_SPOOL_PATH names the spool file, not its directory; the
	// collector reads a directory path as NDJSON and fails (GH-1168).
	cmd := collectorCommand(
		resolved, binary, filepath.Join(spoolDir, "collector.ndjson"), endpoints)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	reservation.release()
	if err := cmd.Start(); err != nil {
		_ = os.RemoveAll(spoolDir)
		return collectorEndpoints{}, nil, fmt.Errorf("start collector agent: %w", err)
	}
	if err := waitCollectorHealth(endpoints.ControlAddress); err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		_ = os.RemoveAll(spoolDir)
		return collectorEndpoints{}, nil, err
	}
	cleanup := func() {
		postCollectorExit(endpoints.ControlAddress)
		_ = cmd.Wait()
		_ = os.RemoveAll(spoolDir)
	}
	return endpoints, cleanup, nil
}

func waitCollectorHealth(controlAddress string) error {
	deadline := time.Now().Add(collectorHealthTimeout)
	client := &http.Client{Timeout: 2 * time.Second}
	for time.Now().Before(deadline) {
		resp, err := client.Get(collectorHealthURL(controlAddress))
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		time.Sleep(250 * time.Millisecond)
	}
	return fmt.Errorf("collector agent did not become healthy within %s", collectorHealthTimeout)
}

// collectorHealthURL and collectorExitURL follow the collector control
// server's lifecycle routes (agents/collector/rest.yaml).
func collectorHealthURL(controlAddress string) string {
	return controlAddress + "/api/lifecycle/health"
}

func collectorExitURL(controlAddress string) string {
	return controlAddress + "/api/lifecycle/exit"
}

func postCollectorExit(controlAddress string) {
	if err := postExitRequest(collectorExitURL(controlAddress)); err != nil {
		fmt.Fprintf(os.Stderr, "warning: collector exit request failed, collector may keep its ports: %v\n", err)
	}
}

// postExitRequest sends the JSON body the exit route requires; a nil body is
// rejected with 400 and the collector keeps running, which leaks its ports
// into the next demo run (GH-1195).
func postExitRequest(url string) error {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Post(url, "application/json", strings.NewReader(`{"reason":"demo cleanup"}`))
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusAccepted {
		return fmt.Errorf("exit route returned %s", resp.Status)
	}
	return nil
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
	config, err := loadDemoConfig(root)
	if err != nil {
		return err
	}
	catalog, err := ownerRoot(config.CatalogRoot, root, filepath.Join(root, filepath.FromSlash(defaultCatalogRoot)))
	if err != nil {
		return fmt.Errorf("resolve canonical profile owner: %w", err)
	}
	if err := requireFile(filepath.Join(catalog, filepath.FromSlash(canonicalProfile)),
		"canonical documentation-curator profile", "catalog_root"); err != nil {
		return err
	}
	if _, err := appmanifest.Load(filepath.Join(root, "agents", "application.yaml"),
		appmanifest.Options{ApplicationRoot: root, CatalogRoot: catalog}); err != nil {
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
	if len(index.UseCases) == 0 || len(index.TestSuites) == 0 {
		return fmt.Errorf("SPECIFICATIONS must index at least one use case and one test suite")
	}
	if len(index.UseCases) != len(index.TestSuites) {
		return fmt.Errorf("SPECIFICATIONS must index the same number of use cases (%d) and test suites (%d)",
			len(index.UseCases), len(index.TestSuites))
	}
	suitesByID := make(map[string]suiteIndexEntry, len(index.TestSuites))
	for _, entry := range index.TestSuites {
		suitesByID[entry.ID] = entry
	}
	for _, useEntry := range index.UseCases {
		if err := requireIndexedFile(root, useEntry.Path); err != nil {
			return err
		}
		var useCase useCaseDocument
		if err := readYAML(filepath.Join(root, filepath.FromSlash(useEntry.Path)), &useCase); err != nil {
			return err
		}
		if useEntry.ID != useCase.ID {
			return fmt.Errorf("SPECIFICATIONS id %s does not match document id %s", useEntry.ID, useCase.ID)
		}
		suiteEntry, ok := suitesByID[useEntry.TestSuite]
		if !ok {
			return fmt.Errorf("use case %s names test suite %s which is not in SPECIFICATIONS", useCase.ID, useEntry.TestSuite)
		}
		if err := requireIndexedFile(root, suiteEntry.Path); err != nil {
			return err
		}
		var suite testSuiteDocument
		if err := readYAML(filepath.Join(root, filepath.FromSlash(suiteEntry.Path)), &suite); err != nil {
			return err
		}
		if suiteEntry.ID != suite.ID {
			return fmt.Errorf("SPECIFICATIONS id %s does not match document id %s", suiteEntry.ID, suite.ID)
		}
		if useCase.TestSuite != suite.ID {
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
	scanRoots := []string{"docs", "README.md", "agent-architecture.slide"}
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

func newStatsOutput(manifest appmanifest.Manifest) statsOutput {
	var output statsOutput
	output.Application.Ownership = manifest.Ownership
	output.Application.AgentsContributed = 0
	for _, root := range manifest.Roots {
		if root.Ownership == "catalog" && !root.Planned {
			output.Application.CanonicalReferences++
		} else if root.Ownership == "local" && !root.Planned {
			output.Application.CompositionWrappers++
		}
	}
	output.Application.CanonicalProfile = "applications/catalog/" + canonicalProfile
	output.Application.DeploymentEntries = len(manifest.Deployment.Entries)
	output.Application.UIAssets = len(manifest.UI.Assets)
	output.Application.PackageAssets = len(manifest.Package.Assets)
	return output
}

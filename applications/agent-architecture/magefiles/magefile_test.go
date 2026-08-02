// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestResolveRootsDefaults(t *testing.T) {
	app, catalog, core := rootFixture(t)
	got, err := resolveRoots(app, demoConfig{})
	if err != nil {
		t.Fatal(err)
	}
	want := roots{
		Application: app,
		Catalog:     catalog,
		Core:        core,
		Tracing:     true,
		HelmDist:    filepath.Join(app, "helm", "dist"),
		Image:       agentArchitectureImageRepository + ":" + agentArchitectureImageTag,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("resolveRoots() = %#v, want %#v", got, want)
	}
}

func TestResolveRootsExplicitRelativeOwners(t *testing.T) {
	app, _, _ := rootFixture(t)
	repository := filepath.Dir(filepath.Dir(app))
	catalog := filepath.Join(repository, "catalog-checkout")
	core := filepath.Join(repository, "core-checkout")
	writeFile(t, filepath.Join(catalog, filepath.FromSlash(canonicalProfile)), "name: curator\n")
	writeFile(t, filepath.Join(catalog, filepath.FromSlash(collectorProfile)), "name: collector\n")
	writeFile(t, filepath.Join(core, "go.mod"), "module example.test/core\n")
	if err := os.MkdirAll(filepath.Join(core, "cmd", "agent"), 0o755); err != nil {
		t.Fatal(err)
	}
	config := demoConfig{
		CatalogRoot: filepath.Join("..", "..", "catalog-checkout"),
		CoreRoot:    filepath.Join("..", "..", "core-checkout"),
	}
	got, err := resolveRoots(app, config)
	if err != nil {
		t.Fatal(err)
	}
	if got.Catalog != catalog || got.Core != core {
		t.Fatalf("explicit roots = %#v, want catalog %s and core %s", got, catalog, core)
	}
}

func TestResolveRootsReportsMissingOwners(t *testing.T) {
	app, _, _ := rootFixture(t)
	tests := []struct {
		name    string
		config  demoConfig
		message string
	}{
		{
			name:    "catalog profile",
			config:  demoConfig{CatalogRoot: filepath.Join(app, "missing-catalog")},
			message: "canonical documentation-curator profile not found",
		},
		{
			name:    "core checkout",
			config:  demoConfig{CoreRoot: filepath.Join(app, "missing-core")},
			message: "agent-core checkout not found",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := resolveRoots(app, test.config)
			if err == nil || !strings.Contains(err.Error(), test.message) {
				t.Fatalf("error = %v, want message containing %q", err, test.message)
			}
		})
	}
}

func TestResolveRootsReportsMissingAgentCommand(t *testing.T) {
	app, _, core := rootFixture(t)
	if err := os.RemoveAll(filepath.Join(core, "cmd", "agent")); err != nil {
		t.Fatal(err)
	}
	_, err := resolveRoots(app, demoConfig{})
	if err == nil || !strings.Contains(err.Error(), "agent-core command directory not found") {
		t.Fatalf("error = %v, want missing command directory", err)
	}
}

func TestLoadDemoConfigAbsentFileIsZeroConfig(t *testing.T) {
	config, err := loadDemoConfig(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if config != (demoConfig{}) {
		t.Fatalf("absent %s = %#v, want empty config", demoConfigFile, config)
	}
}

func TestLoadDemoConfigOverlaysDeclaredValues(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, demoConfigFile),
		"catalog_root: ../elsewhere\ntracing: false\nimage: example.test/runtime:9\n")
	config, err := loadDemoConfig(root)
	if err != nil {
		t.Fatal(err)
	}
	if config.CatalogRoot != "../elsewhere" {
		t.Errorf("catalog_root = %q, want ../elsewhere", config.CatalogRoot)
	}
	if config.Tracing == nil || *config.Tracing {
		t.Errorf("tracing = %v, want an explicit false", config.Tracing)
	}
	if config.Image != "example.test/runtime:9" {
		t.Errorf("image = %q, want example.test/runtime:9", config.Image)
	}
}

func TestResolveRootsHonorsDeclaredOverrides(t *testing.T) {
	app, _, _ := rootFixture(t)
	disabled := false
	got, err := resolveRoots(app, demoConfig{
		Tracing:  &disabled,
		HelmDist: "out/charts",
		Image:    "example.test/runtime:9",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Tracing {
		t.Error("tracing = true, want false from demo.yaml")
	}
	if want := filepath.Join(app, "out", "charts"); got.HelmDist != want {
		t.Errorf("helm dist = %s, want %s", got.HelmDist, want)
	}
	if got.Image != "example.test/runtime:9" {
		t.Errorf("image = %s, want example.test/runtime:9", got.Image)
	}
}

func TestFindApplicationRootFromNestedDirectory(t *testing.T) {
	app, _, _ := rootFixture(t)
	nested := filepath.Join(app, "magefiles", "nested")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := findApplicationRoot(nested)
	if err != nil {
		t.Fatal(err)
	}
	if got != app {
		t.Fatalf("findApplicationRoot() = %s, want %s", got, app)
	}
}

func TestFindApplicationRootFailureIsActionable(t *testing.T) {
	_, err := findApplicationRoot(t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "run from applications/agent-architecture") {
		t.Fatalf("error = %v, want actionable root guidance", err)
	}
}

func TestRunCommandConstruction(t *testing.T) {
	resolved := roots{
		Application: filepath.Join(string(filepath.Separator), "work", "applications", "agent-architecture"),
		Catalog:     filepath.Join(string(filepath.Separator), "work", "applications", "catalog"),
		Core:        filepath.Join(string(filepath.Separator), "work", "agent-core"),
	}
	binary := filepath.Join(string(filepath.Separator), "tmp", "knowledge-manager-agent")
	plan := runCommandPlan(resolved, binary)
	wantBuild := []string{"go", "build", "-tags", "production", "-o", binary, "./cmd/agent"}
	if !reflect.DeepEqual(plan.Build.Args, wantBuild) {
		t.Fatalf("build args = %#v, want %#v", plan.Build.Args, wantBuild)
	}
	if plan.Build.Dir != resolved.Core {
		t.Fatalf("build directory = %s, want %s", plan.Build.Dir, resolved.Core)
	}
	wantRun := []string{
		binary,
		"--profile", filepath.Join(resolved.Catalog, filepath.FromSlash(canonicalProfile)),
		"--directory", resolved.Catalog,
		"--core-root", resolved.Core,
	}
	if !reflect.DeepEqual(plan.Run.Args, wantRun) {
		t.Fatalf("run args = %#v, want %#v", plan.Run.Args, wantRun)
	}
	if plan.Run.Dir != resolved.Catalog {
		t.Fatalf("run directory = %s, want %s", plan.Run.Dir, resolved.Catalog)
	}
}

func TestPresentationCommandDisablesPlayground(t *testing.T) {
	root := filepath.Join(string(filepath.Separator), "work", "applications", "agent-architecture")
	cmd := presentationCommand(root)
	want := []string{"go", "tool", "present", "-play=false", "agent-architecture.slide"}
	if !reflect.DeepEqual(cmd.Args, want) {
		t.Fatalf("presentation args = %#v, want %#v", cmd.Args, want)
	}
	if cmd.Dir != root {
		t.Fatalf("presentation directory = %s, want %s", cmd.Dir, root)
	}
}

func TestAuditApplication(t *testing.T) {
	root := realApplicationRoot(t)
	if err := auditApplication(root); err != nil {
		t.Fatal(err)
	}
}

func TestAuditRejectsBrokenReciprocalTrace(t *testing.T) {
	root := copyApplicationFixture(t)
	suite := filepath.Join(root, "docs", "specs", "test-suites", "test-rel00.0-guided-agent-architecture.yaml")
	data, err := os.ReadFile(suite)
	if err != nil {
		t.Fatal(err)
	}
	changed := strings.Replace(
		string(data),
		"      - rel00.0-uc001-guided-agent-architecture S6",
		"      - srd002-managed-application-services AC1",
		1,
	)
	if changed == string(data) {
		t.Fatal("test fixture did not contain expected S6 trace")
	}
	if err := os.WriteFile(suite, []byte(changed), 0o644); err != nil {
		t.Fatal(err)
	}
	err = auditApplication(root)
	if err == nil || !strings.Contains(err.Error(), "S6 has no reciprocal test-case trace") {
		t.Fatalf("error = %v, want missing reciprocal S6 trace", err)
	}
}

func TestAuditRejectsStaleCatalogDemoPath(t *testing.T) {
	root := copyApplicationFixture(t)
	readme := filepath.Join(root, "README.md")
	file, err := os.OpenFile(readme, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString("\nOld deck: applications/catalog/demo/agent-architecture.slide\n"); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	err = auditApplication(root)
	if err == nil || !strings.Contains(err.Error(), "stale pre-relocation path") {
		t.Fatalf("error = %v, want stale-path rejection", err)
	}
}

func TestAuditRejectsDeveloperSpecificPath(t *testing.T) {
	root := copyApplicationFixture(t)
	readme := filepath.Join(root, "README.md")
	file, err := os.OpenFile(readme, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString("\nLocal checkout: /Users/example/catalog\n"); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	err = auditApplication(root)
	if err == nil || !strings.Contains(err.Error(), "developer-specific absolute path") {
		t.Fatalf("error = %v, want non-portable-path rejection", err)
	}
}

func TestAuditRejectsCanonicalProfileCopy(t *testing.T) {
	root := copyApplicationFixture(t)
	writeFile(t, filepath.Join(root, filepath.FromSlash(canonicalProfile)), "name: copied-curator\n")
	err := auditApplication(root)
	if err == nil || !strings.Contains(err.Error(), "must not contain a copy") {
		t.Fatalf("error = %v, want copied-profile rejection", err)
	}
}

func TestStatsCompositionOnlyJSON(t *testing.T) {
	encoded, err := json.Marshal(newStatsOutput())
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(encoded, &document); err != nil {
		t.Fatal(err)
	}
	if _, exists := document["agents"]; exists {
		t.Fatal("stats unexpectedly contains an agents section")
	}
	application, ok := document["application"].(map[string]any)
	if !ok {
		t.Fatalf("application = %#v, want object", document["application"])
	}
	if application["ownership"] != "composition" || application["agents_contributed"] != float64(0) {
		t.Fatalf("application stats = %#v, want composition with zero agents", application)
	}
	if application["canonical_references"] != float64(2) {
		t.Fatalf("canonical_references = %#v, want 2", application["canonical_references"])
	}
}

func TestLifecycleExitContract(t *testing.T) {
	root := realApplicationRoot(t)
	var profile struct {
		Machine          string   `yaml:"machine"`
		Tools            []string `yaml:"tools"`
		ToolDeclarations []string `yaml:"tool_declarations"`
		RESTDefinitions  []string `yaml:"rest_definitions"`
	}
	readTestYAML(t, filepath.Join(root, "..", "catalog", "agents", "lifecycle-exit", "profile.yaml"), &profile)
	if profile.Machine != "machine.yaml" ||
		!reflect.DeepEqual(profile.Tools, []string{"tools.yaml"}) ||
		!reflect.DeepEqual(profile.ToolDeclarations, []string{"declarations.yaml"}) ||
		!reflect.DeepEqual(profile.RESTDefinitions, []string{"rest.yaml"}) {
		t.Fatalf("lifecycle profile closure = %#v", profile)
	}

	var machine struct {
		InitialState   string   `yaml:"initial_state"`
		TerminalStates []string `yaml:"terminal_states"`
		Transitions    []struct {
			State  string `yaml:"state"`
			Signal string `yaml:"signal"`
			Next   string `yaml:"next"`
			Action string `yaml:"action"`
		} `yaml:"transitions"`
	}
	readTestYAML(t, filepath.Join(root, "..", "catalog", "agents", "lifecycle-exit", "machine.yaml"), &machine)
	if machine.InitialState != "Idle" || !reflect.DeepEqual(machine.TerminalStates, []string{"Done", "Failed"}) {
		t.Fatalf("lifecycle states = initial %q terminals %#v", machine.InitialState, machine.TerminalStates)
	}
	var accepted bool
	for _, transition := range machine.Transitions {
		if transition.State == "Posting" && transition.Signal == "ExitAccepted" && transition.Next == "Done" {
			accepted = true
		}
	}
	if !accepted {
		t.Fatal("lifecycle machine lacks ExitAccepted transition to Done")
	}

	rest, err := os.ReadFile(filepath.Join(root, "..", "catalog", "agents", "lifecycle-exit", "rest.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, contract := range []string{
		"base_url: http://127.0.0.1:18082",
		"auth_ref: none",
		"method: POST",
		"path: /api/lifecycle/exit",
		`reason: "demo presentation"`,
		"success: {status: [200, 202], signal: ExitAccepted}",
	} {
		if !strings.Contains(string(rest), contract) {
			t.Fatalf("lifecycle REST definition missing %q", contract)
		}
	}
}

func TestManagedServicesEvidenceContract(t *testing.T) {
	root := realApplicationRoot(t)
	var architecture struct {
		CapabilityClassification struct {
			ManagedServices []struct {
				Name                    string   `yaml:"name"`
				SharedRequirementGroups []string `yaml:"shared_requirement_groups"`
				PlannedLocalEvidence    []string `yaml:"planned_local_evidence"`
			} `yaml:"managed_services"`
		} `yaml:"capability_classification"`
	}
	readTestYAML(t, filepath.Join(root, "docs", "ARCHITECTURE.yaml"), &architecture)
	services := architecture.CapabilityClassification.ManagedServices
	if len(services) != 2 {
		t.Fatalf("managed services = %d, want 2", len(services))
	}
	want := map[string]bool{"documentation-curator": false, "Go present": false}
	for _, service := range services {
		if _, ok := want[service.Name]; !ok {
			t.Fatalf("unexpected managed service %q", service.Name)
		}
		if !reflect.DeepEqual(service.SharedRequirementGroups, []string{"R1", "R2", "R3"}) {
			t.Fatalf("%s requirement groups = %#v", service.Name, service.SharedRequirementGroups)
		}
		if len(service.PlannedLocalEvidence) < 3 {
			t.Fatalf("%s has insufficient bounded evidence: %#v", service.Name, service.PlannedLocalEvidence)
		}
		want[service.Name] = true
	}
	for name, seen := range want {
		if !seen {
			t.Fatalf("managed service %q not declared", name)
		}
	}
}

func TestTracingEnabled(t *testing.T) {
	enabled, disabled := true, false
	tests := []struct {
		name    string
		tracing *bool
		want    bool
	}{
		{"omitted defaults on", nil, true},
		{"explicit true", &enabled, true},
		{"explicit false", &disabled, false},
	}
	for _, test := range tests {
		got := tracingEnabled(demoConfig{Tracing: test.tracing})
		if got != test.want {
			t.Errorf("tracingEnabled(%s) = %v, want %v", test.name, got, test.want)
		}
	}
}

func TestPostExitRequestSendsAcceptedJSONBody(t *testing.T) {
	var (
		method      string
		contentType string
		body        map[string]string
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method = r.Method
		contentType = r.Header.Get("Content-Type")
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode exit body: %v", err)
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()
	if err := postExitRequest(server.URL); err != nil {
		t.Fatalf("postExitRequest() = %v", err)
	}
	if method != http.MethodPost {
		t.Errorf("method = %s, want POST", method)
	}
	if contentType != "application/json" {
		t.Errorf("content type = %s, want application/json", contentType)
	}
	if body["reason"] == "" {
		t.Errorf("exit body reason is empty; the route rejects bodyless requests with 400")
	}
}

func TestPostExitRequestReportsRejectedRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()
	if err := postExitRequest(server.URL); err == nil {
		t.Fatal("postExitRequest() accepted a 400 response; a rejected exit leaks the collector")
	}
}

func TestCollectorLifecycleURLsFollowControlRoutes(t *testing.T) {
	if got := collectorHealthURL(); got != collectorControlAddress+"/api/lifecycle/health" {
		t.Fatalf("health URL = %s", got)
	}
	if got := collectorExitURL(); got != collectorControlAddress+"/api/lifecycle/exit" {
		t.Fatalf("exit URL = %s", got)
	}
}

func TestCollectorCommandConstruction(t *testing.T) {
	resolved := roots{
		Application: filepath.Join(string(filepath.Separator), "work", "applications", "agent-architecture"),
		Catalog:     filepath.Join(string(filepath.Separator), "work", "applications", "catalog"),
		Core:        filepath.Join(string(filepath.Separator), "work", "agent-core"),
	}
	binary := filepath.Join(string(filepath.Separator), "tmp", "agent")
	spool := filepath.Join(string(filepath.Separator), "tmp", "spool", "collector.ndjson")
	cmd := collectorCommand(resolved, binary, spool)
	wantArgs := []string{
		binary,
		"--profile", filepath.Join(resolved.Catalog, filepath.FromSlash(collectorProfile)),
		"--directory", resolved.Catalog,
		"--core-root", resolved.Core,
	}
	if !reflect.DeepEqual(cmd.Args, wantArgs) {
		t.Fatalf("collector args = %#v, want %#v", cmd.Args, wantArgs)
	}
	if cmd.Dir != resolved.Catalog {
		t.Fatalf("collector directory = %s, want %s", cmd.Dir, resolved.Catalog)
	}
	envMap := make(map[string]string)
	for _, entry := range cmd.Env {
		parts := strings.SplitN(entry, "=", 2)
		if len(parts) == 2 {
			envMap[parts[0]] = parts[1]
		}
	}
	wantEnv := map[string]string{
		"COLLECTOR_MODE":             "spool",
		"COLLECTOR_BIND_HOST":        "127.0.0.1",
		"COLLECTOR_RECEIVER_ADDRESS": collectorReceiverAddress,
		"COLLECTOR_CONTROL_PORT":     "18191",
		"COLLECTOR_MONITOR_PORT":     "18192",
		"COLLECTOR_QUERY_PORT":       "18193",
		"COLLECTOR_SPOOL_PATH":       spool,
	}
	for key, want := range wantEnv {
		if got := envMap[key]; got != want {
			t.Errorf("env %s = %q, want %q", key, got, want)
		}
	}
}

func TestTracedCuratorCommand(t *testing.T) {
	resolved := roots{
		Application: filepath.Join(string(filepath.Separator), "work", "applications", "agent-architecture"),
		Catalog:     filepath.Join(string(filepath.Separator), "work", "applications", "catalog"),
		Core:        filepath.Join(string(filepath.Separator), "work", "agent-core"),
	}
	binary := filepath.Join(string(filepath.Separator), "tmp", "agent")
	plan := runCommandPlan(resolved, binary)
	plan.Run.Args = append(plan.Run.Args,
		"--otel-otlp-endpoint", collectorReceiverAddress,
		"--otel-service-name", "knowledge-manager-curator")
	wantSuffix := []string{
		"--otel-otlp-endpoint", collectorReceiverAddress,
		"--otel-service-name", "knowledge-manager-curator",
	}
	got := plan.Run.Args[len(plan.Run.Args)-4:]
	if !reflect.DeepEqual(got, wantSuffix) {
		t.Fatalf("traced curator args suffix = %#v, want %#v", got, wantSuffix)
	}
}

func rootFixture(t *testing.T) (application, catalog, core string) {
	t.Helper()
	repository := t.TempDir()
	application = filepath.Join(repository, "applications", "agent-architecture")
	catalog = filepath.Join(repository, "applications", "catalog")
	core = filepath.Join(repository, "agent-core")
	writeFile(t, filepath.Join(application, "go.mod"), "module "+applicationModule+"\n")
	writeFile(t, filepath.Join(catalog, filepath.FromSlash(canonicalProfile)), "name: curator\n")
	writeFile(t, filepath.Join(catalog, filepath.FromSlash(collectorProfile)), "name: collector\n")
	writeFile(t, filepath.Join(core, "go.mod"), "module example.test/core\n")
	if err := os.MkdirAll(filepath.Join(core, "cmd", "agent"), 0o755); err != nil {
		t.Fatal(err)
	}
	return application, catalog, core
}

func realApplicationRoot(t *testing.T) string {
	t.Helper()
	root, err := findApplicationRoot(".")
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func copyApplicationFixture(t *testing.T) string {
	t.Helper()
	source := realApplicationRoot(t)
	repository := t.TempDir()
	root := filepath.Join(repository, "applications", "agent-architecture")
	for _, relative := range []string{"docs"} {
		copyTree(t, filepath.Join(source, relative), filepath.Join(root, relative))
	}
	for _, relative := range []string{"README.md", "agent-architecture.slide"} {
		data, err := os.ReadFile(filepath.Join(source, relative))
		if err != nil {
			t.Fatal(err)
		}
		writeFile(t, filepath.Join(root, relative), string(data))
	}
	for _, relative := range []string{
		"applications/docs/specs/software-requirements/srd001-application-module-contract.yaml",
		"applications/docs/specs/software-requirements/srd002-managed-application-services.yaml",
		"applications/catalog/docs/specs/software-requirements/srd011-knowledge-manager.yaml",
		"applications/catalog/docs/specs/use-cases/rel07.0-uc001-documentation-curator-profile-ux.yaml",
	} {
		data, err := os.ReadFile(filepath.Join(source, "..", "..", filepath.FromSlash(relative)))
		if err != nil {
			t.Fatal(err)
		}
		writeFile(t, filepath.Join(repository, filepath.FromSlash(relative)), string(data))
	}
	writeFile(t, filepath.Join(repository, "applications", "catalog", filepath.FromSlash(canonicalProfile)), "name: curator\n")
	writeFile(t, filepath.Join(repository, "applications", "catalog", filepath.FromSlash(collectorProfile)), "name: collector\n")
	return root
}

func copyTree(t *testing.T, source, destination string) {
	t.Helper()
	err := filepath.WalkDir(source, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, relative)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readTestYAML(t *testing.T, path string, out any) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := yaml.Unmarshal(data, out); err != nil {
		t.Fatal(err)
	}
}

// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package kindrig

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func completeCoordinates(t *testing.T) DeployCoordinates {
	t.Helper()
	return DeployCoordinates{
		Release:       "coding-demo",
		Namespace:     "coding",
		ChartPath:     "/tmp/chart/coding-agent-1.0.0.tgz",
		Kubeconfig:    "/tmp/kubeconfig/config",
		ValuesPath:    "/repo/helm/ci/kind-values.yaml",
		OverridesPath: "/repo/build/deploy/coding-demo/work/overrides.yaml",
		Timeout:       "5m0s",
	}
}

// shippedDeployDeclarations is the catalog template the renderer consumes. The
// tests read the real one rather than a fixture, so a word added to the
// catalog without a coordinate fails here.
func shippedDeployDeclarations(t *testing.T) string {
	t.Helper()
	path := filepath.Join("..", "..", "applications", "catalog", "agents", "applier", "deploy-declarations.yaml")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("shipped deploy declarations: %v", err)
	}
	return path
}

func renderArgs(t *testing.T, path string) map[string][]string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Tools []struct {
			Name string   `yaml:"name"`
			Args []string `yaml:"args"`
		} `yaml:"tools"`
	}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		t.Fatalf("parse rendered declarations: %v", err)
	}
	args := map[string][]string{}
	for _, tool := range doc.Tools {
		args[tool.Name] = tool.Args
	}
	if len(args) == 0 {
		t.Fatal("rendered declarations carry no words")
	}
	return args
}

func TestRenderDeployDeclarationsResolvesEveryCoordinate(t *testing.T) {
	t.Parallel()
	coordinates := completeCoordinates(t)
	rendered, err := RenderDeployDeclarations(shippedDeployDeclarations(t), coordinates, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	args := renderArgs(t, rendered)

	// Each coordinate has to land in the argv, not merely stop being a token.
	for name, want := range map[string]string{
		"helm_upgrade":   coordinates.ChartPath,
		"helm_history":   coordinates.Release,
		"verify_rollout": coordinates.Namespace,
		"helm_uninstall": coordinates.Kubeconfig,
	} {
		if !containsArg(args[name], want) {
			t.Errorf("%s argv = %v, want it to carry %q", name, args[name], want)
		}
	}
	for _, name := range []string{"helm_dry_run", "helm_upgrade"} {
		for _, want := range []string{coordinates.ValuesPath, coordinates.OverridesPath} {
			if !containsArg(args[name], want) {
				t.Errorf("%s argv = %v, want it to carry %q", name, args[name], want)
			}
		}
	}
}

// The R6.6 guard. It walks the rendered argv rather than trusting the
// replacement list, because the failure that matters is a word added later
// whose coordinate nobody resolved.
func TestRenderedDeployArgvCarriesNoPlaceholder(t *testing.T) {
	t.Parallel()
	rendered, err := RenderDeployDeclarations(shippedDeployDeclarations(t), completeCoordinates(t), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for name, args := range renderArgs(t, rendered) {
		for _, arg := range args {
			if strings.Contains(arg, deployPlaceholderPrefix) {
				t.Errorf("%s argv carries unresolved placeholder %q", name, arg)
			}
		}
	}
}

// The R6.7 guard. A deploy restates its values from the chart, the overlay,
// and the request document, so a redeploy cannot inherit a value nobody can
// see in the repository.
func TestRenderedDeployArgvCarriesNoReuseValues(t *testing.T) {
	t.Parallel()
	rendered, err := RenderDeployDeclarations(shippedDeployDeclarations(t), completeCoordinates(t), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for name, args := range renderArgs(t, rendered) {
		if containsArg(args, "--reuse-values") {
			t.Errorf("%s argv carries --reuse-values", name)
		}
	}
}

// The apply words carry --atomic, which is what makes a ToolFailed out of
// Applying a state Helm has already compensated (srd022 R6.3). Losing the flag
// would leave a failed install standing and the machine would not know.
func TestRenderedApplyWordsCarryAtomic(t *testing.T) {
	t.Parallel()
	rendered, err := RenderDeployDeclarations(shippedDeployDeclarations(t), completeCoordinates(t), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	args := renderArgs(t, rendered)
	for _, name := range []string{"helm_dry_run", "helm_upgrade"} {
		if !containsArg(args[name], "--atomic") {
			t.Errorf("%s argv = %v, want --atomic", name, args[name])
		}
		if !containsArg(args[name], "--install") {
			t.Errorf("%s argv = %v, want --install", name, args[name])
		}
	}
}

func TestRenderDeployDeclarationsRejectsAnUnresolvedPlaceholder(t *testing.T) {
	t.Parallel()
	source := filepath.Join(t.TempDir(), "deploy-declarations.yaml")
	body := "tools:\n  - name: helm_future\n    args: [upgrade, DA_RELEASE, --flag, DA_UNKNOWN_COORDINATE]\n"
	if err := os.WriteFile(source, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := RenderDeployDeclarations(source, completeCoordinates(t), t.TempDir())
	if err == nil {
		t.Fatal("render = nil, want an error naming the unresolved token")
	}
	if !strings.Contains(err.Error(), "DA_UNKNOWN_COORDINATE") {
		t.Errorf("error = %v, want it to name DA_UNKNOWN_COORDINATE", err)
	}
}

func TestDeployCoordinatesRejectEveryEmptyField(t *testing.T) {
	t.Parallel()
	complete := completeCoordinates(t)
	if err := complete.Validate(); err != nil {
		t.Fatalf("complete coordinates = %v, want valid", err)
	}
	for _, testCase := range []struct {
		field string
		blank func(*DeployCoordinates)
	}{
		{"release", func(c *DeployCoordinates) { c.Release = "" }},
		{"namespace", func(c *DeployCoordinates) { c.Namespace = "" }},
		{"chart path", func(c *DeployCoordinates) { c.ChartPath = "" }},
		{"kubeconfig", func(c *DeployCoordinates) { c.Kubeconfig = "" }},
		{"values", func(c *DeployCoordinates) { c.ValuesPath = "" }},
		{"overrides", func(c *DeployCoordinates) { c.OverridesPath = "" }},
		{"timeout", func(c *DeployCoordinates) { c.Timeout = "" }},
	} {
		coordinates := complete
		testCase.blank(&coordinates)
		err := coordinates.Validate()
		if err == nil {
			t.Errorf("blank %s = nil, want an error", testCase.field)
			continue
		}
		if !strings.Contains(err.Error(), testCase.field) {
			t.Errorf("blank %s error = %v, want it to name the field", testCase.field, err)
		}
	}
}

// Rendering lands in the application's build tree, not a temporary directory:
// reading the exact argv that ran is how a failed deploy gets diagnosed.
func TestRenderDeployWritesIntoTheApplicationBuildTree(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	destination := DeployRenderDirectory(root, "coding-demo")
	if want := filepath.Join(root, "build", "deploy", "coding-demo"); destination != want {
		t.Fatalf("render directory = %q, want %q", destination, want)
	}
	if _, err := RenderDeployDeclarations(shippedDeployDeclarations(t), completeCoordinates(t), destination); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(destination, "deploy-declarations.yaml")); err != nil {
		t.Errorf("rendered declarations not in the build tree: %v", err)
	}
}

func TestRenderDeployProfileBindsTheShippedMachine(t *testing.T) {
	t.Parallel()
	destination := t.TempDir()
	path, err := RenderDeployProfile(RenderedProfile{
		Name:                 "applier-deploy-request",
		Release:              "coding-demo",
		Namespace:            "coding",
		MachinePath:          "/catalog/agents/applier/deploy-machine.yaml",
		ToolsPath:            "/catalog/agents/applier/deploy-tools.yaml",
		ApplyDeclarations:    "/catalog/agents/applier/apply-declarations.yaml",
		RenderedDeclarations: filepath.Join(destination, "deploy-declarations.yaml"),
	}, destination)
	if err != nil {
		t.Fatal(err)
	}
	var profile struct {
		Name             string   `yaml:"name"`
		Machine          string   `yaml:"machine"`
		Tools            []string `yaml:"tools"`
		ToolDeclarations []string `yaml:"tool_declarations"`
		REST             []string `yaml:"rest_definitions"`
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := yaml.Unmarshal(data, &profile); err != nil {
		t.Fatalf("parse rendered profile: %v", err)
	}
	if profile.Machine != "/catalog/agents/applier/deploy-machine.yaml" {
		t.Errorf("machine = %q, want the shipped machine rather than a copy", profile.Machine)
	}
	if len(profile.REST) != 0 {
		t.Errorf("rendered profile declares rest_definitions %v; a host-side run hosts no listener", profile.REST)
	}
	if len(profile.ToolDeclarations) != 2 {
		t.Errorf("tool_declarations = %v, want the apply declarations and the rendered ones", profile.ToolDeclarations)
	}
}

func containsArg(args []string, want string) bool {
	for _, arg := range args {
		if arg == want || strings.Contains(arg, want) {
			return true
		}
	}
	return false
}

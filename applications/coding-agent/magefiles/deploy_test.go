// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"

	"github.com/Nokia-Bell-Labs/declarative-agents/magefiles/kindrig"
	"gopkg.in/yaml.v3"
)

// The harness rejects an empty coordinate, so every field this application
// owns has to be populated before the run starts. Kubeconfig and OverridesPath
// are the harness's to fill.
func TestCodingDeployCoordinatesResolveCompletely(t *testing.T) {
	t.Parallel()
	coordinates := codingDeployCoordinates(
		integrationRoots{Application: "/repo/applications/coding-agent"},
		"/repo/applications/coding-agent/build/deploy/demo/chart/coding-agent-1.0.0.tgz")
	for name, value := range map[string]string{
		"release":   coordinates.Release,
		"namespace": coordinates.Namespace,
		"chart":     coordinates.ChartPath,
		"values":    coordinates.ValuesPath,
		"timeout":   coordinates.Timeout,
	} {
		if strings.TrimSpace(value) == "" {
			t.Errorf("coordinate %s is empty", name)
		}
	}
	if coordinates.Release != codingDemoRelease {
		t.Errorf("release = %q, want %q", coordinates.Release, codingDemoRelease)
	}
	if !strings.HasSuffix(coordinates.ValuesPath, "helm/ci/kind-values.yaml") {
		t.Errorf("values = %q, want the checked-in kind overlay", coordinates.ValuesPath)
	}
}

// A tag is a commit-shaped string that YAML reads as a number when it is bare:
// an unquoted 20260919 decodes as an integer and the image reference stops
// resolving. The imperative path this replaced used --set-string for the same
// reason, and losing the quoting is the one silent way this migration breaks.
func TestCodingDeployOverridesKeepImageTagsAsStrings(t *testing.T) {
	t.Parallel()
	overrides := codingDeployOverrides(codingHelmImages{Agent: "coding-agent:20260919"})

	var decoded struct {
		Image struct {
			Repository any `yaml:"repository"`
			Tag        any `yaml:"tag"`
		} `yaml:"image"`
	}
	if err := yaml.Unmarshal([]byte(overrides), &decoded); err != nil {
		t.Fatalf("overrides do not decode as YAML: %v\n%s", err, overrides)
	}
	if _, ok := decoded.Image.Tag.(string); !ok {
		t.Errorf("image.tag decoded as %T (%v), want a string", decoded.Image.Tag, decoded.Image.Tag)
	}
	if decoded.Image.Tag != "20260919" {
		t.Errorf("image.tag = %v, want the numeric-looking tag preserved", decoded.Image.Tag)
	}
	if repository, _ := decoded.Image.Repository.(string); repository != "coding-agent" {
		t.Errorf("image.repository = %v, want coding-agent", decoded.Image.Repository)
	}
}

// An undeploy removes a release and reads no chart, so the teardown path must
// not run the chart packaging the deploy builder does. Reusing the deploy
// builder is what made every mage undeploy produce an artifact it ignored, and
// next door in agent-architecture the same shape also provisioned ConfigMaps,
// so a teardown created objects on its way to deleting a release (GH-2350).
//
// This reads the source rather than running the builders. Running them needs a
// resolvable catalog and an agent binary, and a test that skips when it cannot
// build one guards nothing; what has to hold is that the teardown path never
// reaches the packaging, and that is a fact about the call graph.
func TestCodingUndeployNeverReachesChartPackaging(t *testing.T) {
	t.Parallel()
	for caller, forbidden := range map[string][]string{
		"Undeploy":              {"codingDeployRequest", "codingDeployChart", "packageHelmChart"},
		"codingUndeployRequest": {"codingDeployRequest", "codingDeployChart", "packageHelmChart"},
	} {
		called := callsWithin(t, "deploy.go", caller)
		for _, name := range forbidden {
			if called[name] {
				t.Errorf("%s calls %s; an undeploy removes a release and reads no chart", caller, name)
			}
		}
	}
	// The teardown still has to reach its own builder, or the check above
	// would pass on an Undeploy that does nothing at all.
	if called := callsWithin(t, "deploy.go", "Undeploy"); !called["codingUndeployRequest"] {
		t.Error("Undeploy does not call codingUndeployRequest")
	}
}

// The undeploy coordinates come from the shared kindrig helper, so the three
// applications agree on what an unread coordinate renders to rather than each
// repeating the literals.
func TestCodingUndeployCoordinatesComeFromTheSharedHelper(t *testing.T) {
	t.Parallel()
	coordinates := kindrig.UndeployCoordinates(
		codingDemoRelease, codingHelmNamespace, codingHelmInstallTimeout.String())
	if coordinates.Release != codingDemoRelease {
		t.Errorf("release = %q, want %q", coordinates.Release, codingDemoRelease)
	}
	if coordinates.Namespace != codingHelmNamespace {
		t.Errorf("namespace = %q, want %q", coordinates.Namespace, codingHelmNamespace)
	}
	if coordinates.Timeout != codingHelmInstallTimeout.String() {
		t.Errorf("timeout = %q, want %q", coordinates.Timeout, codingHelmInstallTimeout.String())
	}
	if !called(t, "deploy.go", "codingUndeployRequest", "UndeployCoordinates") {
		t.Error("codingUndeployRequest does not use kindrig.UndeployCoordinates")
	}
}

// callsWithin reports every function name called in the body of the named
// function, selector calls by their final name so kindrig.Undeploy reads as
// Undeploy.
func callsWithin(t *testing.T, file, function string) map[string]bool {
	t.Helper()
	parsed, err := parser.ParseFile(token.NewFileSet(), file, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", file, err)
	}
	for _, declaration := range parsed.Decls {
		declared, ok := declaration.(*ast.FuncDecl)
		if !ok || declared.Name.Name != function {
			continue
		}
		names := map[string]bool{}
		ast.Inspect(declared, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			switch function := call.Fun.(type) {
			case *ast.Ident:
				names[function.Name] = true
			case *ast.SelectorExpr:
				names[function.Sel.Name] = true
			}
			return true
		})
		return names
	}
	t.Fatalf("%s declares no function %s", file, function)
	return nil
}

func called(t *testing.T, file, function, target string) bool {
	t.Helper()
	return callsWithin(t, file, function)[target]
}

// The deploy and undeploy verbs bind the catalog's own profile variants rather
// than a copy, so a machine change in the catalog reaches this application.
func TestCodingDeployProfilesAreCatalogRelative(t *testing.T) {
	t.Parallel()
	for name, rel := range map[string]string{
		"deploy":   applierDeployProfileRel,
		"undeploy": applierUndeployProfileRel,
	} {
		if !strings.HasPrefix(rel, "agents/applier/") {
			t.Errorf("%s profile %q is not catalog-relative", name, rel)
		}
		if !strings.HasSuffix(rel, "-profile.yaml") {
			t.Errorf("%s profile %q does not name a profile variant", name, rel)
		}
	}
}

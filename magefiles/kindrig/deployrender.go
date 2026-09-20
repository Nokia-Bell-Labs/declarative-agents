// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package kindrig

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Exec args are fully static: ExecBuilder.Build maps parameters only from the
// previous result by name, and the runtime's vars map carries directory and
// request alone, consumed to root builtin filesystem and OTLP tools rather
// than substituted into argv. So an application's chart, release, namespace,
// and kubeconfig cannot arrive as flags on the agent.
//
// In-cluster that is solved at chart-render time, where profiles-configmap.yaml
// rewrites the applier's placeholder coordinates to the installed release.
// Host-side there is no chart render, so the deploy target renders the
// declarations itself before it runs the agent (srd022 R6.6).
const (
	deployPlaceholderPrefix = "DA_"
	overridesFileName       = "overrides.yaml"
)

// deployPlaceholderPattern matches any token the renderer is responsible for.
// The check after substitution scans for it rather than trusting the
// replacement list, because the failure that matters is a word added later
// whose coordinate nobody remembered to resolve (the in-cluster equivalent
// cost GH-217 a leaked release name).
var deployPlaceholderPattern = regexp.MustCompile(`DA_[A-Z_]+`)

// DeployCoordinates is everything one application's deploy words need
// resolved. Every field is required: a deploy that guesses a namespace or
// falls back to an ambient kubeconfig is a deploy against the wrong cluster.
type DeployCoordinates struct {
	Release   string
	Namespace string
	ChartPath string
	// Kubeconfig is a file path, never an environment variable. kindrig's
	// Kubeconfig writes a private one per cluster.
	Kubeconfig string
	// ValuesPath is the application's checked-in overlay.
	ValuesPath string
	// OverridesPath is where write_overrides puts the decided values, which the
	// apply words read with -f. It is the workspace path, resolved, because
	// there is no /work on a developer's machine.
	OverridesPath string
	// Timeout bounds both the apply wait and the rollout wait.
	Timeout string
}

// Validate reports every empty field at once, so a caller fixes its wiring in
// one pass instead of one field per run.
func (c DeployCoordinates) Validate() error {
	var missing []string
	for name, value := range map[string]string{
		"release":    c.Release,
		"namespace":  c.Namespace,
		"chart path": c.ChartPath,
		"kubeconfig": c.Kubeconfig,
		"values":     c.ValuesPath,
		"overrides":  c.OverridesPath,
		"timeout":    c.Timeout,
	} {
		if strings.TrimSpace(value) == "" {
			missing = append(missing, name)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	sort.Strings(missing)
	return fmt.Errorf("deploy coordinates: %s not resolved", strings.Join(missing, ", "))
}

// substitutions maps each placeholder to its resolved value. Longer tokens are
// applied before shorter ones by the replacement order below, so a token that
// contains another cannot be partly rewritten. None of the current seven
// collide; the ordering is kept so an eighth cannot introduce the bug quietly.
func (c DeployCoordinates) substitutions() []struct{ token, value string } {
	pairs := []struct{ token, value string }{
		{"DA_KUBECONFIG", c.Kubeconfig},
		{"DA_NAMESPACE", c.Namespace},
		{"DA_OVERRIDES", c.OverridesPath},
		{"DA_RELEASE", c.Release},
		{"DA_TIMEOUT", c.Timeout},
		{"DA_VALUES", c.ValuesPath},
		{"DA_CHART", c.ChartPath},
	}
	sort.SliceStable(pairs, func(i, j int) bool {
		return len(pairs[i].token) > len(pairs[j].token)
	})
	return pairs
}

// RenderDeployDeclarations resolves the placeholder deploy declarations into
// destination and returns the rendered declarations path.
//
// It renders into the application's build tree rather than a temporary
// directory: a developer reading a failed deploy needs the exact argv that ran.
func RenderDeployDeclarations(source string, coordinates DeployCoordinates, destination string) (string, error) {
	if err := coordinates.Validate(); err != nil {
		return "", err
	}
	template, err := os.ReadFile(source)
	if err != nil {
		return "", fmt.Errorf("read deploy declarations %s: %w", source, err)
	}
	rendered := string(template)
	for _, pair := range coordinates.substitutions() {
		rendered = strings.ReplaceAll(rendered, pair.token, pair.value)
	}
	if survivors := deployPlaceholderPattern.FindAllString(rendered, -1); len(survivors) > 0 {
		return "", fmt.Errorf(
			"deploy declarations %s left %s unresolved; every coordinate a word reads must be a field on DeployCoordinates",
			source, strings.Join(uniqueSorted(survivors), ", "))
	}
	if err := os.MkdirAll(destination, 0o755); err != nil {
		return "", fmt.Errorf("create deploy render directory %s: %w", destination, err)
	}
	path := filepath.Join(destination, "deploy-declarations.yaml")
	if err := os.WriteFile(path, []byte(rendered), 0o644); err != nil {
		return "", fmt.Errorf("write rendered deploy declarations %s: %w", path, err)
	}
	return path, nil
}

// RenderDeployProfile writes a profile that binds the catalog's machine and
// selection to the rendered declarations. The machine and the selection are
// referenced where they are shipped, so a deploy runs the canonical machine
// and never a copy that could drift from it.
func RenderDeployProfile(profile RenderedProfile, destination string) (string, error) {
	if err := os.MkdirAll(destination, 0o755); err != nil {
		return "", fmt.Errorf("create deploy render directory %s: %w", destination, err)
	}
	body := fmt.Sprintf(`# Copyright (c) 2026 Nokia
# SPDX-License-Identifier: BSD-3-Clause
# Rendered by kindrig for release %s in namespace %s. Regenerated on every
# deploy; edit the catalog declarations, not this file.
name: %s
machine: %s
tools:
  - %s
tool_declarations:
  - %s
  - %s
`,
		profile.Release, profile.Namespace, profile.Name,
		profile.MachinePath, profile.ToolsPath,
		profile.ApplyDeclarations, profile.RenderedDeclarations)
	path := filepath.Join(destination, profile.Name+".yaml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		return "", fmt.Errorf("write rendered deploy profile %s: %w", path, err)
	}
	return path, nil
}

// RenderedProfile names the shipped pieces a rendered profile binds together.
type RenderedProfile struct {
	Name                 string
	Release              string
	Namespace            string
	MachinePath          string
	ToolsPath            string
	ApplyDeclarations    string
	RenderedDeclarations string
}

// DeployWorkspace is the directory the machine writes its values document into
// and the apply words read it back from.
//
// It is exported because a caller may need the overrides file before the run
// starts: chatbot-mesh measures its projected release Secret against the
// rendered values before anything reaches the cluster, and the machine writes
// that file as its first transition, which is too late (GH-1475, GH-2345).
// One owner for the path keeps that caller from repeating the literal.
func DeployWorkspace(applicationRoot, release string) string {
	return filepath.Join(DeployRenderDirectory(applicationRoot, release), "work")
}

// DeployOverridesPath is the values document inside that workspace.
func DeployOverridesPath(applicationRoot, release string) string {
	return filepath.Join(DeployWorkspace(applicationRoot, release), overridesFileName)
}

// DeployRenderDirectory is where one release's rendered declarations land.
func DeployRenderDirectory(applicationRoot, release string) string {
	return filepath.Join(applicationRoot, "build", "deploy", release)
}

func uniqueSorted(values []string) []string {
	seen := map[string]bool{}
	var unique []string
	for _, value := range values {
		if seen[value] {
			continue
		}
		seen[value] = true
		unique = append(unique, value)
	}
	sort.Strings(unique)
	return unique
}

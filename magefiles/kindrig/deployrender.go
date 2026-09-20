// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package kindrig

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
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
// The walk scans every scalar for it rather than trusting the replacement
// list, because the failure that matters is a word added later whose
// coordinate nobody remembered to resolve (the in-cluster equivalent cost
// GH-217 a leaked release name).
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
//
// It also rejects a NUL byte, which is the one class of value that cannot
// reach a child process at all: exec argv strings are NUL-terminated, so the
// kernel would silently truncate the argument and the word would run against
// a coordinate nobody wrote. Everything else a caller can produce is the
// renderer's problem rather than the caller's, because substitution goes
// through YAML nodes and quotes what needs quoting.
func (c DeployCoordinates) Validate() error {
	fields := map[string]string{
		"release":    c.Release,
		"namespace":  c.Namespace,
		"chart path": c.ChartPath,
		"kubeconfig": c.Kubeconfig,
		"values":     c.ValuesPath,
		"overrides":  c.OverridesPath,
		"timeout":    c.Timeout,
	}
	var missing, unusable []string
	for name, value := range fields {
		switch {
		case strings.TrimSpace(value) == "":
			missing = append(missing, name)
		case strings.ContainsRune(value, 0):
			unusable = append(unusable, name)
		}
	}
	var problems []string
	if len(missing) > 0 {
		sort.Strings(missing)
		problems = append(problems, fmt.Sprintf("%s not resolved", strings.Join(missing, ", ")))
	}
	if len(unusable) > 0 {
		sort.Strings(unusable)
		problems = append(problems, fmt.Sprintf("%s carry a NUL byte and cannot reach an argv", strings.Join(unusable, ", ")))
	}
	if len(problems) == 0 {
		return nil
	}
	return fmt.Errorf("deploy coordinates: %s", strings.Join(problems, "; "))
}

// byToken maps each placeholder to its resolved value. Substitution replaces
// whole YAML scalars rather than text, so a token that contains another cannot
// be partly rewritten and the map needs no ordering.
func (c DeployCoordinates) byToken() map[string]string {
	return map[string]string{
		"DA_KUBECONFIG": c.Kubeconfig,
		"DA_NAMESPACE":  c.Namespace,
		"DA_OVERRIDES":  c.OverridesPath,
		"DA_RELEASE":    c.Release,
		"DA_TIMEOUT":    c.Timeout,
		"DA_VALUES":     c.ValuesPath,
		"DA_CHART":      c.ChartPath,
	}
}

// RenderDeployDeclarations resolves the placeholder deploy declarations into
// destination and returns the rendered declarations path.
//
// It renders into the application's build tree rather than a temporary
// directory: a developer reading a failed deploy needs the exact argv that ran.
//
// Substitution replaces whole YAML scalar nodes rather than text. Text
// replacement put every caller one metacharacter away from a render the
// runtime cannot parse, and the placeholder scan could not catch it because
// the tokens really were gone: a chart coordinate reading
// "(no chart: undeploy removes a release)" produced a declarations file that
// failed to load (GH-2349). A node carries a value rather than a spelling, so
// a colon, a leading @ or #, or a word a schema would read as a boolean
// survives as the string it was.
func RenderDeployDeclarations(source string, coordinates DeployCoordinates, destination string) (string, error) {
	if err := coordinates.Validate(); err != nil {
		return "", err
	}
	template, err := os.ReadFile(source)
	if err != nil {
		return "", fmt.Errorf("read deploy declarations %s: %w", source, err)
	}
	var document yaml.Node
	if err := yaml.Unmarshal(template, &document); err != nil {
		return "", fmt.Errorf("parse deploy declarations %s: %w", source, err)
	}
	resolution := resolveDeployPlaceholders(&document, coordinates.byToken())
	if len(resolution.embedded) > 0 {
		return "", fmt.Errorf(
			"deploy declarations %s embed a coordinate in a larger value (%s); a coordinate has to be an argument on its own, because the renderer substitutes YAML nodes rather than text",
			source, strings.Join(uniqueSorted(resolution.embedded), ", "))
	}
	if len(resolution.unresolved) > 0 {
		return "", fmt.Errorf(
			"deploy declarations %s left %s unresolved; every coordinate a word reads must be a field on DeployCoordinates",
			source, strings.Join(uniqueSorted(resolution.unresolved), ", "))
	}
	rendered, err := encodeDeployDocument(&document)
	if err != nil {
		return "", fmt.Errorf("encode rendered deploy declarations from %s: %w", source, err)
	}
	if err := os.MkdirAll(destination, 0o755); err != nil {
		return "", fmt.Errorf("create deploy render directory %s: %w", destination, err)
	}
	path := filepath.Join(destination, "deploy-declarations.yaml")
	if err := os.WriteFile(path, rendered, 0o644); err != nil {
		return "", fmt.Errorf("write rendered deploy declarations %s: %w", path, err)
	}
	return path, nil
}

// deployResolution is what one walk of the template found: the tokens no
// coordinate answers, and the scalars that carry a token inside a longer
// value. Both are collected rather than returned on the first hit, so a
// caller fixes its template in one pass.
type deployResolution struct {
	unresolved []string
	embedded   []string
}

// resolveDeployPlaceholders rewrites every scalar that is exactly a
// placeholder token, and reports the ones it could not.
//
// A rewritten scalar is emitted double-quoted. Quoting is not always required
// — most coordinates are ordinary paths — but forcing it does two things a
// conditional would not. It makes the rendered argv legible, because the
// quoted arguments are exactly the ones the renderer decided and the bare
// ones are the flags the catalog wrote. And it settles the value's type here
// rather than in whichever schema version reads the file next, so a namespace
// spelled "yes" or a timeout spelled "1.0" cannot arrive as a bool or a float.
func resolveDeployPlaceholders(node *yaml.Node, resolved map[string]string) deployResolution {
	var resolution deployResolution
	var walk func(*yaml.Node)
	walk = func(current *yaml.Node) {
		if current == nil {
			return
		}
		if current.Kind == yaml.ScalarNode {
			tokens := deployPlaceholderPattern.FindAllString(current.Value, -1)
			switch {
			case len(tokens) == 0:
			case len(tokens) == 1 && tokens[0] == current.Value:
				value, known := resolved[current.Value]
				if !known {
					resolution.unresolved = append(resolution.unresolved, current.Value)
					break
				}
				current.Value = value
				current.Tag = "!!str"
				current.Style = yaml.DoubleQuotedStyle
			default:
				resolution.embedded = append(resolution.embedded, current.Value)
			}
		}
		for _, child := range current.Content {
			walk(child)
		}
	}
	walk(node)
	return resolution
}

// encodeDeployDocument writes the resolved document back out at the
// repository's two-space indent.
//
// The round trip keeps the template's comments, which is why the rendered
// file still explains what it is and where it came from. It also leaves the
// DA_ tokens those comments name intact: they are prose about the template,
// and the text substitution this replaced used to rewrite them into a
// sentence claiming an unrendered word would address a release by its
// resolved name.
func encodeDeployDocument(document *yaml.Node) ([]byte, error) {
	var buffer bytes.Buffer
	encoder := yaml.NewEncoder(&buffer)
	encoder.SetIndent(2)
	if err := encoder.Encode(document); err != nil {
		return nil, err
	}
	if err := encoder.Close(); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
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

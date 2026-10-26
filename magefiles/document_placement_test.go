// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

const documentTypeRegistryPath = "agent-core/docs/constitutions/design.yaml"

type documentTypeRegistry struct {
	DocumentTypes map[string]documentType `yaml:"document_types"`
}

type documentType struct {
	Location       string   `yaml:"location"`
	Format         string   `yaml:"format"`
	RequiredFields []string `yaml:"required_fields"`
	// Markdown is the type's Markdown declaration (GH-2578): a mapping when
	// the type's location holds Markdown documents of a declared structure.
	// A scalar (a note that the type stays in its current format) declares
	// none.
	Markdown yaml.Node `yaml:"markdown"`
}

// declared reports a type whose Markdown structure is declared.
func (t documentType) declared() bool {
	return t.Markdown.Kind == yaml.MappingNode
}

// excluded lists the documents the type's declaration keeps in their own
// format, such as docs/constitutions/design.yaml: each is placed as the type
// at exactly that path, with the type's required fields.
func (t documentType) excluded() []string {
	if !t.declared() {
		return nil
	}
	var declaration struct {
		Exclude []string `yaml:"exclude"`
	}
	if err := t.Markdown.Decode(&declaration); err != nil {
		return nil
	}
	return declaration.Exclude
}

type documentLocationMatcher struct {
	documentType string
	location     string
	format       string
	required     []string
	pattern      *regexp.Regexp
	// declared marks a location whose type declares its Markdown structure:
	// the document must parse under that declaration, whose diagnostics own
	// its required fields.
	declared bool
}

func TestDocumentPlacementRepositoryConformance(t *testing.T) {
	if err := checkDocumentPlacement(repositoryRoot(t)); err != nil {
		t.Fatal(err)
	}
}

func TestDocumentPlacementReportsEveryTrackedStray(t *testing.T) {
	root := newDocumentPlacementRepository(t, `
document_types:
  constitution:
    location: docs/constitutions/name.yaml
  guide:
    location: docs/guides/name.md
`, map[string]string{
		"module/docs/guides/operator.md": "guide",
		"module/docs/stray.md":           "stray",
		"other/docs/unknown.yaml":        "unknown",
	})

	err := checkDocumentPlacement(root)
	if err == nil {
		t.Fatal("placement check passed with tracked stray documents")
	}
	for _, want := range []string{
		"module/docs/stray.md: matches no declared document type location",
		"other/docs/unknown.yaml: matches no declared document type location",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error missing %q:\n%v", want, err)
		}
	}
}

func TestDocumentPlacementLoadsNewTypeWithoutCodeChange(t *testing.T) {
	root := newDocumentPlacementRepository(t, `
document_types:
  constitution:
    location: docs/constitutions/name.yaml
`, map[string]string{
		"module/docs/runbooks/recovery.yaml": "steps: []",
	})
	if err := checkDocumentPlacement(root); err == nil ||
		!strings.Contains(err.Error(), "module/docs/runbooks/recovery.yaml") {
		t.Fatalf("initial placement error = %v, want undeclared runbook", err)
	}

	writeDocumentPlacementFile(t, root, documentTypeRegistryPath, `
document_types:
  constitution:
    location: docs/constitutions/name.yaml
  runbook:
    location: docs/runbooks/name.yaml
`)
	if err := checkDocumentPlacement(root); err != nil {
		t.Fatalf("placement after registry-only change: %v", err)
	}
}

// validGuidelineMarkdown is a minimal engineering guideline in the Markdown
// format, which parses under the embedded declarations.
const validGuidelineMarkdown = `---
id: eng10-markdown-spec-format
title: Sample
---

<!-- field: introduction -->
## Introduction

We describe a sample practice.

<!-- section: the-practice -->
## The practice

Each module follows the practice.

<!-- field: references -->
## References

- https://www.rfc-editor.org/rfc/rfc2119
`

// TestDocumentPlacementAcceptsDeclaredMarkdownLocations proves a type's
// declared Markdown location places a Markdown document, and that an
// undeclared Markdown location still fails (GH-2597).
func TestDocumentPlacementAcceptsDeclaredMarkdownLocations(t *testing.T) {
	const registry = `
document_types:
  engineering_guideline:
    location: docs/engineering/engNN-short-name.md
    converts_from: docs/engineering/engNN-short-name.yaml
    markdown:
      frontmatter: {required: [id, title]}
  constitution:
    location: docs/constitutions/name.yaml
    markdown: stays YAML
`
	declared := newDocumentPlacementRepository(t, registry, map[string]string{
		"docs/engineering/eng10-markdown-spec-format.md": validGuidelineMarkdown,
	})
	if err := checkDocumentPlacement(declared); err != nil {
		t.Fatalf("declared markdown location rejected: %v", err)
	}
	// A declared Markdown document must parse under its declaration; one
	// that does not is a placement failure naming its first finding.
	broken := newDocumentPlacementRepository(t, registry, map[string]string{
		"docs/engineering/eng10-markdown-spec-format.md": strings.Replace(validGuidelineMarkdown, "<!-- field: introduction -->\n", "", 1),
	})
	if err := checkDocumentPlacement(broken); err == nil || !strings.Contains(err.Error(), "heading-untagged") {
		t.Fatalf("unparseable markdown error = %v, want heading-untagged", err)
	}
	undeclared := newDocumentPlacementRepository(t, registry, map[string]string{
		"docs/constitutions/design.md": "# not declared\n",
	})
	if err := checkDocumentPlacement(undeclared); err == nil ||
		!strings.Contains(err.Error(), "docs/constitutions/design.md") {
		t.Fatalf("undeclared markdown location error = %v, want docs/constitutions/design.md", err)
	}
}

// TestDocumentPlacementRejectsYAMLAtACorpusLocation proves the sunset
// (GH-2626 AC2): a YAML file at a corpus document's former location matches
// no declared location, so the corpus cannot regrow a second format.
func TestDocumentPlacementRejectsYAMLAtACorpusLocation(t *testing.T) {
	root := newDocumentPlacementRepository(t, `
document_types:
  engineering_guideline:
    location: docs/engineering/engNN-short-name.md
    converts_from: docs/engineering/engNN-short-name.yaml
    markdown:
      frontmatter: {required: [id, title]}
`, map[string]string{
		"docs/engineering/eng10-markdown-spec-format.md": validGuidelineMarkdown,
		"docs/engineering/eng11-stray.yaml":              "id: eng11-stray\ntitle: Stray\n",
	})
	err := checkDocumentPlacement(root)
	if err == nil || !strings.Contains(err.Error(), "docs/engineering/eng11-stray.yaml: matches no declared document type location") {
		t.Fatalf("placement error = %v, want the stray YAML guideline rejected", err)
	}
	if strings.Contains(err.Error(), "eng10-markdown-spec-format.md") {
		t.Fatalf("placement rejected the Markdown guideline: %v", err)
	}
}

// TestDocumentPlacementPlacesExcludedDocuments proves a document a
// declaration keeps in its own format, the design constitution, is placed
// as its type and still owes the type's required fields.
func TestDocumentPlacementPlacesExcludedDocuments(t *testing.T) {
	// The registry is itself agent-core/docs/constitutions/design.yaml in
	// the fixture repository, so it carries the constitution's fields.
	const registry = `
id: design
title: Design
document_types:
  constitution:
    location: docs/constitutions/name.md
    converts_from: docs/constitutions/name.yaml
    required_fields: [id, title]
    markdown:
      exclude: [docs/constitutions/design.yaml]
`
	placed := newDocumentPlacementRepository(t, registry, map[string]string{
		"docs/constitutions/design.yaml": "id: design\ntitle: Design\n",
	})
	if err := checkDocumentPlacement(placed); err != nil {
		t.Fatalf("excluded design constitution rejected: %v", err)
	}
	incomplete := newDocumentPlacementRepository(t, registry, map[string]string{
		"docs/constitutions/design.yaml":  "id: design\n",
		"docs/constitutions/testing.yaml": "id: testing\ntitle: Testing\n",
	})
	err := checkDocumentPlacement(incomplete)
	if err == nil || !strings.Contains(err.Error(), "docs/constitutions/design.yaml: document type constitution missing required field title") ||
		!strings.Contains(err.Error(), "docs/constitutions/testing.yaml: matches no declared document type location") {
		t.Fatalf("placement error = %v, want design.yaml missing title and testing.yaml unplaced", err)
	}
}

func TestDocumentPlacementExcludesNonSourceTrees(t *testing.T) {
	root := newDocumentPlacementRepository(t, `
document_types:
  constitution:
    location: docs/constitutions/name.yaml
`, map[string]string{
		"module/testdata/docs/fixture.yaml":            "fixture",
		"module/node_modules/pkg/docs/readme.md":       "generated",
		"module/vendor/pkg/docs/readme.md":             "vendored",
		"module/build/docs/generated.yaml":             "generated",
		"module/helm/profiles/docs/generated.yaml":     "generated",
		"module/helm/dist/docs/generated.yaml":         "generated",
		"module/ui/dist/docs/generated-reference.yaml": "generated",
		"generated-files/module/docs/generated.yaml":   "generated",
	})
	if err := checkDocumentPlacement(root); err != nil {
		t.Fatal(err)
	}
}

func TestDocumentPlacementCoversDeclaredPlaceholderForms(t *testing.T) {
	tests := map[string]string{
		"docs/engineering/engNN-short-name.yaml":                  "module/docs/engineering/eng01-kind-test.yaml",
		"docs/specs/software-requirements/srdNNN-short-name.yaml": "docs/specs/software-requirements/srd026-program-restore.yaml",
		"docs/specs/use-cases/relNN.N-ucNNN-short-name.yaml":      "app/docs/specs/use-cases/rel14.0-uc003-rollback.yaml",
		"docs/specs/test-suites/test-relNN.N-short-name.yaml":     "docs/specs/test-suites/test-rel14.0-rollback.yaml",
		"docs/migrations/name.yaml":                               "app/docs/migrations/v0.20260819.0-layout.yaml",
	}
	for location, candidate := range tests {
		t.Run(location, func(t *testing.T) {
			matcher, err := compileDocumentLocation(location)
			if err != nil {
				t.Fatal(err)
			}
			if !matcher.MatchString(candidate) {
				t.Fatalf("%q does not match %q", candidate, location)
			}
		})
	}
}

func TestDocumentPlacementRejectsUnknownPlaceholderSyntax(t *testing.T) {
	for _, location := range []string{
		"docs/runbooks/<slug>.yaml",
		"docs/runbooks/{name}.yaml",
	} {
		t.Run(location, func(t *testing.T) {
			if _, err := compileDocumentLocation(location); err == nil ||
				!strings.Contains(err.Error(), "unrecognized placeholder") {
				t.Fatalf("compile error = %v, want unrecognized placeholder", err)
			}
		})
	}
}

func TestDocumentPlacementReportsMissingRequiredFields(t *testing.T) {
	root := newDocumentPlacementRepository(t, `
document_types:
  constitution:
    location: docs/constitutions/name.yaml
  runbook:
    location: docs/runbooks/name.yaml
    required_fields:
      - id
      - title
      - steps
`, map[string]string{
		"module/docs/runbooks/recovery.yaml": `
id: recovery
steps: []
`,
	})

	err := checkDocumentPlacement(root)
	if err == nil {
		t.Fatal("placement check passed with a missing required field")
	}
	want := "module/docs/runbooks/recovery.yaml: document type runbook missing required field title"
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("error missing %q:\n%v", want, err)
	}
}

func TestDocumentPlacementLoadsRequiredFieldsWithoutCodeChange(t *testing.T) {
	root := newDocumentPlacementRepository(t, `
document_types:
  constitution:
    location: docs/constitutions/name.yaml
  runbook:
    location: docs/runbooks/name.yaml
    required_fields:
      - id
`, map[string]string{
		"module/docs/runbooks/recovery.yaml": "id: recovery",
	})
	if err := checkDocumentPlacement(root); err != nil {
		t.Fatal(err)
	}

	writeDocumentPlacementFile(t, root, documentTypeRegistryPath, `
document_types:
  constitution:
    location: docs/constitutions/name.yaml
  runbook:
    location: docs/runbooks/name.yaml
    required_fields:
      - id
      - owner
`)
	err := checkDocumentPlacement(root)
	if err == nil || !strings.Contains(err.Error(), "missing required field owner") {
		t.Fatalf("placement after required_fields-only change = %v, want missing owner", err)
	}
}

func TestDocumentPlacementChecksPresenceOnlyAndSkipsMarkdown(t *testing.T) {
	root := newDocumentPlacementRepository(t, `
document_types:
  constitution:
    location: docs/constitutions/name.yaml
  runbook:
    location: docs/runbooks/name.yaml
    required_fields:
      - id
      - title
  guide:
    location: docs/guides/name.md
    format: markdown
    required_fields:
      - ignored_for_markdown
`, map[string]string{
		"module/docs/runbooks/recovery.yaml": "id: recovery\ntitle:",
		"module/docs/guides/operator.md":     "# Free-form guide",
	})
	if err := checkDocumentPlacement(root); err != nil {
		t.Fatal(err)
	}
}

func checkDocumentPlacement(root string) error {
	matchers, err := loadDocumentLocationMatchers(filepath.Join(root, documentTypeRegistryPath))
	if err != nil {
		return err
	}
	paths, err := trackedDocumentPaths(root)
	if err != nil {
		return err
	}

	var failures []string
	for _, candidate := range paths {
		if excludedDocumentPath(candidate) {
			continue
		}
		var matched *documentLocationMatcher
		for index := range matchers {
			matcher := &matchers[index]
			if matcher.pattern.MatchString(candidate) {
				matched = matcher
				break
			}
		}
		if matched == nil {
			failures = append(failures,
				candidate+": matches no declared document type location")
			continue
		}
		if strings.EqualFold(matched.format, "markdown") && !matched.declared {
			continue
		}
		missing, err := missingDocumentFields(filepath.Join(root, filepath.FromSlash(candidate)), candidate, matched.required)
		if err != nil {
			failures = append(failures, fmt.Sprintf(
				"%s: document type %s cannot be parsed: %v",
				candidate, matched.documentType, err))
			continue
		}
		for _, field := range missing {
			failures = append(failures, fmt.Sprintf(
				"%s: document type %s missing required field %s",
				candidate, matched.documentType, field))
		}
		// Presence is not resolution: a references entry that named a file
		// which had moved read as current until GH-2341.
		failures = append(failures, documentReferenceFailures(root, candidate)...)
	}
	if len(failures) != 0 {
		return fmt.Errorf("document placement violations:\n%s", strings.Join(failures, "\n"))
	}
	return nil
}

func loadDocumentLocationMatchers(filename string) ([]documentLocationMatcher, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("read document type registry: %w", err)
	}
	var registry documentTypeRegistry
	if err := yaml.Unmarshal(data, &registry); err != nil {
		return nil, fmt.Errorf("parse document type registry: %w", err)
	}

	names := make([]string, 0, len(registry.DocumentTypes))
	for name := range registry.DocumentTypes {
		names = append(names, name)
	}
	sort.Strings(names)

	matchers := make([]documentLocationMatcher, 0, len(names))
	for _, name := range names {
		entry := registry.DocumentTypes[name]
		for _, exclude := range entry.excluded() {
			matchers = append(matchers, documentLocationMatcher{
				documentType: name, location: exclude, format: "yaml",
				required: append([]string(nil), entry.RequiredFields...),
				pattern:  regexp.MustCompile(`(?:^|/)` + regexp.QuoteMeta(exclude) + `$`),
			})
		}
	}
	for _, name := range names {
		entry := registry.DocumentTypes[name]
		pattern, err := compileDocumentLocation(entry.Location)
		if err != nil {
			return nil, fmt.Errorf("document type %q location %q: %w", name, entry.Location, err)
		}
		matcher := documentLocationMatcher{
			documentType: name, location: entry.Location, format: entry.Format,
			pattern: pattern, declared: entry.declared(),
		}
		if !matcher.declared {
			matcher.required = append([]string(nil), entry.RequiredFields...)
		}
		matchers = append(matchers, matcher)
	}
	return matchers, nil
}

func missingDocumentFields(filename, candidate string, required []string) ([]string, error) {
	node, ok, err := corpusDocumentNode(filename, candidate)
	if err != nil || !ok {
		return nil, err
	}
	document := *node

	present := map[string]bool{}
	if len(document.Content) != 0 {
		root := document.Content[0]
		if root.Kind == yaml.MappingNode {
			for index := 0; index+1 < len(root.Content); index += 2 {
				present[root.Content[index].Value] = true
			}
		}
	}
	var missing []string
	for _, field := range required {
		if !present[field] {
			missing = append(missing, field)
		}
	}
	return missing, nil
}

func compileDocumentLocation(location string) (*regexp.Regexp, error) {
	location = filepath.ToSlash(strings.TrimSpace(location))
	if !strings.HasPrefix(location, "docs/") {
		return nil, fmt.Errorf("location must start with docs/")
	}
	if strings.ContainsAny(location, "<>{}") {
		return nil, fmt.Errorf("unrecognized placeholder syntax")
	}

	var pattern strings.Builder
	pattern.WriteString(`(?:^|/)`)
	for index := 0; index < len(location); {
		switch {
		case strings.HasPrefix(location[index:], "-short-name"):
			pattern.WriteString(`(?:-[a-z0-9]+(?:-[a-z0-9]+)*)?`)
			index += len("-short-name")
		case strings.HasPrefix(location[index:], "short-name"):
			pattern.WriteString(`[a-z0-9]+(?:-[a-z0-9]+)*`)
			index += len("short-name")
		case strings.HasPrefix(location[index:], "name"):
			pattern.WriteString(`[a-z0-9][a-z0-9._-]*`)
			index += len("name")
		case location[index] == 'N' && isNumericPlaceholder(location, index):
			end := index
			for end < len(location) && location[end] == 'N' {
				end++
			}
			fmt.Fprintf(&pattern, `[0-9]{%d}`, end-index)
			index = end
		default:
			pattern.WriteString(regexp.QuoteMeta(location[index : index+1]))
			index++
		}
	}
	pattern.WriteString(`$`)
	compiled, err := regexp.Compile(pattern.String())
	if err != nil {
		return nil, fmt.Errorf("compile location: %w", err)
	}
	return compiled, nil
}

func isNumericPlaceholder(location string, index int) bool {
	if index == 0 {
		return true
	}
	previous := location[index-1]
	return previous == '.' || (previous >= 'a' && previous <= 'z')
}

func trackedDocumentPaths(root string) ([]string, error) {
	cmd := exec.Command("git", "ls-files", "-z")
	cmd.Dir = root
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git ls-files: %w", err)
	}

	var paths []string
	for _, item := range bytes.Split(output, []byte{0}) {
		if len(item) == 0 {
			continue
		}
		candidate := filepath.ToSlash(string(item))
		if pathContainsComponent(candidate, "docs") {
			paths = append(paths, candidate)
		}
	}
	sort.Strings(paths)
	return paths, nil
}

func excludedDocumentPath(candidate string) bool {
	parts := strings.Split(filepath.ToSlash(candidate), "/")
	for index, part := range parts {
		switch part {
		case "testdata", "node_modules", "vendor", "build", "generated-files":
			return true
		case "templates":
			// applications/scrum-master/internal/scaffold/templates holds the files
			// tpm init writes into program repositories: scaffolding, not
			// this repository's corpus (GH-2713).
			if index > 0 && parts[index-1] == "scaffold" {
				return true
			}
		case "helm":
			if index+1 < len(parts) &&
				(parts[index+1] == "dist" || parts[index+1] == "profiles") {
				return true
			}
		case "ui":
			if index+1 < len(parts) &&
				(parts[index+1] == "dist" || parts[index+1] == "docs") {
				return true
			}
		}
	}
	return false
}

func pathContainsComponent(candidate, component string) bool {
	for _, part := range strings.Split(filepath.ToSlash(candidate), "/") {
		if part == component {
			return true
		}
	}
	return false
}

func newDocumentPlacementRepository(
	t *testing.T,
	registry string,
	files map[string]string,
) string {
	t.Helper()
	root := t.TempDir()
	writeDocumentPlacementFile(t, root, documentTypeRegistryPath, registry)
	for name, content := range files {
		writeDocumentPlacementFile(t, root, name, content)
	}
	cmd := exec.Command("git", "init", "-q")
	cmd.Dir = root
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, output)
	}
	cmd = exec.Command("git", "add", ".")
	cmd.Dir = root
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git add: %v\n%s", err, output)
	}
	return root
}

func writeDocumentPlacementFile(t *testing.T, root, name, content string) {
	t.Helper()
	filename := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filename, []byte(strings.TrimSpace(content)+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

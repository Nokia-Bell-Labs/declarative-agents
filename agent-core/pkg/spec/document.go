// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package spec

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/pkg/spec/markdown"
	"gopkg.in/yaml.v3"
)

// Corpus documents are Markdown (docs/engineering/eng10-markdown-spec-format.md).
// A document reaches every parser as the YAML tree its declaration parses it
// into, so the typed models and every check read one document model. YAML is
// read only at historical refs: release tags and pinned parent trees that
// predate the migration (GH-2626).
const (
	yamlExt     = ".yaml"
	markdownExt = ".md"
)

// Audit checks the document format raises.
const (
	checkMarkdownFormat = "markdown-format"
	checkNotCanonical   = "not-canonical"
)

// readDocument returns a working-tree corpus document as the YAML tree its
// Markdown parses into. A corpus document in any other format is an error:
// mage convert migrates a YAML corpus once, and nothing reads it in place.
func readDocument(path string) ([]byte, error) {
	if !strings.HasSuffix(path, markdownExt) {
		return nil, fmt.Errorf("%s: corpus documents are Markdown (eng10); run mage convert to migrate a YAML document", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return markdownAsYAML(data, path)
}

// markdownAsYAML parses a Markdown corpus document under its declaration and
// encodes the tree as YAML. A document with structural findings has no tree;
// the format scan reports those findings.
func markdownAsYAML(data []byte, source string) ([]byte, error) {
	d, err := declarationFor(source)
	if err != nil {
		return nil, err
	}
	doc, findings, err := d.Parse(data, source)
	if err != nil {
		return nil, err
	}
	if doc == nil {
		return nil, fmt.Errorf("%s: %d Markdown format finding(s), first %s", source, len(findings), findings[0])
	}
	return yaml.Marshal(doc)
}

func declarationFor(source string) (*markdown.Declaration, error) {
	declarations, err := markdown.Default()
	if err != nil {
		return nil, fmt.Errorf("design constitution: %w", err)
	}
	d, ok := declarations.ForMarkdown(source)
	if !ok {
		return nil, fmt.Errorf("%s: no document type declares this Markdown location", source)
	}
	return d, nil
}

// formatScan finds each corpus document's file and records the findings its
// format raises: a document out of canonical form, and every finding the
// Markdown diagnostics report.
// Findings are collected, never fatal: a document whose structure cannot be
// read is left out and the rest of the corpus loads.
type formatScan struct {
	rootDir  string
	files    map[string]string
	findings []Finding
	leftOut  []string // documents whose structure kept them from loading
	links    markdown.LinkIndex
}

func newFormatScan(rootDir string) *formatScan {
	return &formatScan{rootDir: rootDir, files: map[string]string{}}
}

// record notes the file a document of kind and id was read from.
func (s *formatScan) record(kind, id, path string) {
	s.files[kind+"/"+id] = s.relative(path)
}

// documents lists the Markdown corpus documents in dir whose names start
// with prefix, keeping only those whose structure lets them load.
func (s *formatScan) documents(dir, prefix string) ([]string, error) {
	matches, err := filepath.Glob(filepath.Join(dir, prefix+"*"+markdownExt))
	if err != nil {
		return nil, fmt.Errorf("glob %s: %w", dir, err)
	}
	sort.Strings(matches)
	var paths []string
	for _, match := range matches {
		if s.checkMarkdown(match) {
			paths = append(paths, match)
		} else {
			s.leftOut = append(s.leftOut, s.relative(match))
		}
	}
	return paths, nil
}

// single finds the document with one fixed path, such as docs/road-map.md,
// or "" when it is absent or cannot load.
func (s *formatScan) single(path string) string {
	if _, err := os.Stat(path); err != nil {
		return ""
	}
	if !s.checkMarkdown(path) {
		return ""
	}
	return path
}

// checkMarkdown diagnoses one Markdown document and reports whether its
// structure lets it load.
func (s *formatScan) checkMarkdown(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		s.add(Finding{Check: checkMarkdownFormat, Level: "error", File: s.relative(path), Message: err.Error()})
		return false
	}
	d, err := declarationFor(path)
	if err != nil {
		s.add(Finding{Check: checkMarkdownFormat, Level: "error", File: s.relative(path), Message: err.Error()})
		return false
	}
	formatted, findings, err := d.FormatLinked(data, filepath.ToSlash(path), s.linkIndex())
	loads := err == nil && formatted != nil
	if err != nil && len(findings) == 0 {
		s.add(Finding{Check: checkMarkdownFormat, Level: "error", File: s.relative(path), Line: 1,
			Message: "document cannot be formatted: " + err.Error()})
	}
	for _, f := range findings {
		s.add(Finding{Check: checkMarkdownFormat, CheckID: f.Name, Level: "error", File: s.relative(path), Line: f.Line,
			Message: fmt.Sprintf("%s: expected %s, found %s [design.yaml %s]; %s", f.Name, f.Expected, f.Found, f.Rule, f.Hint)})
	}
	if loads && string(formatted) != string(data) {
		line, found, expected := markdown.FirstDifference(data, formatted)
		s.add(Finding{Check: checkNotCanonical, Level: "error", File: s.relative(path), Line: line,
			Message: fmt.Sprintf("document is not in canonical form: line %d reads %q, canonical form has %q; run mage fmt", line, found, expected)})
	}
	return loads
}

func (s *formatScan) add(f Finding) { s.findings = append(s.findings, f) }

// linkIndex indexes the corpus's Markdown documents once, so the canonical
// form the scan checks links references as mage fmt does (GH-2668).
func (s *formatScan) linkIndex() markdown.LinkIndex {
	if s.links != nil {
		return s.links
	}
	s.links = markdown.LinkIndex{}
	_ = filepath.WalkDir(filepath.Join(s.rootDir, "docs"), func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !strings.HasSuffix(path, markdownExt) {
			return nil
		}
		if _, err := declarationFor(path); err != nil {
			return nil
		}
		if data, err := os.ReadFile(path); err == nil {
			if id := markdown.DocumentID(data); id != "" {
				s.links.Add(id, filepath.ToSlash(path), data)
			}
		}
		return nil
	})
	return s.links
}

func (s *formatScan) relative(path string) string {
	if rel, err := filepath.Rel(s.rootDir, path); err == nil {
		return filepath.ToSlash(rel)
	}
	return filepath.ToSlash(path)
}

// checkDocumentFormat returns the findings the format scan recorded while
// the corpus loaded.
func checkDocumentFormat(corpus *Corpus) []Finding {
	return corpus.FormatFindings
}

// documentFile names the file a document was read from, or the path its id
// implies when the corpus did not record one.
func (c *Corpus) documentFile(kind, subdir, id string) string {
	if file, ok := c.Files[kind+"/"+id]; ok {
		return file
	}
	return filepath.ToSlash(filepath.Join(subdir, id+markdownExt))
}

// isCorpusDocument reports a corpus document at a historical ref, where it
// may predate the migration and be YAML.
func isCorpusDocument(file string) bool {
	return strings.HasSuffix(file, yamlExt) || strings.HasSuffix(file, markdownExt)
}

// documentYAML returns the content of a document read from a git ref as
// YAML: a Markdown document as its parsed tree, and a YAML document, which
// only a ref from before the migration holds, as written.
func documentYAML(content, file string) ([]byte, error) {
	if strings.HasSuffix(file, markdownExt) {
		return markdownAsYAML([]byte(content), file)
	}
	return []byte(content), nil
}

// explain adds to a corpus validation error the format findings of the
// documents the scan left out, which are often why a reference dangles.
func (s *formatScan) explain(err error) error {
	var lines []string
	for _, f := range s.findings {
		if f.Check == checkMarkdownFormat {
			lines = append(lines, fmt.Sprintf("  %s:%d: %s", f.File, f.Line, f.Message))
		}
	}
	if len(lines) == 0 {
		return err
	}
	return fmt.Errorf("%w\ndocuments left out for Markdown format findings:\n%s", err, strings.Join(lines, "\n"))
}

// fileExists reports whether a path a document names exists.
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// loadRelatedDocuments reads the related_documents field of the architecture
// document. A corpus may write the field as a plain list of paths or as a
// table of {doc, purpose} rows; an entry of any other shape is returned as a
// description for a finding, so one malformed entry never stops the audit.
// A corpus without the document has no related documents.
func loadRelatedDocuments(scan *formatScan, file string) ([]string, []string, error) {
	path := scan.single(filepath.Join(scan.rootDir, file))
	if path == "" {
		return nil, nil, nil
	}
	data, err := readDocument(path)
	if err != nil {
		return nil, nil, fmt.Errorf("read architecture %s: %w", path, err)
	}
	var doc struct {
		RelatedDocuments yaml.Node `yaml:"related_documents"`
	}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, nil, fmt.Errorf("parse architecture %s: %w", path, err)
	}
	paths, malformed := relatedDocumentPaths(&doc.RelatedDocuments)
	return paths, malformed, nil
}

// relatedDocumentPaths splits a related_documents node into the paths its
// entries name and descriptions of the entries that name none.
func relatedDocumentPaths(node *yaml.Node) ([]string, []string) {
	switch node.Kind {
	case 0:
		return nil, nil
	case yaml.SequenceNode:
	default:
		return nil, []string{fmt.Sprintf("related_documents is a %s, not a list", yamlKindName(node.Kind))}
	}
	var paths, malformed []string
	for i, item := range node.Content {
		switch {
		case item.Kind == yaml.ScalarNode && item.Value != "":
			paths = append(paths, item.Value)
		case item.Kind == yaml.MappingNode:
			if doc := mappingScalar(item, "doc"); doc != "" {
				paths = append(paths, doc)
				continue
			}
			malformed = append(malformed, fmt.Sprintf("related_documents entry %d has no doc", i+1))
		default:
			malformed = append(malformed, fmt.Sprintf("related_documents entry %d is a %s, not a path or a {doc, purpose} row", i+1, yamlKindName(item.Kind)))
		}
	}
	return paths, malformed
}

func yamlKindName(kind yaml.Kind) string {
	switch kind {
	case yaml.ScalarNode:
		return "scalar"
	case yaml.MappingNode:
		return "mapping"
	case yaml.SequenceNode:
		return "list"
	default:
		return "value"
	}
}

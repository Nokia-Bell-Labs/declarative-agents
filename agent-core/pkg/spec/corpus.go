// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package spec

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"gopkg.in/yaml.v3"
)

// Spec corpus layout paths. These define the expected directory
// structure under the project root for specification artifacts.
// Used by LoadCorpus and the validate state machine.
const (
	DocsDir     = "docs"
	SRDSubdir   = "docs/specs/software-requirements"
	SRDGlob     = "srd*.md"
	UCSubdir    = "docs/specs/use-cases"
	UCGlob      = "rel*.md"
	TSSubdir    = "docs/specs/test-suites"
	TSGlob      = "test-*.md"
	RoadmapFile = "docs/road-map.md"
	SpecFile    = "docs/SPECIFICATIONS.md"
	ArchFile    = "docs/ARCHITECTURE.md"
	AgentsDir   = "agents"
	CoreInstall = "/opt/agent-core"
	SMSubdir    = "docs/specs/semantic-models"
	CFSubdir    = "docs/specs/config-formats"
)

// File-name prefixes of the corpus documents.
const (
	srdPrefix = "srd"
	ucPrefix  = "rel"
	tsPrefix  = "test-"
)

// Corpus holds all parsed specification artifacts for a project.
type Corpus struct {
	RootDir string

	SRDs       map[string]SRD
	UseCases   map[string]UseCase
	TestSuites map[string]TestSuite
	Roadmap    Roadmap
	SpecIndex  SpecIndex

	// RelatedDocuments are the paths ARCHITECTURE.md's related_documents
	// names, in either written shape: a plain path or a {doc, purpose} row.
	RelatedDocuments []string
	// MalformedRelatedDocuments describes related_documents entries that are
	// neither shape; they become findings instead of failing the load.
	MalformedRelatedDocuments []string

	Machines         map[string]core.MachineSpec
	ToolSelections   map[string][]string
	ToolDeclarations map[string]ToolDeclaration
	DocSpecs         map[string]DocSpec

	// UnresolvedDeclFiles lists declaration paths a profile named that could not
	// be read. These are reported as warnings rather than skipped silently, so a
	// profile pointing at an absolute container path on a host checkout says so
	// instead of quietly declaring nothing (GH-1525 R3).
	UnresolvedDeclFiles []string

	SRDOrder     []string
	UCOrder      []string
	MachineOrder []string

	// Files maps "<kind>/<id>" (srd, use_case, test_suite) to the file the
	// document was read from, relative to RootDir, in whichever format it has.
	Files map[string]string
	// FormatFindings are the findings the documents' format raised while the
	// corpus loaded (eng10): Markdown diagnostics, documents in both
	// formats, and Markdown out of canonical form.
	FormatFindings []Finding
}

// CorpusOption configures how LoadCorpus discovers and parses artifacts.
type CorpusOption func(*corpusOptions)

type corpusOptions struct {
	optional bool
}

// WithOptionalCorpus relaxes the SRD-corpus requirement so charter-only audit
// targets load. With this option a missing docs directory, an empty SRD set,
// and missing road-map.md or SPECIFICATIONS.md yield an empty corpus rather
// than an error, letting the jurist run data-driven charters against arbitrary
// repositories. Without it, LoadCorpus keeps requiring a full SRD corpus.
func WithOptionalCorpus() CorpusOption {
	return func(o *corpusOptions) { o.optional = true }
}

// LoadTestSuites loads only formal test-suite artifacts. It supports the
// jurist's evidence-only audit variant for repositories whose broader
// documentation schema is intentionally not an agent-core specification corpus.
func LoadTestSuites(rootDir string) (map[string]TestSuite, error) {
	scan := newFormatScan(rootDir)
	suites, err := discoverAndParseTestSuites(scan)
	if err != nil {
		return nil, err
	}
	if len(scan.leftOut) > 0 {
		return nil, scan.explain(fmt.Errorf("%d test suite(s) cannot be read: %s",
			len(scan.leftOut), strings.Join(scan.leftOut, ", ")))
	}
	return suites, nil
}

// LoadCorpus discovers, parses, and validates all specification artifacts
// under rootDir.
func LoadCorpus(rootDir string, opts ...CorpusOption) (*Corpus, error) {
	var options corpusOptions
	for _, opt := range opts {
		opt(&options)
	}

	docsPath := filepath.Join(rootDir, DocsDir)
	if _, err := os.Stat(docsPath); err != nil {
		if !options.optional || !os.IsNotExist(err) {
			return nil, fmt.Errorf("docs directory not found in %s: %w", rootDir, err)
		}
	}

	scan := newFormatScan(rootDir)
	srds, srdOrder, err := discoverAndParseSRDs(scan, options.optional)
	if err != nil {
		return nil, err
	}

	ucs, ucOrder, err := discoverAndParseUseCases(scan)
	if err != nil {
		return nil, err
	}

	tss, err := discoverAndParseTestSuites(scan)
	if err != nil {
		return nil, err
	}

	rm, err := loadRoadmap(scan, RoadmapFile, options.optional)
	if err != nil {
		return nil, err
	}

	si, err := loadSpecIndex(scan, SpecFile, options.optional)
	if err != nil {
		return nil, err
	}

	related, malformedRelated, err := loadRelatedDocuments(scan, ArchFile)
	if err != nil {
		return nil, err
	}

	machines, toolSel, machineOrder, err := discoverAndParseMachines(rootDir)
	if err != nil {
		return nil, err
	}

	toolDecls, unresolvedDecls, err := discoverAndParseToolDeclarations(rootDir)
	if err != nil {
		return nil, err
	}

	docSpecs, err := discoverAndParseDocSpecs(scan)
	if err != nil {
		return nil, err
	}

	c := &Corpus{
		RootDir:                   rootDir,
		SRDs:                      srds,
		UseCases:                  ucs,
		TestSuites:                tss,
		Roadmap:                   rm,
		SpecIndex:                 si,
		RelatedDocuments:          related,
		MalformedRelatedDocuments: malformedRelated,
		Machines:                  machines,
		ToolSelections:            toolSel,
		ToolDeclarations:          toolDecls,
		UnresolvedDeclFiles:       unresolvedDecls,
		DocSpecs:                  docSpecs,
		SRDOrder:                  srdOrder,
		UCOrder:                   ucOrder,
		MachineOrder:              machineOrder,
		Files:                     scan.files,
		FormatFindings:            scan.findings,
	}

	if err := c.validate(); err != nil {
		return nil, scan.explain(err)
	}
	return c, nil
}

// loadRoadmap parses the road map, treating a missing
// file as an empty roadmap when the corpus is optional.
func loadRoadmap(scan *formatScan, file string, optional bool) (Roadmap, error) {
	path := scan.single(filepath.Join(scan.rootDir, file))
	if path == "" && optional {
		return Roadmap{}, nil
	}
	if path == "" {
		path = filepath.Join(scan.rootDir, file)
	}
	return ParseRoadmap(path)
}

// loadSpecIndex parses the specification index, treating a
// missing file as an empty index when the corpus is optional.
func loadSpecIndex(scan *formatScan, file string, optional bool) (SpecIndex, error) {
	path := scan.single(filepath.Join(scan.rootDir, file))
	if path == "" && optional {
		return SpecIndex{}, nil
	}
	if path == "" {
		path = filepath.Join(scan.rootDir, file)
	}
	return ParseSpecIndex(path)
}

func discoverAndParseSRDs(scan *formatScan, optional bool) (map[string]SRD, []string, error) {
	pattern := filepath.Join(scan.rootDir, SRDSubdir, SRDGlob)
	matches, err := scan.documents(filepath.Join(scan.rootDir, SRDSubdir), srdPrefix)
	if err != nil {
		return nil, nil, fmt.Errorf("glob SRD files: %w", err)
	}
	if len(matches) == 0 {
		if optional {
			return map[string]SRD{}, []string{}, nil
		}
		return nil, nil, fmt.Errorf("no SRD files found matching %s", pattern)
	}

	sort.Strings(matches)

	srds := make(map[string]SRD, len(matches))
	order := make([]string, 0, len(matches))

	for _, path := range matches {
		srd, err := ParseSRD(path)
		if err != nil {
			return nil, nil, err
		}
		if srd.ID == "" {
			return nil, nil, fmt.Errorf("SRD file %s has no id field", path)
		}
		if _, dup := srds[srd.ID]; dup {
			return nil, nil, fmt.Errorf("duplicate SRD id %q in %s", srd.ID, path)
		}
		srds[srd.ID] = srd
		order = append(order, srd.ID)
		scan.record("srd", srd.ID, path)
	}
	return srds, order, nil
}

func discoverAndParseUseCases(scan *formatScan) (map[string]UseCase, []string, error) {
	matches, err := scan.documents(filepath.Join(scan.rootDir, UCSubdir), ucPrefix)
	if err != nil {
		return nil, nil, fmt.Errorf("glob use case files: %w", err)
	}

	sort.Strings(matches)

	ucs := make(map[string]UseCase, len(matches))
	order := make([]string, 0, len(matches))

	for _, path := range matches {
		uc, err := ParseUseCase(path)
		if err != nil {
			return nil, nil, err
		}
		if uc.ID == "" {
			return nil, nil, fmt.Errorf("use case file %s has no id field", path)
		}
		if _, dup := ucs[uc.ID]; dup {
			return nil, nil, fmt.Errorf("duplicate use case id %q in %s", uc.ID, path)
		}
		ucs[uc.ID] = uc
		order = append(order, uc.ID)
		scan.record("use_case", uc.ID, path)
	}
	return ucs, order, nil
}

func discoverAndParseTestSuites(scan *formatScan) (map[string]TestSuite, error) {
	matches, err := scan.documents(filepath.Join(scan.rootDir, TSSubdir), tsPrefix)
	if err != nil {
		return nil, fmt.Errorf("glob test suite files: %w", err)
	}

	tss := make(map[string]TestSuite, len(matches))

	for _, path := range matches {
		ts, err := ParseTestSuite(path)
		if err != nil {
			return nil, err
		}
		if ts.ID == "" {
			return nil, fmt.Errorf("test suite file %s has no id field", path)
		}
		tss[ts.ID] = ts
		scan.record("test_suite", ts.ID, path)
	}
	return tss, nil
}

func discoverAndParseMachines(rootDir string) (map[string]core.MachineSpec, map[string][]string, []string, error) {
	profilesPath := resolveProfileAssetsRoot(rootDir)
	if _, err := os.ReadDir(profilesPath); err != nil {
		if os.IsNotExist(err) {
			return nil, nil, nil, nil
		}
		return nil, nil, nil, fmt.Errorf("read profile root: %w", err)
	}

	machines := make(map[string]core.MachineSpec)
	toolSel := make(map[string][]string)
	selectionPaths := make(map[string]bool)
	var order []string

	for _, pd := range collectProfileDirs(profilesPath) {
		machPath := filepath.Join(pd.Dir, "machine.yaml")
		if _, err := os.Stat(machPath); err != nil {
			continue
		}
		ms, err := core.LoadMachineSpec(machPath)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("parse machine %s: %w", machPath, err)
		}
		machines[pd.Name] = ms
		order = append(order, pd.Name)

		toolsPath := filepath.Join(pd.Dir, "tools.yaml")
		if err := addToolSelection(toolSel, selectionPaths, pd.Name, toolsPath); err != nil {
			if !os.IsNotExist(err) {
				return nil, nil, nil, err
			}
		}
		for key, value := range ms.Configuration {
			if !strings.Contains(strings.ToLower(key), "tools") {
				continue
			}
			path, ok := value.(string)
			if !ok || path == "" {
				continue
			}
			resolved := resolveRootPath(rootDir, filepath.Dir(machPath), path)
			if err := addToolSelection(
				toolSel, selectionPaths, pd.Name+":"+key, resolved,
			); err != nil {
				return nil, nil, nil, err
			}
		}
	}

	sort.Strings(order)
	return machines, toolSel, order, nil
}

func addToolSelection(
	selections map[string][]string,
	seen map[string]bool,
	key, path string,
) error {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("resolve tool selection %s: %w", path, err)
	}
	if seen[absolute] {
		return nil
	}
	data, err := os.ReadFile(absolute)
	if err != nil {
		return err
	}
	tools, err := parseToolSelection(data)
	if err != nil {
		return fmt.Errorf("parse tool selection %s: %w", path, err)
	}
	seen[absolute] = true
	if len(tools) > 0 {
		selections[key] = tools
	}
	return nil
}

func parseToolSelection(data []byte) ([]string, error) {
	var sel ToolSelection
	if err := yaml.Unmarshal(data, &sel); err != nil {
		return nil, err
	}
	return sel.Tools, nil
}

func discoverAndParseDocSpecs(scan *formatScan) (map[string]DocSpec, error) {
	rootDir := scan.rootDir
	specs := make(map[string]DocSpec)
	dirs := []string{
		filepath.Join(rootDir, SMSubdir),
		filepath.Join(rootDir, CFSubdir),
	}
	for _, dir := range dirs {
		matches, err := scan.documents(dir, "")
		if err != nil {
			return nil, fmt.Errorf("glob doc specs in %s: %w", dir, err)
		}
		for _, path := range matches {
			data, err := readDocument(path)
			if err != nil {
				return nil, fmt.Errorf("read doc spec %s: %w", path, err)
			}
			var ds DocSpec
			if err := yaml.Unmarshal(data, &ds); err != nil {
				return nil, fmt.Errorf("parse doc spec %s: %w", path, err)
			}
			if ds.ID == "" {
				continue
			}
			relPath, _ := filepath.Rel(rootDir, path)
			if relPath == "" {
				relPath = path
			}
			ds.SourceFile = relPath
			specs[ds.ID] = ds
		}
	}
	return specs, nil
}

func resolveRootPath(rootDir, base, p string) string {
	if filepath.IsAbs(p) {
		return p
	}
	rootPath := filepath.Join(rootDir, p)
	if _, err := os.Stat(rootPath); err == nil {
		return rootPath
	}
	return filepath.Join(base, p)
}

func (c *Corpus) validate() error {
	var errs []string

	for _, srd := range c.SRDs {
		for _, dep := range srd.DependsOn {
			if _, ok := c.SRDs[dep.SRDID]; !ok {
				errs = append(errs, fmt.Sprintf(
					"SRD %s depends_on %q which does not exist",
					srd.ID, dep.SRDID))
			}
		}
	}

	for _, entry := range c.SpecIndex.SRDIndex {
		if _, ok := c.SRDs[entry.ID]; !ok {
			errs = append(errs, fmt.Sprintf(
				"SPECIFICATIONS.md srd_index references %q which does not exist",
				entry.ID))
		}
	}

	for _, srd := range c.SRDs {
		itemIDs := make(map[string]bool)
		for _, g := range srd.Requirements {
			for _, it := range g.Items {
				itemIDs[it.ID] = true
			}
		}
		for _, ac := range srd.AcceptanceCriteria {
			for _, trace := range ac.Traces {
				if !itemIDs[trace] {
					errs = append(errs, fmt.Sprintf(
						"SRD %s AC %s traces %q which is not a requirement item",
						srd.ID, ac.ID, trace))
				}
			}
		}
	}

	if len(errs) > 0 {
		sort.Strings(errs)
		return fmt.Errorf("corpus validation failed:\n  %s", strings.Join(errs, "\n  "))
	}
	return nil
}

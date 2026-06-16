// Copyright (c) 2026 Nokia. All rights reserved.

package spec

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/runtime/core"
	"gopkg.in/yaml.v3"
)

// Spec corpus layout paths. These define the expected directory
// structure under the project root for specification artifacts.
// Used by LoadCorpus and the validate state machine.
const (
	DocsDir     = "docs"
	SRDSubdir   = "docs/specs/software-requirements"
	SRDGlob     = "srd*.yaml"
	UCSubdir    = "docs/specs/use-cases"
	UCGlob      = "rel*.yaml"
	TSSubdir    = "docs/specs/test-suites"
	TSGlob      = "test-*.yaml"
	RoadmapFile = "docs/road-map.yaml"
	SpecFile    = "docs/SPECIFICATIONS.yaml"
	AgentsDir   = "agents"
	SMSubdir    = "docs/specs/semantic-models"
	CFSubdir    = "docs/specs/config-formats"
)

// Corpus holds all parsed specification artifacts for a project.
type Corpus struct {
	RootDir string

	SRDs       map[string]SRD
	UseCases   map[string]UseCase
	TestSuites map[string]TestSuite
	Roadmap    Roadmap
	SpecIndex  SpecIndex

	Machines         map[string]core.MachineSpec
	ToolSelections   map[string][]string
	ToolDeclarations map[string]ToolDeclaration
	DocSpecs         map[string]DocSpec

	SRDOrder     []string
	UCOrder      []string
	MachineOrder []string
}

// LoadCorpus discovers, parses, and validates all specification artifacts
// under rootDir.
func LoadCorpus(rootDir string) (*Corpus, error) {
	docsPath := filepath.Join(rootDir, DocsDir)
	if _, err := os.Stat(docsPath); err != nil {
		return nil, fmt.Errorf("docs directory not found in %s: %w", rootDir, err)
	}

	srds, srdOrder, err := discoverAndParseSRDs(rootDir)
	if err != nil {
		return nil, err
	}

	ucs, ucOrder, err := discoverAndParseUseCases(rootDir)
	if err != nil {
		return nil, err
	}

	tss, err := discoverAndParseTestSuites(rootDir)
	if err != nil {
		return nil, err
	}

	rmPath := filepath.Join(rootDir, RoadmapFile)
	rm, err := ParseRoadmap(rmPath)
	if err != nil {
		return nil, err
	}

	siPath := filepath.Join(rootDir, SpecFile)
	si, err := ParseSpecIndex(siPath)
	if err != nil {
		return nil, err
	}

	machines, toolSel, machineOrder, err := discoverAndParseMachines(rootDir)
	if err != nil {
		return nil, err
	}

	toolDecls, err := discoverAndParseToolDeclarations(rootDir)
	if err != nil {
		return nil, err
	}

	docSpecs, err := discoverAndParseDocSpecs(rootDir)
	if err != nil {
		return nil, err
	}

	c := &Corpus{
		RootDir:          rootDir,
		SRDs:             srds,
		UseCases:         ucs,
		TestSuites:       tss,
		Roadmap:          rm,
		SpecIndex:        si,
		Machines:         machines,
		ToolSelections:   toolSel,
		ToolDeclarations: toolDecls,
		DocSpecs:         docSpecs,
		SRDOrder:         srdOrder,
		UCOrder:          ucOrder,
		MachineOrder:     machineOrder,
	}

	if err := c.validate(); err != nil {
		return nil, err
	}
	return c, nil
}

func discoverAndParseSRDs(rootDir string) (map[string]SRD, []string, error) {
	pattern := filepath.Join(rootDir, SRDSubdir, SRDGlob)
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, nil, fmt.Errorf("glob SRD files: %w", err)
	}
	if len(matches) == 0 {
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
	}
	return srds, order, nil
}

func discoverAndParseUseCases(rootDir string) (map[string]UseCase, []string, error) {
	pattern := filepath.Join(rootDir, UCSubdir, UCGlob)
	matches, err := filepath.Glob(pattern)
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
	}
	return ucs, order, nil
}

func discoverAndParseTestSuites(rootDir string) (map[string]TestSuite, error) {
	pattern := filepath.Join(rootDir, TSSubdir, TSGlob)
	matches, err := filepath.Glob(pattern)
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
	}
	return tss, nil
}

func discoverAndParseMachines(rootDir string) (map[string]core.MachineSpec, map[string][]string, []string, error) {
	agentsPath := filepath.Join(rootDir, AgentsDir)
	entries, err := os.ReadDir(agentsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil, nil, nil
		}
		return nil, nil, nil, fmt.Errorf("read agents dir: %w", err)
	}

	machines := make(map[string]core.MachineSpec)
	toolSel := make(map[string][]string)
	var order []string

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		agentName := entry.Name()
		machPath := filepath.Join(agentsPath, agentName, "machine.yaml")
		if _, err := os.Stat(machPath); err != nil {
			continue
		}
		ms, err := core.LoadMachineSpec(machPath)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("parse machine %s: %w", machPath, err)
		}
		machines[agentName] = ms
		order = append(order, agentName)

		toolsPath := filepath.Join(agentsPath, agentName, "tools.yaml")
		if data, err := os.ReadFile(toolsPath); err == nil {
			if tools := parseToolSelection(data); len(tools) > 0 {
				toolSel[agentName] = tools
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
			if data, err := os.ReadFile(resolveRootPath(rootDir, filepath.Dir(machPath), path)); err == nil {
				if tools := parseToolSelection(data); len(tools) > 0 {
					toolSel[agentName+":"+key] = tools
				}
			}
		}
	}

	sort.Strings(order)
	return machines, toolSel, order, nil
}

func parseToolSelection(data []byte) []string {
	var sel ToolSelection
	if err := yaml.Unmarshal(data, &sel); err != nil {
		return nil
	}
	return sel.Tools
}

func discoverAndParseDocSpecs(rootDir string) (map[string]DocSpec, error) {
	specs := make(map[string]DocSpec)
	dirs := []string{
		filepath.Join(rootDir, SMSubdir),
		filepath.Join(rootDir, CFSubdir),
	}
	for _, dir := range dirs {
		matches, err := filepath.Glob(filepath.Join(dir, "*.yaml"))
		if err != nil {
			return nil, fmt.Errorf("glob doc specs in %s: %w", dir, err)
		}
		for _, path := range matches {
			data, err := os.ReadFile(path)
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

func discoverAndParseToolDeclarations(rootDir string) (map[string]ToolDeclaration, error) {
	decls := make(map[string]ToolDeclaration)

	declFiles := []string{
		filepath.Join(rootDir, "tools", "builtin.yaml"),
		filepath.Join(rootDir, "tools", "exec.yaml"),
	}
	declFiles = append(declFiles, yamlFilesInDir(filepath.Join(rootDir, "tools", "builtin"))...)
	declFiles = append(declFiles, yamlFilesInDir(filepath.Join(rootDir, "tools", "exec"))...)

	agentsPath := filepath.Join(rootDir, AgentsDir)
	if entries, err := os.ReadDir(agentsPath); err == nil {
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			override := filepath.Join(agentsPath, entry.Name(), "builtin.yaml")
			if _, err := os.Stat(override); err == nil {
				declFiles = append(declFiles, override)
			}
			declFiles = append(declFiles, yamlFilesInDir(filepath.Join(agentsPath, entry.Name(), "llm"))...)
			declFiles = append(declFiles, declarationFilesFromProfile(filepath.Join(agentsPath, entry.Name(), "profile.yaml"))...)
		}
	}

	for _, path := range declFiles {
		data, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("read tool declarations %s: %w", path, err)
		}
		var file ToolDeclFile
		if err := yaml.Unmarshal(data, &file); err != nil {
			return nil, fmt.Errorf("parse tool declarations %s: %w", path, err)
		}
		relPath, _ := filepath.Rel(rootDir, path)
		if relPath == "" {
			relPath = path
		}
		for _, td := range file.Tools {
			td.SourceFile = relPath
			if existing, ok := decls[td.Name]; ok && keepExistingToolDeclaration(existing, td) {
				continue
			}
			decls[td.Name] = td
		}
	}

	return decls, nil
}

func keepExistingToolDeclaration(existing, candidate ToolDeclaration) bool {
	return isAgentLocalToolDeclaration(existing.SourceFile) && !isAgentLocalToolDeclaration(candidate.SourceFile)
}

func isAgentLocalToolDeclaration(sourceFile string) bool {
	return strings.HasPrefix(filepath.ToSlash(sourceFile), "agents/")
}

func yamlFilesInDir(dir string) []string {
	matches, err := filepath.Glob(filepath.Join(dir, "*.yaml"))
	if err != nil {
		return nil
	}
	sort.Strings(matches)
	return matches
}

func declarationFilesFromProfile(path string) []string {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var profile struct {
		ToolDeclarations []string `yaml:"tool_declarations"`
		ToolConfigDirs   []string `yaml:"tool_config_dirs"`
	}
	if err := yaml.Unmarshal(data, &profile); err != nil {
		return nil
	}
	base := filepath.Dir(path)
	var files []string
	for _, dir := range profile.ToolConfigDirs {
		files = append(files, yamlFilesInDir(resolveProfilePath(base, dir))...)
	}
	for _, decl := range profile.ToolDeclarations {
		files = append(files, resolveProfilePath(base, decl))
	}
	return files
}

func resolveProfilePath(base, p string) string {
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(base, p)
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
				"SPECIFICATIONS.yaml srd_index references %q which does not exist",
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

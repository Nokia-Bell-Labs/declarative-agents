// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

// Package reusestats measures duplication, declaration ceremony, and tool reuse
// across owned YAML configuration roots.
package reusestats

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Result is the stable JSON shape emitted by module and repository Mage targets.
type Result struct {
	TotalLines       int     `json:"total_lines"`
	DuplicatedLines  int     `json:"duplicated_lines"`
	DuplicationRatio float64 `json:"duplication_ratio"`
	CeremonyLines    int     `json:"ceremony_lines"`
	BehaviorLines    int     `json:"behavior_lines"`
	CeremonyRatio    float64 `json:"ceremony_ratio"`
	DistinctToolDefs int     `json:"distinct_tool_defs"`
	ToolRefs         int     `json:"tool_refs"`
	// ImportedUnits is the number of distinct files something imports.
	// SharedUnits have two or more importing files; SingleImporterUnits have
	// one, which is an include by another name. Instantiations counts
	// instantiate entries, a fragment applied with arguments (srd052).
	ImportedUnits       int              `json:"imported_units"`
	SharedUnits         int              `json:"shared_units"`
	SingleImporterUnits int              `json:"single_importer_units"`
	Instantiations      int              `json:"instantiations"`
	TopBlocks           []DuplicateBlock `json:"top_blocks"`
}

// DuplicateBlock describes one repeated canonical YAML mapping.
type DuplicateBlock struct {
	Hash  string   `json:"hash"`
	Lines int      `json:"lines"`
	Count int      `json:"count"`
	Files []string `json:"files"`
}

type occurrence struct {
	file  string
	line  int
	lines int
}

type collector struct {
	base        string
	result      Result
	definitions map[string]bool
	blocks      map[string][]occurrence
	// importers maps a resolved unit path to the files that import it, so a
	// file importing one unit twice counts as one importer.
	importers map[string]map[string]bool
}

var ceremonyFields = map[string]bool{
	"problem": true, "goals": true, "requirements": true, "non_goals": true,
	"side_effects": true, "reversibility": true, "undo": true,
	"relationships": true, "visibility": true,
}

var behaviorFields = map[string]bool{
	"config": true, "signature": true, "emits": true,
	"parameters": true, "output": true,
}

var environmentReference = regexp.MustCompile(`\$\{[^}\n]+\}`)

// Reuse emits reuse metrics for the conventional agent-owned YAML roots.
func Reuse() error {
	result, err := Collect(".", "agents", "tools", "testdata")
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(result)
}

// Collect measures all YAML files under roots. Relative roots resolve from
// baseDir; absent roots are ignored so thin module adapters can declare the
// same ownership classes.
func Collect(baseDir string, roots ...string) (Result, error) {
	base, err := filepath.Abs(baseDir)
	if err != nil {
		return Result{}, fmt.Errorf("resolve reuse stats base %s: %w", baseDir, err)
	}
	files, err := yamlFiles(base, roots)
	if err != nil {
		return Result{}, err
	}
	c := collector{
		base: base, definitions: map[string]bool{},
		blocks: map[string][]occurrence{}, importers: map[string]map[string]bool{},
	}
	for _, path := range files {
		if err := c.collectFile(path); err != nil {
			return Result{}, err
		}
	}
	c.finish()
	return c.result, nil
}

func yamlFiles(base string, roots []string) ([]string, error) {
	seen := map[string]bool{}
	for _, root := range roots {
		path := root
		if !filepath.IsAbs(path) {
			path = filepath.Join(base, path)
		}
		info, err := os.Stat(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("stat reuse root %s: %w", path, err)
		}
		if !info.IsDir() {
			addYAMLFile(seen, path)
			continue
		}
		if err := filepath.WalkDir(path, func(candidate string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() && candidate != path && skippedDirectory(entry.Name()) {
				return filepath.SkipDir
			}
			if !entry.IsDir() {
				addYAMLFile(seen, candidate)
			}
			return nil
		}); err != nil {
			return nil, fmt.Errorf("walk reuse root %s: %w", path, err)
		}
	}
	files := make([]string, 0, len(seen))
	for path := range seen {
		files = append(files, path)
	}
	sort.Strings(files)
	return files, nil
}

func skippedDirectory(name string) bool {
	switch name {
	case ".git", "node_modules", "vendor":
		return true
	}
	return false
}

func addYAMLFile(files map[string]bool, path string) {
	extension := strings.ToLower(filepath.Ext(path))
	if extension == ".yaml" || extension == ".yml" {
		files[filepath.Clean(path)] = true
	}
}

func (c *collector) collectFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read reuse YAML %s: %w", path, err)
	}
	c.result.TotalLines += physicalLines(data)
	var document yaml.Node
	if err := yaml.Unmarshal(normalizeEnvironmentReferences(data), &document); err != nil {
		return fmt.Errorf("parse reuse YAML %s: %w", path, err)
	}
	root := documentRoot(&document)
	relative, err := filepath.Rel(c.base, path)
	if err != nil {
		relative = path
	}
	relative = filepath.ToSlash(relative)
	if err := c.collectMappings(root, relative); err != nil {
		return fmt.Errorf("measure reuse YAML %s: %w", path, err)
	}
	c.collectTools(root)
	c.collectMachineActions(root)
	c.collectUnitEdges(root, path)
	return nil
}

// collectUnitEdges records the top-level imports and instantiate entries of
// one declaration file (srd050 R1.2, srd052 R2.1). An import path is relative
// to the importing file, so the same unit reached from two directories
// resolves to one key; a library-rooted path is its own key (srd056 R1.1).
// Only scalar import entries and instantiate entries naming a fragment count;
// anything else is not an edge the loader follows.
func (c *collector) collectUnitEdges(root *yaml.Node, path string) {
	directory := filepath.Dir(path)
	if imports := mappingValue(root, "imports"); imports != nil && imports.Kind == yaml.SequenceNode {
		for _, imported := range imports.Content {
			if imported.Kind == yaml.ScalarNode && imported.Value != "" {
				c.recordImporter(unitKey(directory, imported.Value), path)
			}
		}
	}
	c.collectInstantiations(mappingValue(root, "instantiate"), directory, path)
	// A machine template's body instantiates its stages (srd054 R2.2).
	if machine := mappingValue(root, "machine"); machine != nil && machine.Kind == yaml.MappingNode {
		c.collectInstantiations(mappingValue(machine, "instantiate"), directory, path)
	}
}

func (c *collector) collectInstantiations(instantiate *yaml.Node, directory, path string) {
	if instantiate == nil || instantiate.Kind != yaml.SequenceNode {
		return
	}
	for _, entry := range instantiate.Content {
		if fragment := scalarMappingValue(entry, "fragment"); fragment != "" {
			c.result.Instantiations++
			c.recordImporter(unitKey(directory, fragment), path)
		}
	}
}

func unitKey(directory, reference string) string {
	if filepath.IsAbs(reference) {
		return reference
	}
	return filepath.Join(directory, reference)
}

func (c *collector) recordImporter(unit, importer string) {
	unit = filepath.Clean(unit)
	if c.importers[unit] == nil {
		c.importers[unit] = map[string]bool{}
	}
	c.importers[unit][filepath.Clean(importer)] = true
}

func normalizeEnvironmentReferences(data []byte) []byte {
	return environmentReference.ReplaceAllFunc(data, func(reference []byte) []byte {
		sum := sha256.Sum256(reference)
		return []byte("envref_" + hex.EncodeToString(sum[:8]))
	})
}

func physicalLines(data []byte) int {
	if len(data) == 0 {
		return 0
	}
	lines := bytes.Count(data, []byte{'\n'})
	if data[len(data)-1] != '\n' {
		lines++
	}
	return lines
}

func documentRoot(document *yaml.Node) *yaml.Node {
	if document != nil && document.Kind == yaml.DocumentNode && len(document.Content) > 0 {
		return document.Content[0]
	}
	return document
}

func (c *collector) collectMappings(node *yaml.Node, file string) error {
	if node == nil {
		return nil
	}
	if node.Kind == yaml.MappingNode {
		lines := nodeLines(node)
		if lines >= 3 {
			rendered, err := canonicalMapping(node)
			if err != nil {
				return err
			}
			sum := sha256.Sum256(rendered)
			hash := hex.EncodeToString(sum[:])
			c.blocks[hash] = append(c.blocks[hash], occurrence{
				file: file, line: node.Line, lines: lines,
			})
		}
	}
	for _, child := range node.Content {
		if err := c.collectMappings(child, file); err != nil {
			return err
		}
	}
	return nil
}

func nodeLines(node *yaml.Node) int {
	if node == nil || node.Line == 0 {
		return 0
	}
	end := nodeEndLine(node)
	return end - node.Line + 1
}

func nodeEndLine(node *yaml.Node) int {
	end := node.Line + strings.Count(node.Value, "\n")
	for _, child := range node.Content {
		if childEnd := nodeEndLine(child); childEnd > end {
			end = childEnd
		}
	}
	return end
}

func canonicalMapping(node *yaml.Node) ([]byte, error) {
	cloned := cloneNode(node)
	var output bytes.Buffer
	encoder := yaml.NewEncoder(&output)
	encoder.SetIndent(2)
	if err := encoder.Encode(cloned); err != nil {
		return nil, fmt.Errorf("encode canonical mapping: %w", err)
	}
	if err := encoder.Close(); err != nil {
		return nil, fmt.Errorf("close canonical mapping encoder: %w", err)
	}
	return bytes.TrimSpace(output.Bytes()), nil
}

func cloneNode(node *yaml.Node) *yaml.Node {
	if node == nil {
		return nil
	}
	clone := *node
	clone.HeadComment, clone.LineComment, clone.FootComment = "", "", ""
	clone.Line, clone.Column = 0, 0
	clone.Content = make([]*yaml.Node, len(node.Content))
	for index, child := range node.Content {
		clone.Content[index] = cloneNode(child)
	}
	if node.Alias != nil {
		clone.Alias = cloneNode(node.Alias)
	}
	return &clone
}

func (c *collector) collectTools(root *yaml.Node) {
	tools := mappingValue(root, "tools")
	if tools == nil || tools.Kind != yaml.SequenceNode {
		return
	}
	if mappingValue(root, "machine") != nil {
		return
	}
	for _, tool := range tools.Content {
		switch tool.Kind {
		case yaml.MappingNode:
			if name := scalarMappingValue(tool, "name"); name != "" {
				c.definitions[name] = true
			}
			c.collectToolFieldLines(tool)
		case yaml.ScalarNode:
			if tool.Value != "" {
				c.result.ToolRefs++
			}
		}
	}
}

func (c *collector) collectToolFieldLines(tool *yaml.Node) {
	for index := 0; index+1 < len(tool.Content); index += 2 {
		name := tool.Content[index].Value
		lines := mappingFieldLines(tool, index)
		if ceremonyFields[name] {
			c.result.CeremonyLines += lines
		}
		if behaviorFields[name] {
			c.result.BehaviorLines += lines
		}
	}
}

func mappingFieldLines(mapping *yaml.Node, keyIndex int) int {
	start := mapping.Content[keyIndex].Line
	if keyIndex+2 < len(mapping.Content) {
		return mapping.Content[keyIndex+2].Line - start
	}
	return nodeEndLine(mapping) - start + 1
}

// collectMachineActions counts the named actions a machine declares. A machine
// template declares its transitions under its machine body (srd054), and its
// instances declare none, so the template file is where they count, once
// (GH-2132).
func (c *collector) collectMachineActions(root *yaml.Node) {
	transitions := mappingValue(root, "transitions")
	if machine := mappingValue(root, "machine"); transitions == nil && machine != nil && machine.Kind == yaml.MappingNode {
		transitions = mappingValue(machine, "transitions")
	}
	if transitions == nil || transitions.Kind != yaml.SequenceNode {
		return
	}
	for _, transition := range transitions.Content {
		action := scalarMappingValue(transition, "action")
		if action != "" && action != "$tool" {
			c.result.ToolRefs++
		}
	}
}

func mappingValue(mapping *yaml.Node, name string) *yaml.Node {
	if mapping == nil || mapping.Kind != yaml.MappingNode {
		return nil
	}
	for index := 0; index+1 < len(mapping.Content); index += 2 {
		if mapping.Content[index].Value == name {
			return mapping.Content[index+1]
		}
	}
	return nil
}

func scalarMappingValue(mapping *yaml.Node, name string) string {
	value := mappingValue(mapping, name)
	if value == nil || value.Kind != yaml.ScalarNode {
		return ""
	}
	return value.Value
}

func (c *collector) finish() {
	c.result.DistinctToolDefs = len(c.definitions)
	c.result.ImportedUnits = len(c.importers)
	for _, files := range c.importers {
		if len(files) >= 2 {
			c.result.SharedUnits++
		} else {
			c.result.SingleImporterUnits++
		}
	}
	blocks := make([]DuplicateBlock, 0)
	duplicatedByHash := map[string]int{}
	for hash, occurrences := range c.blocks {
		if len(occurrences) < 2 {
			continue
		}
		sort.Slice(occurrences, func(i, j int) bool {
			if occurrences[i].file != occurrences[j].file {
				return occurrences[i].file < occurrences[j].file
			}
			return occurrences[i].line < occurrences[j].line
		})
		for _, duplicate := range occurrences[1:] {
			c.result.DuplicatedLines += duplicate.lines
			duplicatedByHash[hash] += duplicate.lines
		}
		blocks = append(blocks, duplicateBlock(hash, occurrences))
	}
	sort.Slice(blocks, func(i, j int) bool {
		left := duplicatedByHash[blocks[i].Hash]
		right := duplicatedByHash[blocks[j].Hash]
		if left != right {
			return left > right
		}
		return blocks[i].Hash < blocks[j].Hash
	})
	if len(blocks) > 10 {
		blocks = blocks[:10]
	}
	c.result.TopBlocks = blocks
	c.result.DuplicationRatio = ratio(c.result.DuplicatedLines, c.result.TotalLines)
	c.result.CeremonyRatio = ratio(c.result.CeremonyLines, c.result.BehaviorLines)
}

func duplicateBlock(hash string, occurrences []occurrence) DuplicateBlock {
	files := map[string]bool{}
	for _, item := range occurrences {
		files[item.file] = true
	}
	paths := make([]string, 0, len(files))
	for path := range files {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return DuplicateBlock{
		Hash: hash, Lines: occurrences[0].lines,
		Count: len(occurrences), Files: paths,
	}
}

func ratio(numerator, denominator int) float64 {
	if denominator == 0 {
		return 0
	}
	return float64(numerator) / float64(denominator)
}

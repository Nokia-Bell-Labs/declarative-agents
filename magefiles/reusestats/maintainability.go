// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package reusestats

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

// DefaultRoots are the conventional agent-owned YAML roots every stats
// participant measures, so the repository fold can recollect the same files.
var DefaultRoots = []string{"agents", "tools", "testdata"}

// longestFileCount is how many of the longest files a result names.
const longestFileCount = 10

// Maintainability measures what makes declarations hard to maintain: file
// length, how many files one agent spans, and import coupling (GH-2111).
type Maintainability struct {
	Files               int        `json:"files"`
	MedianLines         int        `json:"median_lines"`
	P90Lines            int        `json:"p90_lines"`
	MaxLines            int        `json:"max_lines"`
	FilesOver300        int        `json:"files_over_300"`
	FilesOver500        int        `json:"files_over_500"`
	LongestFiles        []FileSize `json:"longest_files"`
	Agents              int        `json:"agents"`
	MedianFilesPerAgent int        `json:"median_files_per_agent"`
	MaxFilesPerAgent    int        `json:"max_files_per_agent"`
	MedianImportFanOut  int        `json:"median_import_fan_out"`
	MaxImportFanOut     int        `json:"max_import_fan_out"`
	MedianUnitFanIn     int        `json:"median_unit_fan_in"`
	MaxUnitFanIn        int        `json:"max_unit_fan_in"`
}

// FileSize names one file and its physical line count.
type FileSize struct {
	Path  string `json:"path"`
	Lines int    `json:"lines"`
}

// Samples are the raw observations behind Maintainability. A repository total
// folds samples, not module medians, so its medians are exact.
type Samples struct {
	Files         []FileSize
	FilesPerAgent []int
	ImportFanOut  []int
	UnitFanIn     []int
}

// Collect measures all YAML files under roots. Relative roots resolve from
// baseDir; absent roots are ignored so thin module adapters can declare the
// same ownership classes.
func Collect(baseDir string, roots ...string) (Result, error) {
	result, _, err := collect(baseDir, roots)
	return result, err
}

// CollectSamples returns the maintainability samples behind Collect's counters,
// for a caller folding several modules into one exact total.
func CollectSamples(baseDir string, roots ...string) (Samples, error) {
	_, samples, err := collect(baseDir, roots)
	return samples, err
}

func collect(baseDir string, roots []string) (Result, Samples, error) {
	base, err := filepath.Abs(baseDir)
	if err != nil {
		return Result{}, Samples{}, fmt.Errorf("resolve reuse stats base %s: %w", baseDir, err)
	}
	files, err := yamlFiles(base, roots)
	if err != nil {
		return Result{}, Samples{}, err
	}
	c := collector{
		base: base, definitions: map[string]bool{},
		blocks: map[string][]occurrence{}, importers: map[string]map[string]bool{},
	}
	var samples Samples
	for _, path := range files {
		lines, err := c.collectFile(path)
		if err != nil {
			return Result{}, Samples{}, err
		}
		samples.Files = append(samples.Files, FileSize{Path: c.relative(path), Lines: lines})
	}
	c.finish()
	samples.FilesPerAgent = filesPerAgent(files)
	samples.ImportFanOut, samples.UnitFanIn = importDegrees(c.importers)
	c.result.Maintainability = Summarize(samples)
	return c.result, samples, nil
}

func (c *collector) relative(path string) string {
	relative, err := filepath.Rel(c.base, path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(relative)
}

// filesPerAgent counts, for each agent directory (one holding profile.yaml),
// the YAML files whose nearest agent ancestor it is, leaving out tests/ and
// units/ subtrees.
func filesPerAgent(files []string) []int {
	agents := map[string]int{}
	for _, path := range files {
		if filepath.Base(path) == "profile.yaml" {
			agents[filepath.Dir(path)] = 0
		}
	}
	for _, path := range files {
		if agent, ok := owningAgent(path, agents); ok {
			agents[agent]++
		}
	}
	counts := make([]int, 0, len(agents))
	for _, count := range agents {
		counts = append(counts, count)
	}
	sort.Ints(counts)
	return counts
}

func owningAgent(path string, agents map[string]int) (string, bool) {
	for directory := filepath.Dir(path); ; directory = filepath.Dir(directory) {
		if name := filepath.Base(directory); name == "tests" || name == "units" {
			return "", false
		}
		if _, ok := agents[directory]; ok {
			return directory, true
		}
		if parent := filepath.Dir(directory); parent == directory {
			return "", false
		}
	}
}

// importDegrees returns each importing file's distinct unit count and each
// unit's distinct importer count, from the graph shared_units already counts.
func importDegrees(importers map[string]map[string]bool) (fanOut, fanIn []int) {
	outgoing := map[string]int{}
	for _, files := range importers {
		fanIn = append(fanIn, len(files))
		for file := range files {
			outgoing[file]++
		}
	}
	for _, count := range outgoing {
		fanOut = append(fanOut, count)
	}
	sort.Ints(fanOut)
	sort.Ints(fanIn)
	return fanOut, fanIn
}

// Summarize reduces samples to the reported counters.
func Summarize(samples Samples) Maintainability {
	lines := make([]int, len(samples.Files))
	for index, file := range samples.Files {
		lines[index] = file.Lines
	}
	sort.Ints(lines)
	m := Maintainability{
		Files: len(lines), MedianLines: median(lines), P90Lines: percentile90(lines), MaxLines: maximum(lines),
		LongestFiles: longestFiles(samples.Files),
		Agents:       len(samples.FilesPerAgent), MedianFilesPerAgent: median(sorted(samples.FilesPerAgent)),
		MaxFilesPerAgent:   maximum(sorted(samples.FilesPerAgent)),
		MedianImportFanOut: median(sorted(samples.ImportFanOut)), MaxImportFanOut: maximum(sorted(samples.ImportFanOut)),
		MedianUnitFanIn: median(sorted(samples.UnitFanIn)), MaxUnitFanIn: maximum(sorted(samples.UnitFanIn)),
	}
	for _, count := range lines {
		if count > 300 {
			m.FilesOver300++
		}
		if count > 500 {
			m.FilesOver500++
		}
	}
	return m
}

// Merge concatenates module samples, prefixing each file path with its module.
func Merge(modules map[string]Samples) Samples {
	names := make([]string, 0, len(modules))
	for name := range modules {
		names = append(names, name)
	}
	sort.Strings(names)
	var merged Samples
	for _, name := range names {
		samples := modules[name]
		for _, file := range samples.Files {
			merged.Files = append(merged.Files, FileSize{Path: strings.TrimPrefix(name+"/"+file.Path, "./"), Lines: file.Lines})
		}
		merged.FilesPerAgent = append(merged.FilesPerAgent, samples.FilesPerAgent...)
		merged.ImportFanOut = append(merged.ImportFanOut, samples.ImportFanOut...)
		merged.UnitFanIn = append(merged.UnitFanIn, samples.UnitFanIn...)
	}
	return merged
}

func longestFiles(files []FileSize) []FileSize {
	ranked := append([]FileSize(nil), files...)
	sort.Slice(ranked, func(i, j int) bool {
		if ranked[i].Lines != ranked[j].Lines {
			return ranked[i].Lines > ranked[j].Lines
		}
		return ranked[i].Path < ranked[j].Path
	})
	if len(ranked) > longestFileCount {
		ranked = ranked[:longestFileCount]
	}
	return ranked
}

func sorted(values []int) []int {
	copied := append([]int(nil), values...)
	sort.Ints(copied)
	return copied
}

// median is the lower median of sorted values, an observed value rather than
// an average of two.
func median(values []int) int {
	if len(values) == 0 {
		return 0
	}
	return values[(len(values)-1)/2]
}

// percentile90 is the nearest-rank 90th percentile of sorted values.
func percentile90(values []int) int {
	if len(values) == 0 {
		return 0
	}
	rank := (9*len(values) + 9) / 10
	return values[rank-1]
}

func maximum(values []int) int {
	if len(values) == 0 {
		return 0
	}
	return values[len(values)-1]
}

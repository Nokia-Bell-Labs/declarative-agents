// Copyright (c) 2026 Nokia. All rights reserved.

package filesystem

import (
	"encoding/json"
	"fmt"
	"mime"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

const (
	SignalDocumentListReady      core.Signal = "DocumentListReady"
	SignalDocumentReady          core.Signal = "DocumentReady"
	SignalDocumentMissing        core.Signal = "DocumentMissing"
	SignalDocumentResourceDenied core.Signal = "DocumentResourceDenied"
	SignalDocumentParseFailed    core.Signal = "DocumentParseFailed"
)

// ResourceConfig defines trusted filesystem resources available to resource words.
type ResourceConfig struct {
	Resources map[string]ResourceDefinition `json:"resources"`
	Resource  string                        `json:"resource,omitempty"`
}

// ResourceDefinition defines one read-only filesystem resource.
type ResourceDefinition struct {
	Root          string                 `json:"root"`
	Include       []string               `json:"include"`
	Extensions    []string               `json:"extensions"`
	Modes         []string               `json:"modes"`
	MaxBytes      int64                  `json:"max_bytes"`
	CategoryRules []ResourceCategoryRule `json:"category_rules,omitempty"`
}

// ResourceCategoryRule assigns profile-owned presentation metadata by path.
type ResourceCategoryRule struct {
	Pattern  string `json:"pattern"`
	Category string `json:"category"`
}

type resourceEntry struct {
	Path      string                 `json:"path"`
	Name      string                 `json:"name"`
	Category  string                 `json:"category"`
	Extension string                 `json:"extension"`
	Size      int64                  `json:"size"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

type resourceDetail struct {
	Path        string      `json:"path"`
	Raw         string      `json:"raw,omitempty"`
	Parsed      interface{} `json:"parsed,omitempty"`
	ContentType string      `json:"content_type"`
	Extension   string      `json:"extension"`
	Size        int64       `json:"size"`
}

type listResourceCmd struct {
	root      string
	resource  string
	paths     string
	resources ResourceConfig
}

func (l *listResourceCmd) Name() string                   { return "list_resource" }
func (l *listResourceCmd) Undo(_ core.Result) core.Result { return core.NoopUndo(l.Name()) }

func (l *listResourceCmd) Execute() core.Result {
	def, base, ok := l.resourceBase(l.resource)
	if !ok {
		return commandError(l.Name(), fmt.Errorf("resource %q is not configured", l.resource))
	}
	entries, err := listResourceEntries(base, def, l.paths)
	if err != nil {
		return resourceError(l.Name(), err)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
	return resourceJSON(l.Name(), SignalDocumentListReady, entries)
}

func (l *listResourceCmd) resourceBase(name string) (ResourceDefinition, string, bool) {
	def, ok := l.resources.Resources[name]
	if !ok {
		return ResourceDefinition{}, "", false
	}
	return def, resourceRoot(l.root, def.Root), true
}

type readResourceCmd struct {
	root      string
	resource  string
	path      string
	resources ResourceConfig
}

func (r *readResourceCmd) Name() string                   { return "read_resource" }
func (r *readResourceCmd) Undo(_ core.Result) core.Result { return core.NoopUndo(r.Name()) }

func (r *readResourceCmd) Execute() core.Result {
	def, base, ok := r.resourceBase(r.resource)
	if !ok {
		return commandError(r.Name(), fmt.Errorf("resource %q is not configured", r.resource))
	}
	resolved, rel, err := resolveResourcePath(base, def, r.path)
	if err != nil {
		return resourceError(r.Name(), err)
	}
	detail, err := readResourceDetail(resolved, rel, def)
	if err != nil {
		return resourceError(r.Name(), err)
	}
	return resourceJSON(r.Name(), SignalDocumentReady, detail)
}

func (r *readResourceCmd) resourceBase(name string) (ResourceDefinition, string, bool) {
	def, ok := r.resources.Resources[name]
	if !ok {
		return ResourceDefinition{}, "", false
	}
	return def, resourceRoot(r.root, def.Root), true
}

// ListResourceBuilder constructs list_resource commands.
type ListResourceBuilder struct {
	Root      string
	Resources ResourceConfig
}

func (b *ListResourceBuilder) Build(res core.Result) core.Command {
	return &listResourceCmd{
		root:      b.Root,
		resource:  configuredResource(b.Resources, res.Output),
		paths:     res.Output,
		resources: b.Resources,
	}
}

// ReadResourceBuilder constructs read_resource commands.
type ReadResourceBuilder struct {
	Root      string
	Resources ResourceConfig
}

func (b *ReadResourceBuilder) Build(res core.Result) core.Command {
	p := extractStringParam(res.Output, "path")
	if p == "" {
		return missingParam("read_resource", "path")
	}
	return &readResourceCmd{
		root:      b.Root,
		resource:  configuredResource(b.Resources, res.Output),
		path:      p,
		resources: b.Resources,
	}
}

func configuredResource(config ResourceConfig, output string) string {
	if config.Resource != "" {
		return config.Resource
	}
	return extractStringParam(output, "resource")
}

func listResourceEntries(base string, def ResourceDefinition, paths string) ([]resourceEntry, error) {
	var entries []resourceEntry
	for _, candidate := range strings.Split(strings.TrimSpace(paths), "\n") {
		if strings.TrimSpace(candidate) == "" {
			continue
		}
		rel, err := normalizeResourcePath(candidate)
		if err != nil {
			return nil, err
		}
		if !resourcePathAllowed(rel, def, "") {
			continue
		}
		if _, err := ValidatePath(base, rel); err != nil {
			return nil, err
		}
		if err := appendResourceEntry(&entries, base, filepath.Join(base, filepath.FromSlash(rel)), def); err != nil {
			return nil, err
		}
	}
	return entries, nil
}

func appendResourceEntry(entries *[]resourceEntry, base, abs string, def ResourceDefinition) error {
	rel, err := resourceRel(base, abs)
	if err != nil {
		return err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return nil
	}
	*entries = append(*entries, resourceEntry{
		Path:      rel,
		Name:      strings.TrimSuffix(path.Base(rel), path.Ext(rel)),
		Category:  resourceCategory(rel, def.CategoryRules),
		Extension: strings.TrimPrefix(path.Ext(rel), "."),
		Size:      info.Size(),
	})
	return nil
}

func readResourceDetail(abs, rel string, def ResourceDefinition) (resourceDetail, error) {
	info, err := os.Stat(abs)
	if err != nil {
		return resourceDetail{}, err
	}
	if info.IsDir() {
		return resourceDetail{}, resourceDenied("directory reads are not allowed")
	}
	if def.MaxBytes > 0 && info.Size() > def.MaxBytes {
		return resourceDetail{}, fmt.Errorf("size_limit_exceeded: %s", rel)
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return resourceDetail{}, err
	}
	if IsBinary(data) {
		return resourceDetail{}, resourceDenied("binary files are not allowed")
	}
	return buildResourceDetail(rel, data, info.Size(), def)
}

func buildResourceDetail(rel string, data []byte, size int64, def ResourceDefinition) (resourceDetail, error) {
	detail := resourceDetail{
		Path:        rel,
		ContentType: resourceContentType(rel),
		Extension:   strings.TrimPrefix(path.Ext(rel), "."),
		Size:        size,
	}
	if hasMode(def.Modes, "raw") || hasMode(def.Modes, "raw_yaml") || hasMode(def.Modes, "raw_markdown") {
		detail.Raw = string(data)
	}
	if hasMode(def.Modes, "parsed_yaml") {
		parsed, err := parseYAML(data)
		if err != nil {
			return resourceDetail{}, resourceParseFailed(err)
		}
		detail.Parsed = parsed
	}
	return detail, nil
}

func resolveResourcePath(base string, def ResourceDefinition, requested string) (string, string, error) {
	rel, err := normalizeResourcePath(requested)
	if err != nil {
		return "", "", err
	}
	if !resourcePathAllowed(rel, def, "") {
		return "", "", resourceDenied("resource path is not allowed")
	}
	resolved, err := ValidatePath(base, rel)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return "", "", os.ErrNotExist
		}
		return "", "", err
	}
	return resolved, rel, nil
}

func normalizeResourcePath(requested string) (string, error) {
	cleaned := path.Clean(filepath.ToSlash(requested))
	if cleaned == "." || cleaned == "" {
		return "", resourceDenied("path_required")
	}
	if path.IsAbs(cleaned) || strings.Contains(cleaned, "\x00") {
		return "", resourceDenied("invalid_path")
	}
	for _, part := range strings.Split(cleaned, "/") {
		if part == ".." {
			return "", resourceDenied("invalid_path")
		}
	}
	return cleaned, nil
}

func resourcePathAllowed(rel string, def ResourceDefinition, prefix string) bool {
	if prefix != "" && !strings.HasPrefix(rel, strings.TrimPrefix(path.Clean(prefix), "/")) {
		return false
	}
	return extensionAllowed(rel, def.Extensions) && includeAllowed(rel, def.Include)
}

func includeAllowed(rel string, patterns []string) bool {
	if len(patterns) == 0 {
		return true
	}
	for _, pattern := range patterns {
		if resourcePatternMatches(pattern, rel) {
			return true
		}
	}
	return false
}

func resourcePatternMatches(pattern, rel string) bool {
	pattern = filepath.ToSlash(pattern)
	if strings.HasSuffix(pattern, "/**") {
		return strings.HasPrefix(rel, strings.TrimSuffix(pattern, "**"))
	}
	if strings.HasPrefix(pattern, "**/") {
		matched, _ := path.Match(strings.TrimPrefix(pattern, "**/"), path.Base(rel))
		return matched
	}
	matched, _ := path.Match(pattern, rel)
	return matched
}

func extensionAllowed(rel string, allowed []string) bool {
	if len(allowed) == 0 {
		return true
	}
	ext := strings.TrimPrefix(path.Ext(rel), ".")
	for _, candidate := range allowed {
		if strings.TrimPrefix(candidate, ".") == ext {
			return true
		}
	}
	return false
}

func resourceRoot(workspaceRoot, configured string) string {
	if filepath.IsAbs(configured) {
		return configured
	}
	return filepath.Join(workspaceRoot, configured)
}

func resourceRel(base, abs string) (string, error) {
	rel, err := filepath.Rel(base, abs)
	if err != nil {
		return "", err
	}
	return filepath.ToSlash(rel), nil
}

func resourceCategory(rel string, rules []ResourceCategoryRule) string {
	for _, rule := range rules {
		if resourcePatternMatches(rule.Pattern, rel) {
			return rule.Category
		}
	}
	dir := path.Dir(rel)
	if dir == "." {
		return "root"
	}
	return strings.Split(dir, "/")[0]
}

func resourceContentType(rel string) string {
	if strings.HasSuffix(rel, ".yaml") || strings.HasSuffix(rel, ".yml") {
		return "application/x-yaml"
	}
	if ct := mime.TypeByExtension(path.Ext(rel)); ct != "" {
		return ct
	}
	return "text/plain; charset=utf-8"
}

func parseYAML(data []byte) (interface{}, error) {
	var parsed interface{}
	if err := yaml.Unmarshal(data, &parsed); err != nil {
		return nil, err
	}
	return parsed, nil
}

func hasMode(modes []string, target string) bool {
	for _, mode := range modes {
		if mode == target {
			return true
		}
	}
	return false
}

func resourceJSON(name string, signal core.Signal, value interface{}) core.Result {
	data, err := json.Marshal(value)
	if err != nil {
		return commandError(name, fmt.Errorf("encode resource output: %w", err))
	}
	return core.Result{Output: string(data), Signal: signal, CommandName: name}
}

func resourceError(name string, err error) core.Result {
	switch {
	case os.IsNotExist(err):
		return core.Result{Signal: SignalDocumentMissing, Output: "resource not found", CommandName: name}
	case strings.HasPrefix(err.Error(), "parse_failed:"):
		return core.Result{Signal: SignalDocumentParseFailed, Output: err.Error(), CommandName: name}
	case strings.HasPrefix(err.Error(), "resource_denied:"):
		return core.Result{Signal: SignalDocumentResourceDenied, Output: err.Error(), CommandName: name}
	default:
		return commandError(name, err)
	}
}

func resourceDenied(reason string) error {
	return fmt.Errorf("resource_denied: %s", reason)
}

func resourceParseFailed(err error) error {
	return fmt.Errorf("parse_failed: %w", err)
}

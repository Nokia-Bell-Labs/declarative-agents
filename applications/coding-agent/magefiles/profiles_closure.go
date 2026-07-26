// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	profileManifestPath  = "agents/application.yaml"
	profileOutputEnv     = "CODING_AGENT_PROFILES_OUTPUT"
	defaultProfileOutput = "build/profiles"
)

type applicationProfileManifest struct {
	SchemaVersion int    `yaml:"schema_version"`
	Application   string `yaml:"application"`
	AgentProfiles struct {
		CompatibleRelease string             `yaml:"compatible_release"`
		References        []profileReference `yaml:"references"`
	} `yaml:"agent_profiles"`
	Runtime struct {
		MountPath             string `yaml:"mount_path"`
		ImageContainsProfiles bool   `yaml:"image_contains_profiles"`
	} `yaml:"runtime"`
	Deployment struct {
		ServingProfiles []profileReference `yaml:"serving_profiles"`
	} `yaml:"deployment"`
}

type profileReference struct {
	Role        string `yaml:"role"`
	Source      string `yaml:"source"`
	RuntimePath string `yaml:"runtime_path"`
}

type packageSource struct {
	Kind              string `yaml:"kind"`
	CompatibleRelease string `yaml:"compatible_release"`
	Release           string `yaml:"release,omitempty"`
	Revision          string `yaml:"revision"`
	Dirty             bool   `yaml:"dirty,omitempty"`
}

type packagedProfileManifest struct {
	SchemaVersion int                `yaml:"schema_version"`
	Application   string             `yaml:"application"`
	Source        packageSource      `yaml:"source"`
	MountPath     string             `yaml:"mount_path"`
	Profiles      []profileReference `yaml:"profiles"`
	Files         []string           `yaml:"files"`
}

type closureAsset struct {
	source string
	dest   string
}

type profileClosure struct {
	sourceRoot string
	assets     map[string]string
	pending    []closureAsset
}

// Package assembles the coding application's complete, deterministic profile
// closure under build/profiles (or CODING_AGENT_PROFILES_OUTPUT).
func Package() error {
	appRoot, err := os.Getwd()
	if err != nil {
		return err
	}
	appRoot, err = filepath.Abs(filepath.Clean(appRoot))
	if err != nil {
		return fmt.Errorf("coding-agent package: resolve application root: %w", err)
	}
	manifestPath := filepath.Join(appRoot, filepath.FromSlash(profileManifestPath))
	manifest, err := readApplicationProfileManifest(manifestPath)
	if err != nil {
		return err
	}
	profilesRoot, err := resolveCatalogRoot("coding-agent package", appRoot)
	if err != nil {
		return err
	}
	output := envOrDefault(profileOutputEnv, filepath.Join(appRoot, filepath.FromSlash(defaultProfileOutput)))
	source, err := inspectPackageSource(profilesRoot, manifest.AgentProfiles.CompatibleRelease)
	if err != nil {
		return err
	}
	shards, err := packageServingDeployment(appRoot, profilesRoot, output, manifest, source)
	if err != nil {
		return err
	}
	fmt.Printf("packaged %d serving role shards in %s from %s %s\n", len(shards), output, source.Kind, source.Revision)
	return nil
}

func readApplicationProfileManifest(filename string) (applicationProfileManifest, error) {
	var manifest applicationProfileManifest
	data, err := os.ReadFile(filename)
	if err != nil {
		return manifest, fmt.Errorf("read application profile manifest: %w", err)
	}
	if err := yaml.Unmarshal(data, &manifest); err != nil {
		return manifest, fmt.Errorf("parse application profile manifest: %w", err)
	}
	if manifest.SchemaVersion != 1 {
		return manifest, fmt.Errorf("unsupported application profile manifest schema %d", manifest.SchemaVersion)
	}
	if strings.TrimSpace(manifest.Application) == "" {
		return manifest, errors.New("application profile manifest has no application")
	}
	if !isCompatibleProfileRelease(manifest.AgentProfiles.CompatibleRelease) {
		return manifest, fmt.Errorf(
			"compatible_release %q must match applications/catalog/v0.* or agent-profiles/v0.*",
			manifest.AgentProfiles.CompatibleRelease,
		)
	}
	if manifest.Runtime.MountPath != "/profiles" {
		return manifest, fmt.Errorf("runtime mount_path %q must preserve the mounted-profile contract at /profiles", manifest.Runtime.MountPath)
	}
	if manifest.Runtime.ImageContainsProfiles {
		return manifest, errors.New("runtime image must remain profile-free")
	}
	if len(manifest.AgentProfiles.References) == 0 {
		return manifest, errors.New("application profile manifest has no profile references")
	}
	if err := validateProfileReferences(manifest.AgentProfiles.References, "profile"); err != nil {
		return manifest, err
	}
	if err := validateServingReferences(manifest.Deployment.ServingProfiles); err != nil {
		return manifest, err
	}
	return manifest, nil
}

func validateProfileReferences(references []profileReference, kind string) error {
	roles := make(map[string]struct{}, len(references))
	for _, ref := range references {
		if strings.TrimSpace(ref.Role) == "" {
			return fmt.Errorf("%s reference has no role", kind)
		}
		if _, exists := roles[ref.Role]; exists {
			return fmt.Errorf("duplicate %s role %q", kind, ref.Role)
		}
		roles[ref.Role] = struct{}{}
		if _, err := cleanRelativeProfilePath(ref.Source); err != nil {
			return fmt.Errorf("%s role %s source: %w", kind, ref.Role, err)
		}
		if _, err := cleanRelativeProfilePath(ref.RuntimePath); err != nil {
			return fmt.Errorf("%s role %s runtime_path: %w", kind, ref.Role, err)
		}
	}
	return nil
}

func validateServingReferences(references []profileReference) error {
	if len(references) == 0 {
		return errors.New("application profile manifest has no deployment serving profiles")
	}
	if err := validateProfileReferences(references, "serving profile"); err != nil {
		return err
	}
	want := map[string]profileReference{
		"planner": {
			Source: "agents/serving/planner/profile.yaml", RuntimePath: "applications/coding-agent/planner/profile.yaml",
		},
		"executor": {
			Source: "agents/serving/executor/profile.yaml", RuntimePath: "applications/coding-agent/executor/profile.yaml",
		},
		"critic": {
			Source: "agents/serving/critic/profile.yaml", RuntimePath: "applications/coding-agent/critic/profile.yaml",
		},
	}
	for _, ref := range references {
		expected, exists := want[ref.Role]
		if !exists {
			return fmt.Errorf("unsupported deployment serving role %q", ref.Role)
		}
		if ref.Source != expected.Source {
			return fmt.Errorf("serving profile role %s source %q, want %q", ref.Role, ref.Source, expected.Source)
		}
		if ref.RuntimePath != expected.RuntimePath {
			return fmt.Errorf("serving profile role %s runtime_path %q, want %q", ref.Role, ref.RuntimePath, expected.RuntimePath)
		}
		delete(want, ref.Role)
	}
	if len(want) > 0 {
		missing := make([]string, 0, len(want))
		for role := range want {
			missing = append(missing, role)
		}
		sort.Strings(missing)
		return fmt.Errorf("deployment serving profiles missing roles: %s", strings.Join(missing, ", "))
	}
	return nil
}

func isCompatibleProfileRelease(version string) bool {
	for _, prefix := range []string{"applications/catalog/v0.", "agent-profiles/v0."} {
		suffix := strings.TrimPrefix(version, prefix)
		if strings.HasPrefix(version, prefix) && suffix != "" && !strings.ContainsAny(suffix, `/\`) {
			return true
		}
	}
	return false
}

func assembleProfileClosure(manifest applicationProfileManifest, sourceRoot, output string, source packageSource) ([]string, error) {
	closure := &profileClosure{
		sourceRoot: filepath.Clean(sourceRoot),
		assets:     make(map[string]string),
	}
	for _, ref := range manifest.AgentProfiles.References {
		sourcePath, err := cleanRelativeProfilePath(ref.Source)
		if err != nil {
			return nil, fmt.Errorf("profile role %s source: %w", ref.Role, err)
		}
		destPath, err := cleanRelativeProfilePath(ref.RuntimePath)
		if err != nil {
			return nil, fmt.Errorf("profile role %s runtime_path: %w", ref.Role, err)
		}
		if err := closure.enqueue(sourcePath, destPath); err != nil {
			return nil, fmt.Errorf("profile role %s: %w", ref.Role, err)
		}
	}
	if err := closure.resolve(); err != nil {
		return nil, err
	}
	return writeProfilePackage(manifest, closure.sourceRoot, closure.assets, output, source)
}

func (c *profileClosure) enqueue(source, dest string) error {
	cleanSource, err := cleanRelativeProfilePath(source)
	if err != nil {
		return err
	}
	cleanDest, err := cleanRelativeProfilePath(dest)
	if err != nil {
		return err
	}
	if previous, exists := c.assets[cleanDest]; exists {
		if previous != cleanSource {
			return fmt.Errorf("conflicting destination %s from %s and %s", cleanDest, previous, cleanSource)
		}
		return nil
	}
	c.assets[cleanDest] = cleanSource
	c.pending = append(c.pending, closureAsset{source: cleanSource, dest: cleanDest})
	return nil
}

func (c *profileClosure) resolve() error {
	for len(c.pending) > 0 {
		asset := c.pending[0]
		c.pending = c.pending[1:]
		sourcePath, err := secureSourcePath(c.sourceRoot, asset.source)
		if err != nil {
			return err
		}
		info, err := os.Lstat(sourcePath)
		if err != nil {
			return fmt.Errorf("dangling profile reference %s: %w", asset.source, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("profile asset %s is a symlink; symlinks are not packageable", asset.source)
		}
		if info.IsDir() {
			if err := c.expandDirectory(asset); err != nil {
				return err
			}
			delete(c.assets, asset.dest)
			continue
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("profile asset %s is not a regular file", asset.source)
		}
		if strings.HasSuffix(asset.source, ".yaml") || strings.HasSuffix(asset.source, ".yml") {
			if err := c.resolveYAMLReferences(asset, sourcePath); err != nil {
				return err
			}
		}
	}
	return nil
}

func (c *profileClosure) expandDirectory(directory closureAsset) error {
	root, err := secureSourcePath(c.sourceRoot, directory.source)
	if err != nil {
		return err
	}
	return filepath.WalkDir(root, func(filename string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if filename == root {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("profile directory %s contains symlink %s", directory.source, filename)
		}
		if entry.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, filename)
		if err != nil {
			return err
		}
		return c.enqueue(
			path.Join(directory.source, filepath.ToSlash(rel)),
			path.Join(directory.dest, filepath.ToSlash(rel)),
		)
	})
}

func (c *profileClosure) resolveYAMLReferences(asset closureAsset, filename string) error {
	data, err := os.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("read profile asset %s: %w", asset.source, err)
	}
	var document yaml.Node
	if err := yaml.Unmarshal(data, &document); err != nil {
		return fmt.Errorf("parse profile asset %s: %w", asset.source, err)
	}
	refs, err := runtimeYAMLReferences(&document)
	if err != nil {
		return fmt.Errorf("%s: %w", asset.source, err)
	}
	for _, ref := range refs {
		if isExternalCoreReference(ref) {
			continue
		}
		if filepath.IsAbs(ref) || strings.HasPrefix(ref, "/") {
			return fmt.Errorf("%s: absolute profile reference is not allowed: %s", asset.source, ref)
		}
		if strings.ContainsAny(ref, "*?[") {
			return fmt.Errorf("%s: runtime profile reference may not be a glob: %s", asset.source, ref)
		}
		var sourceRef, destRef string
		if strings.HasPrefix(filepath.ToSlash(ref), "agents/") {
			sourceRef = filepath.ToSlash(ref)
			destRef = filepath.ToSlash(ref)
		} else {
			sourceRef = path.Join(path.Dir(asset.source), filepath.ToSlash(ref))
			destRef = path.Join(path.Dir(asset.dest), filepath.ToSlash(ref))
		}
		if err := c.enqueue(sourceRef, destRef); err != nil {
			return fmt.Errorf("%s references %s: %w", asset.source, ref, err)
		}
	}
	return nil
}

func runtimeYAMLReferences(document *yaml.Node) ([]string, error) {
	if document == nil || len(document.Content) == 0 {
		return nil, nil
	}
	var refs []string
	root := document.Content[0]
	if root.Kind != yaml.MappingNode {
		return nil, nil
	}
	topLevelKeys := map[string]bool{
		"machine": true, "tools": true, "tool_declarations": true,
		"tool_config_dirs": true, "rest_definitions": true, "rest_config_dirs": true,
	}
	nestedKeys := map[string]bool{
		"profile": true, "point_machine": true, "point_tools": true,
		"point_tool_declarations": true, "includes": true,
	}
	var visit func(*yaml.Node, int) error
	visit = func(node *yaml.Node, depth int) error {
		if node.Kind != yaml.MappingNode {
			return nil
		}
		for i := 0; i+1 < len(node.Content); i += 2 {
			key, value := node.Content[i].Value, node.Content[i+1]
			isReference := nestedKeys[key] || (depth == 0 && topLevelKeys[key])
			if isReference {
				if values, ok := referenceScalars(value); ok {
					for _, value := range values {
						if looksLikeRuntimePath(value) {
							refs = append(refs, value)
						}
					}
				}
			}
			if err := visitNestedMappings(value, depth+1, visit); err != nil {
				return err
			}
		}
		return nil
	}
	if err := visit(root, 0); err != nil {
		return nil, err
	}
	sort.Strings(refs)
	return compactStrings(refs), nil
}

func visitNestedMappings(node *yaml.Node, depth int, visit func(*yaml.Node, int) error) error {
	switch node.Kind {
	case yaml.MappingNode:
		return visit(node, depth)
	case yaml.SequenceNode, yaml.DocumentNode:
		for _, child := range node.Content {
			if err := visitNestedMappings(child, depth, visit); err != nil {
				return err
			}
		}
	}
	return nil
}

func referenceScalars(node *yaml.Node) ([]string, bool) {
	switch node.Kind {
	case yaml.ScalarNode:
		return []string{strings.TrimSpace(node.Value)}, true
	case yaml.SequenceNode:
		values := make([]string, 0, len(node.Content))
		for _, child := range node.Content {
			if child.Kind != yaml.ScalarNode {
				return nil, false
			}
			values = append(values, strings.TrimSpace(child.Value))
		}
		return values, true
	default:
		return nil, false
	}
}

func looksLikeRuntimePath(value string) bool {
	return strings.HasSuffix(value, ".yaml") || strings.HasSuffix(value, ".yml") ||
		strings.HasPrefix(value, "/opt/agent-core/")
}

func isExternalCoreReference(ref string) bool {
	return strings.HasPrefix(filepath.ToSlash(filepath.Clean(ref)), "/opt/agent-core/")
}

func cleanRelativeProfilePath(value string) (string, error) {
	if strings.TrimSpace(value) == "" {
		return "", errors.New("profile path is empty")
	}
	if strings.Contains(value, `\`) {
		return "", fmt.Errorf("profile path uses a backslash: %s", value)
	}
	if filepath.IsAbs(value) || strings.HasPrefix(value, "/") {
		return "", fmt.Errorf("profile path must be relative: %s", value)
	}
	clean := path.Clean(filepath.ToSlash(value))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("profile path escapes catalog root: %s", value)
	}
	return clean, nil
}

func secureSourcePath(root, rel string) (string, error) {
	clean, err := cleanRelativeProfilePath(rel)
	if err != nil {
		return "", err
	}
	filename := filepath.Join(root, filepath.FromSlash(clean))
	relative, err := filepath.Rel(root, filename)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("profile path escapes catalog root: %s", rel)
	}
	return filename, nil
}

func writeProfilePackage(manifest applicationProfileManifest, sourceRoot string, assets map[string]string, output string, source packageSource) ([]string, error) {
	parent := filepath.Dir(output)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return nil, fmt.Errorf("create profile package parent: %w", err)
	}
	stage, err := os.MkdirTemp(parent, ".profiles-stage-*")
	if err != nil {
		return nil, fmt.Errorf("create profile package staging directory: %w", err)
	}
	defer func() { _ = os.RemoveAll(stage) }()

	destinations := make([]string, 0, len(assets))
	for dest := range assets {
		destinations = append(destinations, dest)
	}
	sort.Strings(destinations)
	for _, dest := range destinations {
		sourcePath, err := secureSourcePath(sourceRoot, assets[dest])
		if err != nil {
			return nil, err
		}
		if err := copyProfileAsset(sourcePath, filepath.Join(stage, filepath.FromSlash(dest))); err != nil {
			return nil, err
		}
	}
	metadata := packagedProfileManifest{
		SchemaVersion: 1,
		Application:   manifest.Application,
		Source:        source,
		MountPath:     manifest.Runtime.MountPath,
		Profiles:      append([]profileReference(nil), manifest.AgentProfiles.References...),
		Files:         destinations,
	}
	data, err := yaml.Marshal(metadata)
	if err != nil {
		return nil, fmt.Errorf("marshal package manifest: %w", err)
	}
	if err := os.WriteFile(filepath.Join(stage, "package-manifest.yaml"), data, 0o644); err != nil {
		return nil, fmt.Errorf("write package manifest: %w", err)
	}
	if err := os.RemoveAll(output); err != nil {
		return nil, fmt.Errorf("replace profile package: %w", err)
	}
	if err := os.Rename(stage, output); err != nil {
		return nil, fmt.Errorf("publish profile package: %w", err)
	}
	return destinations, nil
}

func copyProfileAsset(source, destination string) error {
	info, err := os.Stat(source)
	if err != nil {
		return fmt.Errorf("stat profile asset %s: %w", source, err)
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return fmt.Errorf("create profile asset directory: %w", err)
	}
	data, err := os.ReadFile(source)
	if err != nil {
		return fmt.Errorf("read profile asset %s: %w", source, err)
	}
	if err := os.WriteFile(destination, data, info.Mode().Perm()); err != nil {
		return fmt.Errorf("write profile asset %s: %w", destination, err)
	}
	return nil
}

func inspectPackageSource(root, compatibleRelease string) (packageSource, error) {
	source := packageSource{
		Kind:              "checkout",
		CompatibleRelease: compatibleRelease,
		Revision:          "unversioned-checkout",
	}
	revision, err := gitOutput(root, "rev-parse", "HEAD")
	if err != nil {
		return source, nil
	}
	source.Revision = revision
	status, statusErr := gitOutput(root, "status", "--porcelain", "--", ".")
	source.Dirty = statusErr == nil && status != ""
	exact, tagErr := gitOutput(root, "describe", "--tags", "--exact-match", "--match", compatibleRelease, "HEAD")
	if tagErr == nil && exact == compatibleRelease && !source.Dirty {
		source.Kind = "release"
		source.Release = compatibleRelease
	}
	return source, nil
}

func gitOutput(directory string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", directory}, args...)...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

func compactStrings(values []string) []string {
	if len(values) < 2 {
		return values
	}
	result := values[:1]
	for _, value := range values[1:] {
		if value != result[len(result)-1] {
			result = append(result, value)
		}
	}
	return result
}

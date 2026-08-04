// Copyright (c) 2026 Nokia. All rights reserved.

// Package appmanifest validates application composition manifests and resolves
// their package-time profile closure. It deliberately knows no application
// role names and does not implement agent-core runtime semantics.
package appmanifest

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

const SchemaVersion = 1

var (
	identifierPattern    = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
	compatibilityPattern = regexp.MustCompile(`^(?:(?:applications/catalog|agent-profiles)/)?v0\.[A-Za-z0-9][A-Za-z0-9._-]*$`)
	templatePattern      = regexp.MustCompile(`\$\{[^}\r\n]+\}`)
	knownStatuses        = stringSet("planned", "audit_only", "partial", "dependency_gated", "implemented", "not_applicable")
	knownCapabilities    = stringSet("runnable_module", "managed_service", "packaged", "helm_managed", "kind_demo", "ui")
)

// Options names the two ownership boundaries used by validation and closure.
// ApplicationRoot defaults to the parent of the manifest's agents directory.
// CatalogRoot is mandatory when the manifest declares a catalog-owned root.
type Options struct {
	ApplicationRoot   string
	CatalogRoot       string
	MaxFiles          int
	RuntimeOwnedRoots []string
}

// Manifest is the application-owned composition authority.
type Manifest struct {
	SchemaVersion int                   `yaml:"schema_version"`
	Application   string                `yaml:"application"`
	Ownership     string                `yaml:"ownership"`
	ModuleStatus  string                `yaml:"module_status"`
	Capabilities  map[string]Capability `yaml:"capabilities"`
	Roots         []Root                `yaml:"roots"`
	Runtime       Runtime               `yaml:"runtime"`
	Deployment    Deployment            `yaml:"deployment,omitempty"`
	UI            UI                    `yaml:"ui,omitempty"`
}

type Capability struct {
	Status   string   `yaml:"status"`
	Evidence []string `yaml:"evidence,omitempty"`
}

// Root is one direct catalog or application-local profile entry.
type Root struct {
	ID                string `yaml:"id"`
	Ownership         string `yaml:"ownership"`
	Source            string `yaml:"source"`
	RuntimePath       string `yaml:"runtime_path"`
	CompatibleRelease string `yaml:"compatible_release,omitempty"`
	Planned           bool   `yaml:"planned,omitempty"`
}

type Runtime struct {
	MountPath             string `yaml:"mount_path"`
	ImageContainsProfiles bool   `yaml:"image_contains_profiles,omitempty"`
}

type Deployment struct {
	Entries []DeploymentEntry `yaml:"entries,omitempty"`
}

type DeploymentEntry struct {
	ID          string `yaml:"id"`
	Root        string `yaml:"root"`
	Workload    string `yaml:"workload,omitempty"`
	ProfilePath string `yaml:"profile_path,omitempty"`
	MountPath   string `yaml:"mount_path,omitempty"`
}

type UI struct {
	Assets []UIAsset `yaml:"assets,omitempty"`
}

type UIAsset struct {
	ID             string `yaml:"id"`
	Owner          string `yaml:"owner"`
	Source         string `yaml:"source"`
	RuntimePath    string `yaml:"runtime_path"`
	RESTDefinition string `yaml:"rest_definition"`
	SharedTokens   string `yaml:"shared_tokens"`
}

// Load parses one strict schema-versioned YAML document and validates all
// declared paths against their ownership roots.
func Load(filename string, options Options) (Manifest, error) {
	var manifest Manifest
	info, err := os.Lstat(filename)
	if err != nil {
		return manifest, fmt.Errorf("read application manifest: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return manifest, fmt.Errorf("application manifest must be a regular non-symlink file: %s", filename)
	}
	data, err := os.ReadFile(filename)
	if err != nil {
		return manifest, fmt.Errorf("read application manifest: %w", err)
	}
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&manifest); err != nil {
		return manifest, fmt.Errorf("parse application manifest: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err == nil {
		return manifest, errors.New("application manifest must contain exactly one YAML document")
	} else if err != io.EOF {
		return manifest, fmt.Errorf("parse application manifest: %w", err)
	}
	if options.ApplicationRoot == "" {
		options.ApplicationRoot = filepath.Dir(filepath.Dir(filename))
	}
	if err := manifest.validate(options); err != nil {
		return manifest, err
	}
	return manifest, nil
}

func (manifest *Manifest) validate(options Options) error {
	if manifest.SchemaVersion != SchemaVersion {
		return fmt.Errorf("unsupported application manifest schema %d", manifest.SchemaVersion)
	}
	if !identifierPattern.MatchString(manifest.Application) {
		return fmt.Errorf("application %q must be a lower-kebab identifier", manifest.Application)
	}
	if strings.TrimSpace(manifest.Ownership) == "" {
		return errors.New("application manifest has no ownership")
	}
	if !knownStatuses[manifest.ModuleStatus] {
		return fmt.Errorf("unknown module_status %q", manifest.ModuleStatus)
	}
	if err := manifest.validateCapabilities(); err != nil {
		return err
	}
	if manifest.Runtime.ImageContainsProfiles {
		return errors.New("runtime image must remain profile-free")
	}
	if manifest.Runtime.MountPath != "" {
		mount, err := cleanRuntimeMount(manifest.Runtime.MountPath)
		if err != nil {
			return fmt.Errorf("runtime mount_path: %w", err)
		}
		manifest.Runtime.MountPath = mount
	}

	appRoot, err := absoluteRoot(options.ApplicationRoot, "application")
	if err != nil {
		return err
	}
	if filepath.Base(appRoot) != manifest.Application {
		return fmt.Errorf("application %q does not match application root %q", manifest.Application, filepath.Base(appRoot))
	}
	var catalogRoot string
	if options.CatalogRoot != "" {
		catalogRoot, err = absoluteRoot(options.CatalogRoot, "catalog")
		if err != nil {
			return err
		}
	}

	rootIDs := make(map[string]*Root, len(manifest.Roots))
	runtimePaths := make(map[string]string, len(manifest.Roots)+len(manifest.UI.Assets))
	for index := range manifest.Roots {
		root := &manifest.Roots[index]
		if !identifierPattern.MatchString(root.ID) {
			return fmt.Errorf("root id %q must be a lower-kebab identifier", root.ID)
		}
		if _, duplicate := rootIDs[root.ID]; duplicate {
			return fmt.Errorf("duplicate root id %q", root.ID)
		}
		if root.Ownership != "catalog" && root.Ownership != "local" {
			return fmt.Errorf("root %s has unknown ownership %q", root.ID, root.Ownership)
		}
		root.Source, err = cleanRelative(root.Source)
		if err != nil {
			return fmt.Errorf("root %s source: %w", root.ID, err)
		}
		parts := strings.Split(root.Source, "/")
		if len(parts) < 3 || parts[0] != "agents" || !profileFilename(parts[len(parts)-1]) {
			return fmt.Errorf("root %s source must name a profile below agents/<actor>", root.ID)
		}
		root.RuntimePath, err = cleanRelative(root.RuntimePath)
		if err != nil {
			return fmt.Errorf("root %s runtime_path: %w", root.ID, err)
		}
		if previous := runtimePaths[root.RuntimePath]; previous != "" {
			return fmt.Errorf("duplicate normalized runtime path %q for %s and root %s", root.RuntimePath, previous, root.ID)
		}
		runtimePaths[root.RuntimePath] = "root " + root.ID
		ownerRoot := appRoot
		if root.Ownership == "catalog" {
			if catalogRoot == "" {
				return fmt.Errorf("root %s requires an explicit catalog root", root.ID)
			}
			if !compatibilityPattern.MatchString(root.CompatibleRelease) {
				return fmt.Errorf("root %s has invalid compatible_release %q", root.ID, root.CompatibleRelease)
			}
			ownerRoot = catalogRoot
		} else {
			if root.CompatibleRelease != "" {
				return fmt.Errorf("local root %s must not declare compatible_release", root.ID)
			}
			if parts[1] == "tests" {
				return fmt.Errorf("local root %s source must be below agents/<actor>", root.ID)
			}
		}
		if !root.Planned {
			if _, err := securePath(ownerRoot, root.Source, false); err != nil {
				return fmt.Errorf("root %s source: %w", root.ID, err)
			}
		}
		rootIDs[root.ID] = root
	}
	if manifest.ModuleStatus != "audit_only" && len(manifest.Roots) == 0 {
		return errors.New("application manifest has no direct roots")
	}
	if err := manifest.validateDeployment(rootIDs); err != nil {
		return err
	}
	if err := manifest.validateUI(appRoot, catalogRoot, rootIDs, runtimePaths); err != nil {
		return err
	}
	if capabilityActive(manifest.Capabilities["packaged"]) && manifest.Runtime.MountPath == "" {
		return errors.New("packaged capability requires runtime mount_path")
	}
	return nil
}

func (manifest *Manifest) validateCapabilities() error {
	if len(manifest.Capabilities) == 0 {
		return errors.New("application manifest has no capabilities")
	}
	for name, capability := range manifest.Capabilities {
		if !knownCapabilities[name] {
			return fmt.Errorf("unknown capability %q", name)
		}
		if !knownStatuses[capability.Status] {
			return fmt.Errorf("capability %s has unknown status %q", name, capability.Status)
		}
		if (capability.Status == "implemented" || capability.Status == "partial" ||
			capability.Status == "dependency_gated") && len(nonempty(capability.Evidence)) == 0 {
			return fmt.Errorf("capability %s status %s requires evidence", name, capability.Status)
		}
	}
	baseline, exists := manifest.Capabilities["runnable_module"]
	if !exists {
		return errors.New("application manifest must declare runnable_module capability")
	}
	if manifest.ModuleStatus == "audit_only" && capabilityActive(baseline) {
		return errors.New("audit_only module cannot claim runnable behavior")
	}
	if capabilityActive(manifest.Capabilities["helm_managed"]) &&
		!capabilityActive(manifest.Capabilities["packaged"]) {
		return errors.New("helm_managed capability requires packaged capability")
	}
	return nil
}

func (manifest *Manifest) validateDeployment(roots map[string]*Root) error {
	seen := make(map[string]bool, len(manifest.Deployment.Entries))
	for index := range manifest.Deployment.Entries {
		entry := &manifest.Deployment.Entries[index]
		if !identifierPattern.MatchString(entry.ID) || seen[entry.ID] {
			return fmt.Errorf("deployment entry id %q is invalid or duplicated", entry.ID)
		}
		seen[entry.ID] = true
		root := roots[entry.Root]
		if root == nil {
			return fmt.Errorf("deployment entry %s refers to undeclared root %q", entry.ID, entry.Root)
		}
		if root.Planned {
			return fmt.Errorf("deployment entry %s refers to planned root %q", entry.ID, entry.Root)
		}
		var err error
		if entry.ProfilePath != "" {
			entry.ProfilePath, err = cleanRelative(entry.ProfilePath)
			if err != nil {
				return fmt.Errorf("deployment entry %s profile_path: %w", entry.ID, err)
			}
		}
		if entry.MountPath != "" {
			entry.MountPath, err = cleanRuntimeMount(entry.MountPath)
			if err != nil {
				return fmt.Errorf("deployment entry %s mount_path: %w", entry.ID, err)
			}
		}
	}
	return nil
}

func (manifest *Manifest) validateUI(appRoot, catalogRoot string, roots map[string]*Root, runtimePaths map[string]string) error {
	if len(manifest.UI.Assets) > 0 {
		ui, declared := manifest.Capabilities["ui"]
		if !declared || ui.Status == "not_applicable" || ui.Status == "planned" {
			return errors.New("UI assets require an applicable ui capability")
		}
	} else if capabilityActive(manifest.Capabilities["ui"]) {
		return errors.New("active ui capability has no declared assets")
	}
	seen := make(map[string]bool, len(manifest.UI.Assets))
	for index := range manifest.UI.Assets {
		asset := &manifest.UI.Assets[index]
		if !identifierPattern.MatchString(asset.ID) || seen[asset.ID] {
			return fmt.Errorf("UI asset id %q is invalid or duplicated", asset.ID)
		}
		seen[asset.ID] = true
		ownerRoot := appRoot
		if asset.Owner != manifest.Application {
			root := roots[asset.Owner]
			if root == nil {
				return fmt.Errorf("UI asset %s has undeclared owner %q", asset.ID, asset.Owner)
			}
			if root.Ownership == "catalog" {
				ownerRoot = catalogRoot
			}
		}
		var err error
		asset.Source, err = cleanRelative(asset.Source)
		if err != nil {
			return fmt.Errorf("UI asset %s source: %w", asset.ID, err)
		}
		asset.RuntimePath, err = cleanRelative(asset.RuntimePath)
		if err != nil {
			return fmt.Errorf("UI asset %s runtime_path: %w", asset.ID, err)
		}
		asset.RESTDefinition, err = cleanRelative(asset.RESTDefinition)
		if err != nil {
			return fmt.Errorf("UI asset %s rest_definition: %w", asset.ID, err)
		}
		if strings.TrimSpace(asset.SharedTokens) == "" {
			return fmt.Errorf("UI asset %s has no shared_tokens policy", asset.ID)
		}
		if previous := runtimePaths[asset.RuntimePath]; previous != "" {
			return fmt.Errorf("duplicate normalized runtime path %q for %s and UI asset %s", asset.RuntimePath, previous, asset.ID)
		}
		runtimePaths[asset.RuntimePath] = "UI asset " + asset.ID
		if _, err := securePath(ownerRoot, asset.Source, true); err != nil {
			return fmt.Errorf("UI asset %s source: %w", asset.ID, err)
		}
		restPath, err := securePath(ownerRoot, asset.RESTDefinition, false)
		if err != nil {
			return fmt.Errorf("UI asset %s rest_definition: %w", asset.ID, err)
		}
		data, err := os.ReadFile(restPath)
		if err != nil {
			return fmt.Errorf("UI asset %s rest_definition: %w", asset.ID, err)
		}
		var document yaml.Node
		if err := yaml.Unmarshal(yamlTemplateSafe(data), &document); err != nil {
			return fmt.Errorf("UI asset %s rest_definition: %w", asset.ID, err)
		}
		if !mappingKeyExists(&document, "static_assets") {
			return fmt.Errorf("UI asset %s REST definition has no static_assets binding", asset.ID)
		}
	}
	return nil
}

func yamlTemplateSafe(data []byte) []byte {
	return templatePattern.ReplaceAll(data, []byte("manifest_value"))
}

func capabilityActive(capability Capability) bool {
	return capability.Status == "implemented" || capability.Status == "partial" ||
		capability.Status == "dependency_gated"
}

func cleanRelative(value string) (string, error) {
	if strings.TrimSpace(value) == "" {
		return "", errors.New("path is empty")
	}
	if strings.Contains(value, `\`) || path.IsAbs(value) || isWindowsPath(value) {
		return "", fmt.Errorf("path must be a portable relative path: %s", value)
	}
	for _, component := range strings.Split(value, "/") {
		if component == ".." {
			return "", fmt.Errorf("path contains traversal: %s", value)
		}
	}
	clean := path.Clean(value)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("path escapes its ownership root: %s", value)
	}
	return clean, nil
}

func cleanRuntimeMount(value string) (string, error) {
	if strings.Contains(value, `\`) || !path.IsAbs(value) || isWindowsPath(value) {
		return "", fmt.Errorf("mount must be an absolute runtime path: %s", value)
	}
	for _, component := range strings.Split(value, "/") {
		if component == ".." {
			return "", fmt.Errorf("mount contains traversal: %s", value)
		}
	}
	return path.Clean(value), nil
}

func securePath(root, relative string, allowDirectory bool) (string, error) {
	candidate := filepath.Join(root, filepath.FromSlash(relative))
	if err := pathWithin(root, candidate); err != nil {
		return "", err
	}
	current := root
	for _, component := range strings.Split(relative, "/") {
		current = filepath.Join(current, component)
		info, err := os.Lstat(current)
		if err != nil {
			return "", fmt.Errorf("dangling path %s: %w", relative, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("path %s contains symlink %s", relative, component)
		}
	}
	info, err := os.Lstat(candidate)
	if err != nil {
		return "", fmt.Errorf("dangling path %s: %w", relative, err)
	}
	if (!allowDirectory && !info.Mode().IsRegular()) || (allowDirectory && !info.Mode().IsRegular() && !info.IsDir()) {
		return "", fmt.Errorf("path %s is not a packageable file or directory", relative)
	}
	return candidate, nil
}

func pathWithin(root, candidate string) error {
	relative, err := filepath.Rel(filepath.Clean(root), filepath.Clean(candidate))
	if err != nil || filepath.IsAbs(relative) || relative == ".." ||
		strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return fmt.Errorf("path escapes ownership root %s", root)
	}
	return nil
}

func absoluteRoot(root, name string) (string, error) {
	if strings.TrimSpace(root) == "" {
		return "", fmt.Errorf("%s root is required", name)
	}
	absolute, err := filepath.Abs(filepath.Clean(root))
	if err != nil {
		return "", fmt.Errorf("resolve %s root: %w", name, err)
	}
	info, err := os.Stat(absolute)
	if err != nil || !info.IsDir() {
		return "", fmt.Errorf("%s root is not a directory: %s", name, root)
	}
	return absolute, nil
}

func mappingKeyExists(node *yaml.Node, key string) bool {
	if node == nil {
		return false
	}
	if node.Kind == yaml.MappingNode {
		for index := 0; index+1 < len(node.Content); index += 2 {
			if node.Content[index].Value == key {
				return true
			}
		}
	}
	for _, child := range node.Content {
		if mappingKeyExists(child, key) {
			return true
		}
	}
	return false
}

func stringSet(values ...string) map[string]bool {
	result := make(map[string]bool, len(values))
	for _, value := range values {
		result[value] = true
	}
	return result
}

func nonempty(values []string) []string {
	result := values[:0]
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			result = append(result, value)
		}
	}
	sort.Strings(result)
	return result
}

func isWindowsPath(value string) bool {
	return len(value) >= 2 && ((value[0] >= 'A' && value[0] <= 'Z') ||
		(value[0] >= 'a' && value[0] <= 'z')) && value[1] == ':'
}

func profileFilename(name string) bool {
	return name == "profile.yaml" ||
		(strings.HasPrefix(name, "profile-") && strings.HasSuffix(name, ".yaml")) ||
		(strings.HasSuffix(name, "-profile.yaml"))
}

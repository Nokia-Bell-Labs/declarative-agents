// Copyright (c) 2026 Nokia. All rights reserved.

package appmanifest

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

const defaultMaxClosureFiles = 10000

// Inventory is a deterministic description of the resolved package closure.
type Inventory struct {
	SchemaVersion int              `yaml:"schema_version"`
	Application   string           `yaml:"application"`
	Roots         []RootProvenance `yaml:"roots"`
	Files         []InventoryFile  `yaml:"files"`
}

type RootProvenance struct {
	ID                string `yaml:"id"`
	Ownership         string `yaml:"ownership"`
	Source            string `yaml:"source"`
	RuntimePath       string `yaml:"runtime_path"`
	CompatibleRelease string `yaml:"compatible_release,omitempty"`
}

type InventoryFile struct {
	Source      string   `yaml:"source"`
	RuntimePath string   `yaml:"runtime_path"`
	Checksum    string   `yaml:"checksum"`
	Roots       []string `yaml:"roots"`
}

type closureItem struct {
	ownership string
	source    string
	runtime   string
	rootID    string
	lineage   []string
}

type closureResolver struct {
	applicationRoot string
	catalogRoot     string
	runtimeOwned    []string
	maxFiles        int
	files           map[string]InventoryFile
	visited         map[string]bool
	queue           []closureItem
}

// Resolve validates a manifest, traverses every executable root and declared UI
// asset, and returns a source-independent, deterministically sorted inventory.
func Resolve(manifest Manifest, options Options) (Inventory, error) {
	if options.ApplicationRoot == "" {
		return Inventory{}, errors.New("closure resolution requires an explicit application root")
	}
	if err := manifest.validate(options); err != nil {
		return Inventory{}, err
	}
	appRoot, _ := absoluteRoot(options.ApplicationRoot, "application")
	var catalogRoot string
	if options.CatalogRoot != "" {
		catalogRoot, _ = absoluteRoot(options.CatalogRoot, "catalog")
	}
	maxFiles := options.MaxFiles
	if maxFiles <= 0 {
		maxFiles = defaultMaxClosureFiles
	}
	runtimeOwned := append([]string{"/opt/agent-core"}, options.RuntimeOwnedRoots...)
	for index, root := range runtimeOwned {
		runtimeOwned[index] = path.Clean(filepath.ToSlash(root))
	}
	sort.Strings(runtimeOwned)

	resolver := &closureResolver{
		applicationRoot: appRoot,
		catalogRoot:     catalogRoot,
		runtimeOwned:    runtimeOwned,
		maxFiles:        maxFiles,
		files:           make(map[string]InventoryFile),
		visited:         make(map[string]bool),
	}
	inventory := Inventory{SchemaVersion: SchemaVersion, Application: manifest.Application}
	for _, root := range manifest.Roots {
		if root.Planned {
			continue
		}
		inventory.Roots = append(inventory.Roots, RootProvenance{
			ID: root.ID, Ownership: root.Ownership, Source: logicalSource(root.Ownership, root.Source),
			RuntimePath: root.RuntimePath, CompatibleRelease: root.CompatibleRelease,
		})
		resolver.queue = append(resolver.queue, closureItem{
			ownership: root.Ownership, source: root.Source, runtime: root.RuntimePath,
			rootID: root.ID, lineage: []string{sourceKey(root.Ownership, root.Source)},
		})
	}
	rootByID := make(map[string]Root, len(manifest.Roots))
	for _, root := range manifest.Roots {
		rootByID[root.ID] = root
	}
	for _, asset := range manifest.UI.Assets {
		ownership := "local"
		if owner, exists := rootByID[asset.Owner]; exists && owner.Ownership == "catalog" {
			ownership = "catalog"
		}
		id := "ui-" + asset.ID
		inventory.Roots = append(inventory.Roots, RootProvenance{
			ID: id, Ownership: ownership, Source: logicalSource(ownership, asset.Source),
			RuntimePath: asset.RuntimePath,
		})
		resolver.queue = append(resolver.queue, closureItem{
			ownership: ownership, source: asset.Source, runtime: asset.RuntimePath,
			rootID: id, lineage: []string{sourceKey(ownership, asset.Source)},
		})
	}
	if err := resolver.run(); err != nil {
		return Inventory{}, err
	}
	for _, deployment := range manifest.Deployment.Entries {
		target := deployment.ProfilePath
		if target == "" {
			target = rootByID[deployment.Root].RuntimePath
		}
		file, exists := resolver.files[target]
		if !exists || !contains(file.Roots, deployment.Root) {
			return Inventory{}, fmt.Errorf("deployment entry %s profile_path %s is not reachable from root %s",
				deployment.ID, target, deployment.Root)
		}
	}
	sort.Slice(inventory.Roots, func(i, j int) bool { return inventory.Roots[i].ID < inventory.Roots[j].ID })
	for _, file := range resolver.files {
		inventory.Files = append(inventory.Files, file)
	}
	sort.Slice(inventory.Files, func(i, j int) bool {
		if inventory.Files[i].RuntimePath != inventory.Files[j].RuntimePath {
			return inventory.Files[i].RuntimePath < inventory.Files[j].RuntimePath
		}
		return inventory.Files[i].Source < inventory.Files[j].Source
	})
	return inventory, nil
}

func (resolver *closureResolver) run() error {
	maxWork := resolver.maxFiles * 16
	for len(resolver.queue) > 0 {
		if len(resolver.visited) >= maxWork {
			return fmt.Errorf("profile closure exceeded bounded work limit %d", maxWork)
		}
		item := resolver.queue[0]
		resolver.queue = resolver.queue[1:]
		visitKey := sourceKey(item.ownership, item.source) + "\x00" + item.runtime + "\x00" + item.rootID
		if resolver.visited[visitKey] {
			continue
		}
		resolver.visited[visitKey] = true
		filename, err := resolver.sourcePath(item.ownership, item.source, true)
		if err != nil {
			return fmt.Errorf("%s: %w", logicalSource(item.ownership, item.source), err)
		}
		info, err := os.Lstat(filename)
		if err != nil {
			return fmt.Errorf("dangling reference %s: %w", logicalSource(item.ownership, item.source), err)
		}
		if info.IsDir() {
			if err := resolver.expandDirectory(item, filename); err != nil {
				return err
			}
			continue
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("closure source %s is not a regular file", logicalSource(item.ownership, item.source))
		}
		data, err := os.ReadFile(filename)
		if err != nil {
			return fmt.Errorf("read closure source %s: %w", logicalSource(item.ownership, item.source), err)
		}
		if err := resolver.addFile(item, data); err != nil {
			return err
		}
		if strings.HasSuffix(item.source, ".yaml") || strings.HasSuffix(item.source, ".yml") {
			if err := resolver.resolveYAML(item, data); err != nil {
				return err
			}
		}
	}
	return nil
}

func (resolver *closureResolver) expandDirectory(item closureItem, directory string) error {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return fmt.Errorf("read closure directory %s: %w", logicalSource(item.ownership, item.source), err)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	for _, entry := range entries {
		source := path.Join(item.source, entry.Name())
		runtime := path.Join(item.runtime, entry.Name())
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("closure directory %s contains symlink %s", logicalSource(item.ownership, item.source), entry.Name())
		}
		resolver.queue = append(resolver.queue, closureItem{
			ownership: item.ownership, source: source, runtime: runtime,
			rootID: item.rootID, lineage: append([]string(nil), item.lineage...),
		})
	}
	return nil
}

func (resolver *closureResolver) addFile(item closureItem, data []byte) error {
	sum := sha256.Sum256(data)
	checksum := "sha256:" + hex.EncodeToString(sum[:])
	source := logicalSource(item.ownership, item.source)
	if previous, exists := resolver.files[item.runtime]; exists {
		if previous.Checksum != checksum {
			return fmt.Errorf("conflicting destination %s from %s and %s", item.runtime, previous.Source, source)
		}
		if source < previous.Source {
			previous.Source = source
		}
		previous.Roots = appendUniqueSorted(previous.Roots, item.rootID)
		resolver.files[item.runtime] = previous
		return nil
	}
	if len(resolver.files) >= resolver.maxFiles {
		return fmt.Errorf("profile closure exceeds maximum of %d files", resolver.maxFiles)
	}
	resolver.files[item.runtime] = InventoryFile{
		Source: source, RuntimePath: item.runtime, Checksum: checksum, Roots: []string{item.rootID},
	}
	return nil
}

func (resolver *closureResolver) resolveYAML(item closureItem, data []byte) error {
	var document yaml.Node
	if err := yaml.Unmarshal(yamlTemplateSafe(data), &document); err != nil {
		return fmt.Errorf("parse closure source %s: %w", logicalSource(item.ownership, item.source), err)
	}
	references := yamlReferences(&document)
	for _, reference := range references {
		if resolver.isRuntimeOwned(reference) {
			continue
		}
		if path.IsAbs(filepath.ToSlash(reference)) || isWindowsPath(reference) {
			return fmt.Errorf("%s contains disallowed absolute reference %s",
				logicalSource(item.ownership, item.source), reference)
		}
		if strings.ContainsAny(reference, "*?[") {
			return fmt.Errorf("%s contains unbounded glob reference %s",
				logicalSource(item.ownership, item.source), reference)
		}
		ownership, source, runtime, err := resolveReference(item, reference)
		if err != nil {
			return fmt.Errorf("%s references %s: %w", logicalSource(item.ownership, item.source), reference, err)
		}
		key := sourceKey(ownership, source)
		if contains(item.lineage, key) {
			// REST machine_request endpoints and their request profiles commonly
			// refer back to one another. A repeated REST edge adds no file and is
			// bounded by the visited set; cycles outside REST remain invalid.
			currentBase, targetBase := path.Base(item.source), path.Base(source)
			currentIsREST := currentBase == "rest.yaml" || strings.HasSuffix(currentBase, "-rest.yaml")
			targetIsREST := targetBase == "rest.yaml" || strings.HasSuffix(targetBase, "-rest.yaml")
			if currentIsREST || targetIsREST {
				continue
			}
			return fmt.Errorf("cyclic closure reference: %s -> %s", strings.Join(item.lineage, " -> "), key)
		}
		lineage := append(append([]string(nil), item.lineage...), key)
		resolver.queue = append(resolver.queue, closureItem{
			ownership: ownership, source: source, runtime: runtime,
			rootID: item.rootID, lineage: lineage,
		})
	}
	return nil
}

func (resolver *closureResolver) sourcePath(ownership, relative string, allowDirectory bool) (string, error) {
	root := resolver.applicationRoot
	if ownership == "catalog" {
		if resolver.catalogRoot == "" {
			return "", errors.New("catalog reference requires an explicit catalog root")
		}
		root = resolver.catalogRoot
	}
	return securePath(root, relative, allowDirectory)
}

func (resolver *closureResolver) isRuntimeOwned(reference string) bool {
	clean := path.Clean(filepath.ToSlash(reference))
	for _, root := range resolver.runtimeOwned {
		if clean == root || strings.HasPrefix(clean, root+"/") {
			return true
		}
	}
	return false
}

func resolveReference(item closureItem, reference string) (string, string, string, error) {
	portable := filepath.ToSlash(strings.TrimSpace(reference))
	if portable == "" || strings.Contains(portable, `\`) {
		return "", "", "", errors.New("reference is empty or non-portable")
	}
	if strings.HasPrefix(portable, "agents/") {
		source, err := cleanJoined("", portable)
		if err != nil {
			return "", "", "", err
		}
		return "catalog", source, source, nil
	}
	source, err := cleanJoined(path.Dir(item.source), portable)
	if err != nil {
		return "", "", "", err
	}
	runtime, err := cleanJoined(path.Dir(item.runtime), portable)
	if err != nil {
		return "", "", "", err
	}
	return item.ownership, source, runtime, nil
}

func cleanJoined(base, reference string) (string, error) {
	clean := path.Clean(path.Join(base, reference))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || path.IsAbs(clean) {
		return "", fmt.Errorf("reference escapes ownership root: %s", reference)
	}
	return clean, nil
}

func yamlReferences(document *yaml.Node) []string {
	var references []string
	var visit func(*yaml.Node, int, []string)
	visit = func(node *yaml.Node, depth int, ancestors []string) {
		switch node.Kind {
		case yaml.DocumentNode, yaml.SequenceNode:
			for _, child := range node.Content {
				visit(child, depth, ancestors)
			}
		case yaml.MappingNode:
			for index := 0; index+1 < len(node.Content); index += 2 {
				key, value := node.Content[index].Value, node.Content[index+1]
				topLevelField := depth == 0 && stringSet(
					"machine", "tools", "tool_declarations", "tool_config_dirs",
					"rest_definitions", "rest_config_dirs")[key]
				pathField := topLevelField ||
					stringSet("profile", "subject_profile", "machine", "point_machine",
						"point_tools", "point_tool_declarations", "includes")[key] ||
					(key == "path" && contains(ancestors, "openapi"))
				if pathField {
					allowDirectory := topLevelField && (key == "tool_config_dirs" || key == "rest_config_dirs")
					references = append(references, referenceStrings(value, allowDirectory)...)
				}
				visit(value, depth+1, append(ancestors, key))
			}
		}
	}
	visit(document, 0, nil)
	sort.Strings(references)
	result := references[:0]
	for _, reference := range references {
		reference = strings.TrimSpace(reference)
		if reference == "" || strings.Contains(reference, "${") {
			continue
		}
		if len(result) == 0 || result[len(result)-1] != reference {
			result = append(result, reference)
		}
	}
	return result
}

func referenceStrings(node *yaml.Node, allowDirectory bool) []string {
	var values []string
	if node.Kind == yaml.ScalarNode {
		values = []string{node.Value}
	} else if node.Kind == yaml.SequenceNode {
		for _, child := range node.Content {
			if child.Kind == yaml.ScalarNode {
				values = append(values, child.Value)
			}
		}
	}
	result := values[:0]
	for _, value := range values {
		clean := strings.TrimSpace(value)
		if allowDirectory || strings.HasSuffix(clean, ".yaml") || strings.HasSuffix(clean, ".yml") ||
			strings.HasPrefix(filepath.ToSlash(clean), "/opt/agent-core/") {
			result = append(result, value)
		}
	}
	return result
}

func logicalSource(ownership, source string) string {
	if ownership == "catalog" {
		return "catalog/" + source
	}
	return "application/" + source
}

func sourceKey(ownership, source string) string { return ownership + ":" + source }

func appendUniqueSorted(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	values = append(values, value)
	sort.Strings(values)
	return values
}

func contains(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}

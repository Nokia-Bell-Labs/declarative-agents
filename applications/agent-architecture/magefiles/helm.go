// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/magefiles/appmanifest"
	"gopkg.in/yaml.v3"
)

const (
	configMapPayloadLimit    = 900 * 1024
	preparedManifestFilename = "prepared-manifest.yaml"
	// The documentation-curator's browser UI and the catalog docs tree exceed the
	// profile ConfigMap payload limit, so instead of a per-app runtime image
	// (retired, GH-1368) they are packed into one deterministic tar.gz, split into
	// sub-1-MiB shards under curatorAssetsDir, and shipped as ConfigMaps the
	// curator init container concatenates and unpacks into /work.
	curatorAssetsDir        = "curator-assets"
	curatorAssetShardPrefix = "part-"
	// 512 KiB of raw gzip per shard base64-encodes to ~699 KiB, well under the
	// ~1 MiB ConfigMap object limit even with the object's metadata.
	curatorAssetShardSize = 512 * 1024
)

type preparedRole struct {
	Role              string   `yaml:"role"`
	Root              string   `yaml:"root"`
	Path              string   `yaml:"path"`
	Profile           string   `yaml:"profile"`
	Ownership         string   `yaml:"ownership"`
	Source            string   `yaml:"source"`
	CompatibleRelease string   `yaml:"compatible_release,omitempty"`
	AssetRoots        []string `yaml:"asset_roots,omitempty"`
	Checksum          string   `yaml:"checksum"`
	Files             []string `yaml:"files"`
}

type preparedManifest struct {
	SchemaVersion         int                   `yaml:"schema_version"`
	Application           string                `yaml:"application"`
	CompositionManifest   string                `yaml:"composition_manifest"`
	MountPath             string                `yaml:"mount_path"`
	ConfigMapPayloadLimit int                   `yaml:"config_map_payload_limit_bytes"`
	Roles                 []preparedRole        `yaml:"roles"`
	ExternalAssetRoots    []string              `yaml:"external_asset_roots,omitempty"`
	Closure               appmanifest.Inventory `yaml:"closure"`
}

type preparedRolePlan struct {
	role  preparedRole
	files []appmanifest.InventoryFile
}

type compositionPlan struct {
	manifest           appmanifest.Manifest
	inventory          appmanifest.Inventory
	roles              []preparedRolePlan
	externalAssetRoots []string
}

// HelmPrepare resolves all deployment entries from agents/application.yaml and
// atomically stages their shared appmanifest closures under helm/profiles,
// writing checksum and provenance metadata the package target validates
// (srd001 R2.1, R2.2).
func HelmPrepare() error {
	resolved, err := resolveRootsFromWorkingDirectory()
	if err != nil {
		return err
	}
	chartRoot := filepath.Join(resolved.Application, "helm")
	if err := prepareChartProfiles(resolved.Application, resolved.Catalog, chartRoot); err != nil {
		return err
	}
	// Curator UI shards are no longer staged into the chart: they are provisioned
	// out-of-release by the deploy/test tooling (GH-1402), so HelmPrepare only
	// stages the profile closures the chart carries.
	fmt.Printf("prepared manifest-declared profile closures from %s\n", resolved.Catalog)
	return nil
}

// prepareCuratorAssets packs the documentation-curator's browser UI and the
// catalog docs tree into one deterministic tar.gz and splits it into sub-1-MiB
// shards under chartRoot/curator-assets, atomically swapping the shard directory
// so a failed staging never leaves a partial asset set. The deploy/test tooling
// (provisionCuratorUIShards) reads these shards and creates one out-of-release
// ConfigMap per shard (GH-1402); the curator init container concatenates the
// mounted shards and unpacks them into /work (GH-1368; replaces the baked
// /opt/curator-ui).
func prepareCuratorAssets(applicationRoot, catalogRoot, chartRoot string) error {
	archive, err := buildCuratorAssetArchive(applicationRoot, catalogRoot)
	if err != nil {
		return err
	}
	destination := filepath.Join(chartRoot, curatorAssetsDir)
	stage, err := os.MkdirTemp(chartRoot, ".curator-assets-stage-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(stage) }()
	for index, offset := 0, 0; offset < len(archive); index++ {
		end := offset + curatorAssetShardSize
		if end > len(archive) {
			end = len(archive)
		}
		name := fmt.Sprintf("%s%03d", curatorAssetShardPrefix, index)
		if err := os.WriteFile(filepath.Join(stage, name), archive[offset:end], 0o644); err != nil {
			return fmt.Errorf("write curator asset shard %s: %w", name, err)
		}
		offset = end
	}
	if err := os.RemoveAll(destination); err != nil {
		return err
	}
	if err := os.Rename(stage, destination); err != nil {
		return fmt.Errorf("publish curator assets: %w", err)
	}
	return nil
}

// buildCuratorAssetArchive returns a deterministic gzip tarball of the
// manifest-declared assets moved out of oversized ConfigMaps. Both UI and
// catalog documentation paths come only from the shared appmanifest closure.
func buildCuratorAssetArchive(applicationRoot, catalogRoot string) ([]byte, error) {
	plan, err := resolveCompositionPlan(applicationRoot, catalogRoot)
	if err != nil {
		return nil, err
	}
	external := make(map[string]bool, len(plan.externalAssetRoots))
	for _, id := range plan.externalAssetRoots {
		external[id] = true
	}
	type archiveAsset struct {
		name   string
		source string
	}
	var assets []archiveAsset
	for _, file := range plan.inventory.Files {
		include := false
		for _, root := range file.Roots {
			if external[root] {
				include = true
				break
			}
		}
		if include {
			source, err := inventorySourcePath(applicationRoot, catalogRoot, file.Source)
			if err != nil {
				return nil, err
			}
			assets = append(assets, archiveAsset{name: file.PackagePath, source: source})
		}
	}
	sort.Slice(assets, func(i, j int) bool { return assets[i].name < assets[j].name })
	if len(assets) == 0 {
		return nil, fmt.Errorf("manifest declares no external curator assets")
	}

	var buffer bytes.Buffer
	gz := gzip.NewWriter(&buffer)
	writer := tar.NewWriter(gz)
	for _, asset := range assets {
		data, err := os.ReadFile(asset.source)
		if err != nil {
			return nil, fmt.Errorf("read curator asset %s: %w", asset.name, err)
		}
		header := &tar.Header{
			Name:     asset.name,
			Mode:     0o644,
			Size:     int64(len(data)),
			Typeflag: tar.TypeReg,
			Format:   tar.FormatGNU,
		}
		if err := writer.WriteHeader(header); err != nil {
			return nil, fmt.Errorf("write curator asset header %s: %w", header.Name, err)
		}
		if _, err := writer.Write(data); err != nil {
			return nil, fmt.Errorf("write curator asset body %s: %w", header.Name, err)
		}
	}
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("close curator asset tar: %w", err)
	}
	if err := gz.Close(); err != nil {
		return nil, fmt.Errorf("close curator asset gzip: %w", err)
	}
	return buffer.Bytes(), nil
}

// prepareChartProfiles resolves the shared application manifest closure, splits
// it only by declared deployment entry, and atomically publishes those derived
// role packages. No Helm-owned role or source inventory participates.
func prepareChartProfiles(applicationRoot, catalogRoot, chartRoot string) error {
	plan, err := resolveCompositionPlan(applicationRoot, catalogRoot)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(chartRoot, 0o755); err != nil {
		return err
	}
	stage, err := os.MkdirTemp(chartRoot, ".profiles-stage-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(stage) }()

	roles := make([]preparedRole, 0, len(plan.roles))
	for _, rolePlan := range plan.roles {
		roleRoot := filepath.Join(stage, rolePlan.role.Path)
		for _, file := range rolePlan.files {
			source, err := inventorySourcePath(applicationRoot, catalogRoot, file.Source)
			if err != nil {
				return err
			}
			destination := filepath.Join(roleRoot, filepath.FromSlash(file.RuntimePath))
			if err := copyRegularFile(source, destination); err != nil {
				return fmt.Errorf("stage role %s file %s: %w", rolePlan.role.Role, file.RuntimePath, err)
			}
		}
		files, err := regularRelativeFiles(roleRoot)
		if err != nil {
			return err
		}
		if len(files) == 0 {
			return fmt.Errorf("staged %s closure is empty", rolePlan.role.Role)
		}
		checksum, err := roleClosureChecksum(roleRoot, files)
		if err != nil {
			return err
		}
		role := rolePlan.role
		role.Checksum = checksum
		role.Files = files
		roles = append(roles, role)
	}
	manifest := preparedManifest{
		SchemaVersion:         1,
		Application:           plan.manifest.Application,
		CompositionManifest:   "agents/application.yaml",
		MountPath:             plan.manifest.Runtime.MountPath,
		ConfigMapPayloadLimit: configMapPayloadLimit,
		Roles:                 roles,
		ExternalAssetRoots:    plan.externalAssetRoots,
		Closure:               plan.inventory,
	}
	if err := writeYAML(filepath.Join(stage, preparedManifestFilename), manifest); err != nil {
		return err
	}
	destination := filepath.Join(chartRoot, "profiles")
	if err := os.RemoveAll(destination); err != nil {
		return err
	}
	if err := os.Rename(stage, destination); err != nil {
		return fmt.Errorf("publish staged profiles: %w", err)
	}
	return nil
}

func resolveCompositionPlan(applicationRoot, catalogRoot string) (compositionPlan, error) {
	options := appmanifest.Options{ApplicationRoot: applicationRoot, CatalogRoot: catalogRoot}
	manifest, err := appmanifest.Load(
		filepath.Join(applicationRoot, "agents", "application.yaml"), options)
	if err != nil {
		return compositionPlan{}, err
	}
	inventory, err := appmanifest.Resolve(manifest, options)
	if err != nil {
		return compositionPlan{}, fmt.Errorf("resolve application composition: %w", err)
	}
	plan := compositionPlan{manifest: manifest, inventory: inventory}
	roots := make(map[string]appmanifest.Root, len(manifest.Roots))
	for _, root := range manifest.Roots {
		roots[root.ID] = root
	}
	assetRootsByOwner := make(map[string][]string)
	for _, asset := range manifest.UI.Assets {
		assetRootsByOwner[asset.Owner] = append(assetRootsByOwner[asset.Owner], "ui-"+asset.ID)
	}
	for _, asset := range manifest.Package.Assets {
		assetRootsByOwner[asset.Owner] = append(assetRootsByOwner[asset.Owner], "asset-"+asset.ID)
	}
	entries := append([]appmanifest.DeploymentEntry(nil), manifest.Deployment.Entries...)
	sort.Slice(entries, func(i, j int) bool { return entries[i].ID < entries[j].ID })
	for _, entry := range entries {
		root := roots[entry.Root]
		assetRoots := assetRootsByOwner[entry.Root]
		packageRoots := append([]string{entry.Root}, assetRoots...)
		files := inventoryFilesForRoots(inventory, packageRoots)
		if len(files) == 0 {
			return compositionPlan{}, fmt.Errorf(
				"deployment entry %s root %s resolved an empty closure", entry.ID, entry.Root)
		}
		payload, err := inventoryPayload(applicationRoot, catalogRoot, files)
		if err != nil {
			return compositionPlan{}, err
		}
		if payload > configMapPayloadLimit {
			if len(assetRoots) == 0 {
				return compositionPlan{}, fmt.Errorf(
					"deployment entry %s payload %d exceeds limit %d and has no declared assets to externalize",
					entry.ID, payload, configMapPayloadLimit)
			}
			files = excludeInventoryRoots(files, assetRoots)
			if len(files) == 0 {
				return compositionPlan{}, fmt.Errorf(
					"deployment entry %s contains only external package assets", entry.ID)
			}
			payload, err = inventoryPayload(applicationRoot, catalogRoot, files)
			if err != nil {
				return compositionPlan{}, err
			}
			if payload > configMapPayloadLimit {
				return compositionPlan{}, fmt.Errorf(
					"deployment entry %s ConfigMap payload %d exceeds limit %d after externalizing assets",
					entry.ID, payload, configMapPayloadLimit)
			}
			plan.externalAssetRoots = append(plan.externalAssetRoots, assetRoots...)
		}
		profile := entry.ProfilePath
		if profile == "" {
			profile = root.RuntimePath
		}
		source := root.Source
		if root.Ownership == "catalog" {
			source = "catalog/" + source
		} else {
			source = "application/" + source
		}
		plan.roles = append(plan.roles, preparedRolePlan{
			role: preparedRole{
				Role: entry.ID, Root: entry.Root, Path: entry.ID, Profile: profile,
				Ownership: root.Ownership, Source: source,
				CompatibleRelease: root.CompatibleRelease,
				AssetRoots:        append([]string(nil), assetRoots...),
			},
			files: files,
		})
	}
	sort.Strings(plan.externalAssetRoots)
	plan.externalAssetRoots = uniqueStrings(plan.externalAssetRoots)
	return plan, nil
}

func inventoryFilesForRoots(inventory appmanifest.Inventory, roots []string) []appmanifest.InventoryFile {
	var files []appmanifest.InventoryFile
	for _, file := range inventory.Files {
		for _, root := range roots {
			if containsValue(file.Roots, root) {
				files = append(files, file)
				break
			}
		}
	}
	return files
}

func excludeInventoryRoots(files []appmanifest.InventoryFile, excluded []string) []appmanifest.InventoryFile {
	var kept []appmanifest.InventoryFile
	for _, file := range files {
		exclude := false
		for _, root := range excluded {
			if containsValue(file.Roots, root) {
				exclude = true
				break
			}
		}
		if !exclude {
			kept = append(kept, file)
		}
	}
	return kept
}

func inventoryPayload(applicationRoot, catalogRoot string, files []appmanifest.InventoryFile) (int, error) {
	payload := 0
	for _, file := range files {
		source, err := inventorySourcePath(applicationRoot, catalogRoot, file.Source)
		if err != nil {
			return 0, err
		}
		data, err := os.ReadFile(source)
		if err != nil {
			return 0, err
		}
		payload += len(strings.ReplaceAll(file.RuntimePath, "/", "__")) + len(data)
	}
	return payload, nil
}

func inventorySourcePath(applicationRoot, catalogRoot, logical string) (string, error) {
	for prefix, root := range map[string]string{
		"application/": applicationRoot,
		"catalog/":     catalogRoot,
	} {
		if strings.HasPrefix(logical, prefix) {
			relative := strings.TrimPrefix(logical, prefix)
			return filepath.Join(root, filepath.FromSlash(relative)), nil
		}
	}
	return "", fmt.Errorf("inventory source %q has no ownership prefix", logical)
}

func uniqueStrings(values []string) []string {
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

func containsValue(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

// validatePreparedProfiles rejects a staged closure whose manifest is stale,
// whose files were added, removed, or tampered with, or whose ConfigMap payload
// would exceed the single-partition limit (srd001 R2.2).
func validatePreparedProfiles(profilesRoot string) error {
	var manifest preparedManifest
	if err := readStrictYAML(filepath.Join(profilesRoot, preparedManifestFilename), &manifest); err != nil {
		return err
	}
	if manifest.SchemaVersion != 1 || manifest.Application == "" ||
		manifest.CompositionManifest != "agents/application.yaml" ||
		manifest.MountPath == "" || manifest.ConfigMapPayloadLimit != configMapPayloadLimit ||
		manifest.Closure.Application != manifest.Application {
		return fmt.Errorf("prepared manifest has an invalid contract")
	}
	if len(manifest.Roles) == 0 {
		return fmt.Errorf("prepared manifest has no deployment roles")
	}
	closureRoots := make(map[string]appmanifest.RootProvenance, len(manifest.Closure.Roots))
	for _, root := range manifest.Closure.Roots {
		closureRoots[root.ID] = root
	}
	closureFiles := make(map[string]appmanifest.InventoryFile, len(manifest.Closure.Files))
	for _, file := range manifest.Closure.Files {
		closureFiles[file.RuntimePath] = file
	}
	roleNames := make(map[string]bool, len(manifest.Roles))
	for index, role := range manifest.Roles {
		if role.Role == "" || role.Root == "" || role.Path != role.Role ||
			role.Profile == "" || roleNames[role.Role] ||
			(index > 0 && manifest.Roles[index-1].Role >= role.Role) {
			return fmt.Errorf("prepared role %d is stale or malformed: %#v", index, role)
		}
		roleNames[role.Role] = true
		provenance, exists := closureRoots[role.Root]
		if !exists || provenance.Ownership != role.Ownership ||
			provenance.Source != role.Source ||
			provenance.CompatibleRelease != role.CompatibleRelease {
			return fmt.Errorf("prepared role %s provenance does not match closure root %s", role.Role, role.Root)
		}
		if role.Checksum == "" {
			return fmt.Errorf("prepared role %s has no checksum", role.Role)
		}
		if !sort.StringsAreSorted(role.Files) || hasDuplicateStrings(role.Files) {
			return fmt.Errorf("prepared role %s files must be sorted and unique", role.Role)
		}
		roleRoot := filepath.Join(profilesRoot, role.Path)
		actual, err := regularRelativeFiles(roleRoot)
		if err != nil {
			return err
		}
		if !reflect.DeepEqual(actual, role.Files) {
			return fmt.Errorf("prepared role %s files = %v, manifest = %v", role.Role, actual, role.Files)
		}
		for _, path := range actual {
			file, exists := closureFiles[path]
			allowedRoots := append([]string{role.Root}, role.AssetRoots...)
			traceable := false
			for _, root := range allowedRoots {
				if containsValue(file.Roots, root) {
					traceable = true
					break
				}
			}
			if !exists || !traceable {
				return fmt.Errorf("prepared role %s file %s is not traceable to root %s",
					role.Role, path, role.Root)
			}
		}
		checksum, err := roleClosureChecksum(roleRoot, actual)
		if err != nil {
			return err
		}
		if checksum != role.Checksum {
			return fmt.Errorf("prepared role %s checksum mismatch: manifest %s, content %s", role.Role, role.Checksum, checksum)
		}
		payload := 0
		for _, path := range actual {
			data, err := os.ReadFile(filepath.Join(roleRoot, filepath.FromSlash(path)))
			if err != nil {
				return err
			}
			payload += len(strings.ReplaceAll(path, "/", "__")) + len(data)
		}
		if payload > manifest.ConfigMapPayloadLimit {
			return fmt.Errorf("prepared role %s ConfigMap payload %d exceeds limit %d",
				role.Role, payload, manifest.ConfigMapPayloadLimit)
		}
		if role.Profile != "" {
			if _, err := os.Stat(filepath.Join(roleRoot, filepath.FromSlash(role.Profile))); err != nil {
				return fmt.Errorf("prepared role %s is missing its entry profile %s: %w", role.Role, role.Profile, err)
			}
		}
	}
	entries, err := os.ReadDir(profilesRoot)
	if err != nil {
		return err
	}
	var names []string
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	sort.Strings(names)
	want := make([]string, 0, len(manifest.Roles)+1)
	for _, role := range manifest.Roles {
		want = append(want, role.Path)
	}
	want = append(want, preparedManifestFilename)
	sort.Strings(want)
	if !reflect.DeepEqual(names, want) {
		return fmt.Errorf("staged profiles top-level entries = %v, want %v", names, want)
	}
	return nil
}

func roleClosureChecksum(root string, files []string) (string, error) {
	digest := sha256.New()
	for _, path := range files {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))
		if err != nil {
			return "", err
		}
		_, _ = digest.Write([]byte(path))
		_, _ = digest.Write([]byte{0})
		_, _ = digest.Write(data)
		_, _ = digest.Write([]byte{0})
	}
	return fmt.Sprintf("%x", digest.Sum(nil)), nil
}

func regularRelativeFiles(root string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("closure contains symlink %s", path)
		}
		if entry.IsDir() {
			return nil
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("closure contains non-regular file %s", path)
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		files = append(files, filepath.ToSlash(rel))
		return nil
	})
	sort.Strings(files)
	return files, err
}

func writeYAML(filename string, value any) error {
	data, err := yaml.Marshal(value)
	if err != nil {
		return fmt.Errorf("marshal %s: %w", filepath.Base(filename), err)
	}
	if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filename, data, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", filename, err)
	}
	return nil
}

func readStrictYAML(filename string, target any) error {
	file, err := os.Open(filename)
	if err != nil {
		return fmt.Errorf("read %s: %w", filename, err)
	}
	defer func() { _ = file.Close() }()
	decoder := yaml.NewDecoder(file)
	decoder.KnownFields(true)
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("parse %s: %w", filename, err)
	}
	return nil
}

func hasDuplicateStrings(values []string) bool {
	for index := 1; index < len(values); index++ {
		if values[index] == values[index-1] {
			return true
		}
	}
	return false
}

func copyRegularFile(source, destination string) error {
	info, err := os.Stat(source)
	if err != nil {
		return fmt.Errorf("stat %s: %w", source, err)
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return err
	}
	data, err := os.ReadFile(source)
	if err != nil {
		return fmt.Errorf("read %s: %w", source, err)
	}
	if err := os.WriteFile(destination, data, info.Mode().Perm()); err != nil {
		return fmt.Errorf("write %s: %w", destination, err)
	}
	return nil
}

func copyDirTree(source, destination string) error {
	return filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("source tree contains symlink %s", path)
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, rel)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("source tree contains non-regular file %s", path)
		}
		return copyRegularFile(path, target)
	})
}

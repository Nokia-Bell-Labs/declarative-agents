// Copyright (c) 2026 Nokia. All rights reserved.

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

	"gopkg.in/yaml.v3"
)

const (
	profilesMountPath        = "/profiles"
	curatorProfileRuntime    = "agents/knowledge-manager/documentation-curator/profile.yaml"
	collectorProfileRuntime  = "agents/collector/profile.yaml"
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

// curatorAssetTree is one source directory staged into the curator asset archive
// under a workspace-relative prefix. arcPrefix mirrors the paths the retired
// image baked under /opt/curator-ui so untarring into --directory /work resolves
// the curator profile's ui/*/dist static_assets roots and its docs/ document
// root (GH-1261, GH-1293).
type curatorAssetTree struct {
	arcPrefix string
	source    string
}

func curatorAssetSources(catalogRoot string) []curatorAssetTree {
	return []curatorAssetTree{
		{
			arcPrefix: "agents/knowledge-manager/documentation-curator/ui",
			source:    filepath.Join(catalogRoot, "agents", "knowledge-manager", "documentation-curator", "ui"),
		},
		{arcPrefix: "docs", source: filepath.Join(catalogRoot, "docs")},
	}
}

// helmRoleSpecs names the two catalog-owned closures the chart mounts. Order is
// the sorted order the prepared manifest and its validator expect.
var helmRoleSpecs = []roleSpec{
	{role: "collector", sourceRel: "agents/collector", profileRuntime: collectorProfileRuntime, uiSubdir: "ui/dist"},
	{role: "curator", sourceRel: "agents/knowledge-manager/documentation-curator", profileRuntime: curatorProfileRuntime},
}

type roleSpec struct {
	role           string
	sourceRel      string
	profileRuntime string
	// uiSubdir, when set, is a subdirectory under sourceRel whose tree is staged
	// recursively alongside the top-level closure files. The collector serves its
	// trace UI from ui/dist (srd020 R7); it fits the ConfigMap payload limit, so
	// it ships in the closure rather than being skipped like larger UI trees.
	uiSubdir string
}

type preparedRole struct {
	Role     string   `yaml:"role"`
	Path     string   `yaml:"path"`
	Profile  string   `yaml:"profile"`
	Checksum string   `yaml:"checksum"`
	Files    []string `yaml:"files"`
}

type preparedManifest struct {
	SchemaVersion         int            `yaml:"schema_version"`
	Application           string         `yaml:"application"`
	MountPath             string         `yaml:"mount_path"`
	ConfigMapPayloadLimit int            `yaml:"config_map_payload_limit_bytes"`
	Roles                 []preparedRole `yaml:"roles"`
}

// HelmPrepare regenerates the catalog-owned curator and collector profile
// closures and atomically stages them as the chart's only profile source under
// helm/profiles, writing a checksum-bearing manifest the package target
// validates (srd001 R2.1, R2.2).
func HelmPrepare() error {
	resolved, err := resolveRootsFromWorkingDirectory()
	if err != nil {
		return err
	}
	chartRoot := filepath.Join(resolved.Application, "helm")
	if err := prepareChartProfiles(resolved.Catalog, chartRoot); err != nil {
		return err
	}
	if err := stageApplierProfile(resolved.Application, chartRoot); err != nil {
		return err
	}
	// Curator UI shards are no longer staged into the chart: they are provisioned
	// out-of-release by the deploy/test tooling (GH-1402), so HelmPrepare only
	// stages the profile closures the chart carries.
	fmt.Printf("prepared curator and collector profile closures from %s\n", resolved.Catalog)
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
func prepareCuratorAssets(catalogRoot, chartRoot string) error {
	archive, err := buildCuratorAssetArchive(catalogRoot)
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

// buildCuratorAssetArchive returns a deterministic gzip tarball of the curator
// asset trees. Files are emitted in sorted order with fixed metadata and the
// gzip header carries no name or timestamp, so identical inputs produce identical
// bytes; GNU headers carry the long asset paths busybox tar reads on extraction.
func buildCuratorAssetArchive(catalogRoot string) ([]byte, error) {
	var buffer bytes.Buffer
	gz := gzip.NewWriter(&buffer)
	writer := tar.NewWriter(gz)
	for _, tree := range curatorAssetSources(catalogRoot) {
		files, err := regularRelativeFiles(tree.source)
		if err != nil {
			return nil, fmt.Errorf("read curator asset tree %s: %w", tree.source, err)
		}
		if len(files) == 0 {
			return nil, fmt.Errorf("curator asset tree %s is empty", tree.source)
		}
		for _, rel := range files {
			data, err := os.ReadFile(filepath.Join(tree.source, filepath.FromSlash(rel)))
			if err != nil {
				return nil, fmt.Errorf("read curator asset %s: %w", rel, err)
			}
			header := &tar.Header{
				Name:     tree.arcPrefix + "/" + rel,
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
	}
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("close curator asset tar: %w", err)
	}
	if err := gz.Close(); err != nil {
		return nil, fmt.Errorf("close curator asset gzip: %w", err)
	}
	return buffer.Bytes(), nil
}

// stageApplierProfile stages the application-owned applier profile
// (srd002-applier) into the chart alongside the catalog-owned curator and
// collector closures: a flat family of files mounted at /profiles/agents/applier.
// The applier is not a catalog-referenced role and is not regenerated from the
// catalog, so it never enters the prepared manifest's role set; like the two
// catalog closures it is a profile the chart carries under profiles/, and the
// applier.yaml template renders it only when applier.enabled is set.
func stageApplierProfile(appRoot, chartRoot string) error {
	source := filepath.Join(appRoot, "agents", "applier")
	destination := filepath.Join(chartRoot, "profiles", "applier", "agents", "applier")
	if err := os.MkdirAll(destination, 0o755); err != nil {
		return fmt.Errorf("stage applier profile: %w", err)
	}
	entries, err := os.ReadDir(source)
	if err != nil {
		return fmt.Errorf("read applier profile: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		data, err := os.ReadFile(filepath.Join(source, entry.Name()))
		if err != nil {
			return fmt.Errorf("read applier %s: %w", entry.Name(), err)
		}
		if err := os.WriteFile(filepath.Join(destination, entry.Name()), data, info.Mode().Perm()&fs.ModePerm); err != nil {
			return fmt.Errorf("write applier %s: %w", entry.Name(), err)
		}
	}
	return nil
}

// prepareChartProfiles stages every role closure into a temporary directory and
// swaps it into helm/profiles in one rename, so a failed staging never leaves a
// partial closure the chart would mount.
func prepareChartProfiles(catalogRoot, chartRoot string) error {
	if err := os.MkdirAll(chartRoot, 0o755); err != nil {
		return err
	}
	stage, err := os.MkdirTemp(chartRoot, ".profiles-stage-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(stage) }()

	roles := make([]preparedRole, 0, len(helmRoleSpecs))
	for _, spec := range helmRoleSpecs {
		source := filepath.Join(catalogRoot, filepath.FromSlash(spec.sourceRel))
		destination := filepath.Join(stage, spec.role, filepath.FromSlash(spec.sourceRel))
		if err := stageTopLevelFiles(source, destination, spec.role); err != nil {
			return err
		}
		if spec.uiSubdir != "" {
			uiSource := filepath.Join(source, filepath.FromSlash(spec.uiSubdir))
			uiDestination := filepath.Join(destination, filepath.FromSlash(spec.uiSubdir))
			if err := stageSubtree(uiSource, uiDestination, spec.role); err != nil {
				return err
			}
		}
		files, err := regularRelativeFiles(filepath.Join(stage, spec.role))
		if err != nil {
			return err
		}
		if len(files) == 0 {
			return fmt.Errorf("staged %s closure is empty", spec.role)
		}
		checksum, err := roleClosureChecksum(filepath.Join(stage, spec.role), files)
		if err != nil {
			return err
		}
		roles = append(roles, preparedRole{
			Role:     spec.role,
			Path:     spec.role,
			Profile:  spec.profileRuntime,
			Checksum: checksum,
			Files:    files,
		})
	}
	manifest := preparedManifest{
		SchemaVersion:         1,
		Application:           "agent-architecture",
		MountPath:             profilesMountPath,
		ConfigMapPayloadLimit: configMapPayloadLimit,
		Roles:                 roles,
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

// stageTopLevelFiles copies the regular files directly under source into
// destination. Directories (the curator and collector UI trees) are skipped:
// the mounted closure carries only the boot profile the runtime needs, not the
// multi-megabyte browser assets that would overrun a ConfigMap.
func stageTopLevelFiles(source, destination, role string) error {
	entries, err := os.ReadDir(source)
	if err != nil {
		return fmt.Errorf("read %s closure %s: %w", role, source, err)
	}
	if err := os.MkdirAll(destination, 0o755); err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("%s closure entry %s is a symlink", role, entry.Name())
		}
		if !info.Mode().IsRegular() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(source, entry.Name()))
		if err != nil {
			return fmt.Errorf("read %s %s: %w", role, entry.Name(), err)
		}
		if err := os.WriteFile(filepath.Join(destination, entry.Name()), data, info.Mode().Perm()); err != nil {
			return fmt.Errorf("write %s %s: %w", role, entry.Name(), err)
		}
	}
	return nil
}

// stageSubtree copies the regular files under source into destination,
// preserving the tree shape. It stages a served UI's built assets (srd020 R7)
// into a role closure; symlinks are rejected the same way stageTopLevelFiles
// rejects them so the archive carries no links.
func stageSubtree(source, destination, role string) error {
	return filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, rel)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("%s closure entry %s is a symlink", role, rel)
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read %s %s: %w", role, rel, err)
		}
		return os.WriteFile(target, data, info.Mode().Perm())
	})
}

// validatePreparedProfiles rejects a staged closure whose manifest is stale,
// whose files were added, removed, or tampered with, or whose ConfigMap payload
// would exceed the single-partition limit (srd001 R2.2).
func validatePreparedProfiles(profilesRoot string) error {
	var manifest preparedManifest
	if err := readStrictYAML(filepath.Join(profilesRoot, preparedManifestFilename), &manifest); err != nil {
		return err
	}
	if manifest.SchemaVersion != 1 || manifest.Application != "agent-architecture" ||
		manifest.MountPath != profilesMountPath || manifest.ConfigMapPayloadLimit != configMapPayloadLimit {
		return fmt.Errorf("prepared manifest has an invalid contract")
	}
	if len(manifest.Roles) != len(helmRoleSpecs) {
		return fmt.Errorf("prepared manifest has %d roles, want %d", len(manifest.Roles), len(helmRoleSpecs))
	}
	for index, spec := range helmRoleSpecs {
		role := manifest.Roles[index]
		if role.Role != spec.role || role.Path != spec.role || role.Profile != spec.profileRuntime {
			return fmt.Errorf("prepared role %d is stale or malformed: %#v", index, role)
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
	// The applier profile is application-owned and staged alongside the two
	// catalog closures (stageApplierProfile); it carries no manifest role entry,
	// so it appears only as a top-level directory here.
	want := []string{"applier", "collector", "curator", preparedManifestFilename}
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

// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

const configMapPayloadLimit = 900 * 1024

type configMapPartition struct {
	Index     int      `yaml:"index"`
	SizeBytes int      `yaml:"size_bytes"`
	Files     []string `yaml:"files"`
}

type applicationPackageSource struct {
	Revision string `yaml:"revision"`
	Dirty    bool   `yaml:"dirty,omitempty"`
}

type rolePackageManifest struct {
	SchemaVersion     int                      `yaml:"schema_version"`
	Application       string                   `yaml:"application"`
	Role              string                   `yaml:"role"`
	Source            packageSource            `yaml:"source"`
	ApplicationSource applicationPackageSource `yaml:"application_source"`
	MountPath         string                   `yaml:"mount_path"`
	Profile           string                   `yaml:"profile"`
	Files             []string                 `yaml:"files"`
	ConfigMaps        []configMapPartition     `yaml:"config_maps"`
}

type deploymentShard struct {
	Role       string `yaml:"role"`
	Path       string `yaml:"path"`
	Profile    string `yaml:"profile"`
	Manifest   string `yaml:"manifest"`
	ConfigMaps int    `yaml:"config_maps"`
}

type deploymentPackageManifest struct {
	SchemaVersion         int                      `yaml:"schema_version"`
	Application           string                   `yaml:"application"`
	Source                packageSource            `yaml:"source"`
	ApplicationSource     applicationPackageSource `yaml:"application_source"`
	MountPath             string                   `yaml:"mount_path"`
	ImageContainsProfiles bool                     `yaml:"image_contains_profiles"`
	ConfigMapPayloadLimit int                      `yaml:"config_map_payload_limit_bytes"`
	Shards                []deploymentShard        `yaml:"shards"`
}

// PackageValidate assembles disposable role shards and validates every mounted
// serving entry profile with the real agent binary.
func PackageValidate() error {
	roots, err := resolveIntegrationRoots()
	if err != nil {
		return err
	}
	manifest, err := readApplicationProfileManifest(
		filepath.Join(roots.Application, filepath.FromSlash(profileManifestPath)))
	if err != nil {
		return err
	}
	source, err := inspectPackageSource(roots.Profiles, manifest.AgentProfiles.CompatibleRelease)
	if err != nil {
		return err
	}
	stage, err := os.MkdirTemp("", "coding-agent-package-validation-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	output := filepath.Join(stage, "profiles")
	shards, err := packageServingDeployment(
		roots.Application, roots.Profiles, output, manifest, source)
	if err != nil {
		return err
	}
	binary, cleanupBinary, err := buildAgent(roots.Core)
	if err != nil {
		return err
	}
	defer cleanupBinary()
	profiles := make([]string, 0, len(shards))
	for _, shard := range shards {
		profiles = append(profiles,
			filepath.Join(output, shard.Path, filepath.FromSlash(shard.Profile)))
	}
	if err := bootSmokeProfiles(binary, roots.Core, profiles); err != nil {
		return err
	}
	fmt.Printf("package validation passed for %d serving role shards\n", len(shards))
	return nil
}

func packageServingDeployment(
	appRoot, profilesRoot, output string,
	manifest applicationProfileManifest,
	source packageSource,
) ([]deploymentShard, error) {
	applicationSource := inspectApplicationPackageSource(appRoot)
	virtualRoot, cleanupSource, err := stageDeploymentSource(appRoot, profilesRoot)
	if err != nil {
		return nil, err
	}
	defer cleanupSource()

	parent := filepath.Dir(output)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return nil, fmt.Errorf("create deployment package parent: %w", err)
	}
	stage, err := os.MkdirTemp(parent, ".deployment-stage-*")
	if err != nil {
		return nil, fmt.Errorf("create deployment package stage: %w", err)
	}
	defer func() { _ = os.RemoveAll(stage) }()

	references := append([]profileReference(nil), manifest.Deployment.ServingProfiles...)
	sort.Slice(references, func(i, j int) bool { return references[i].Role < references[j].Role })
	shards := make([]deploymentShard, 0, len(references))
	for _, ref := range references {
		closure, err := resolveServingRoleClosure(virtualRoot, ref)
		if err != nil {
			return nil, fmt.Errorf("serving role %s: %w", ref.Role, err)
		}
		roleRoot := filepath.Join(stage, ref.Role)
		files, partitions, err := writeRoleClosure(virtualRoot, roleRoot, closure.assets)
		if err != nil {
			return nil, fmt.Errorf("serving role %s: %w", ref.Role, err)
		}
		roleManifest := rolePackageManifest{
			SchemaVersion:     1,
			Application:       manifest.Application,
			Role:              ref.Role,
			Source:            source,
			ApplicationSource: applicationSource,
			MountPath:         manifest.Runtime.MountPath,
			Profile:           ref.RuntimePath,
			Files:             files,
			ConfigMaps:        partitions,
		}
		manifestPath := filepath.Join("manifests", ref.Role+".yaml")
		if err := writeYAML(filepath.Join(stage, manifestPath), roleManifest); err != nil {
			return nil, err
		}
		shards = append(shards, deploymentShard{
			Role: ref.Role, Path: ref.Role, Profile: ref.RuntimePath,
			Manifest:   filepath.ToSlash(manifestPath),
			ConfigMaps: len(partitions),
		})
	}
	deployment := deploymentPackageManifest{
		SchemaVersion:         1,
		Application:           manifest.Application,
		Source:                source,
		ApplicationSource:     applicationSource,
		MountPath:             manifest.Runtime.MountPath,
		ImageContainsProfiles: manifest.Runtime.ImageContainsProfiles,
		ConfigMapPayloadLimit: configMapPayloadLimit,
		Shards:                shards,
	}
	if err := writeYAML(filepath.Join(stage, "deployment-manifest.yaml"), deployment); err != nil {
		return nil, err
	}
	if err := os.RemoveAll(output); err != nil {
		return nil, fmt.Errorf("replace deployment package: %w", err)
	}
	if err := os.Rename(stage, output); err != nil {
		return nil, fmt.Errorf("publish deployment package: %w", err)
	}
	return shards, nil
}

func inspectApplicationPackageSource(root string) applicationPackageSource {
	source := applicationPackageSource{Revision: "unversioned-checkout"}
	if revision, err := gitOutput(root, "rev-parse", "HEAD"); err == nil {
		source.Revision = revision
	}
	if status, err := gitOutput(root, "status", "--porcelain", "--", "."); err == nil {
		source.Dirty = status != ""
	}
	return source
}

func stageDeploymentSource(appRoot, profilesRoot string) (string, func(), error) {
	stage, err := os.MkdirTemp("", "coding-agent-deployment-source-*")
	if err != nil {
		return "", nil, err
	}
	cleanup := func() { _ = os.RemoveAll(stage) }
	if err := copySourceTreeStrict(
		filepath.Join(profilesRoot, "agents"),
		filepath.Join(stage, "agents"),
	); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("stage canonical agent sources: %w", err)
	}
	if err := copySourceTreeStrict(
		filepath.Join(appRoot, "agents", "serving"),
		filepath.Join(stage, "applications", "coding-agent"),
	); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("stage application serving sources: %w", err)
	}
	return stage, cleanup, nil
}

func copySourceTreeStrict(source, destination string) error {
	return filepath.WalkDir(source, func(filename string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(source, filename)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, rel)
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("source tree contains symlink %s", filename)
		}
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("source tree contains non-regular file %s", filename)
		}
		return copyProfileAsset(filename, target)
	})
}

func resolveServingRoleClosure(root string, ref profileReference) (*profileClosure, error) {
	closure := &profileClosure{
		sourceRoot: root,
		assets:     make(map[string]string),
	}
	if err := closure.enqueue(ref.RuntimePath, ref.RuntimePath); err != nil {
		return nil, err
	}
	if err := closure.resolve(); err != nil {
		return nil, err
	}
	return closure, nil
}

func writeRoleClosure(root, destination string, assets map[string]string) ([]string, []configMapPartition, error) {
	files := make([]string, 0, len(assets))
	for path := range assets {
		files = append(files, path)
	}
	sort.Strings(files)
	partitions, err := partitionConfigMapFiles(root, assets, files, configMapPayloadLimit)
	if err != nil {
		return nil, nil, err
	}
	for _, path := range files {
		source, err := secureSourcePath(root, assets[path])
		if err != nil {
			return nil, nil, err
		}
		if err := copyProfileAsset(source, filepath.Join(destination, filepath.FromSlash(path))); err != nil {
			return nil, nil, err
		}
	}
	return files, partitions, nil
}

func partitionConfigMapFiles(
	root string,
	assets map[string]string,
	files []string,
	limit int,
) ([]configMapPartition, error) {
	var partitions []configMapPartition
	current := configMapPartition{}
	keys := make(map[string]string, len(files))
	for _, path := range files {
		source, err := secureSourcePath(root, assets[path])
		if err != nil {
			return nil, err
		}
		data, err := os.ReadFile(source)
		if err != nil {
			return nil, fmt.Errorf("read ConfigMap asset %s: %w", path, err)
		}
		key := strings.ReplaceAll(path, "/", "__")
		if previous, exists := keys[key]; exists {
			return nil, fmt.Errorf("ConfigMap key conflict %s from %s and %s", key, previous, path)
		}
		keys[key] = path
		size := len(key) + len(data)
		if size > limit {
			return nil, fmt.Errorf(
				"profile asset %s is %d bytes encoded and exceeds ConfigMap payload limit %d; a single entry cannot be sharded",
				path, size, limit)
		}
		if len(current.Files) > 0 && current.SizeBytes+size > limit {
			current.Index = len(partitions)
			partitions = append(partitions, current)
			current = configMapPartition{}
		}
		current.Files = append(current.Files, path)
		current.SizeBytes += size
	}
	if len(current.Files) > 0 {
		current.Index = len(partitions)
		partitions = append(partitions, current)
	}
	return partitions, nil
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

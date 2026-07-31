// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

func validatePreparedPackage(root string, extraEntries ...string) error {
	var deployment deploymentPackageManifest
	if err := readStrictYAML(
		filepath.Join(root, "deployment-manifest.yaml"), &deployment); err != nil {
		return err
	}
	if deployment.SchemaVersion != 1 || deployment.Application != "coding-agent" ||
		deployment.MountPath != "/profiles" || deployment.ImageContainsProfiles ||
		deployment.ConfigMapPayloadLimit != configMapPayloadLimit {
		return fmt.Errorf("invalid deployment manifest contract")
	}
	wantRoles := []string{"critic", "executor", "planner"}
	if len(deployment.Shards) != len(wantRoles) {
		return fmt.Errorf("deployment manifest has %d shards, want 3", len(deployment.Shards))
	}
	for index, shard := range deployment.Shards {
		role := wantRoles[index]
		if shard.Role != role || shard.Path != role ||
			shard.Manifest != "manifests/"+role+".yaml" ||
			shard.Profile != "applications/coding-agent/"+role+"/profile.yaml" {
			return fmt.Errorf("deployment shard %d is stale or malformed: %#v", index, shard)
		}
		if err := validateRolePackage(root, deployment, shard); err != nil {
			return fmt.Errorf("%s shard: %w", role, err)
		}
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return err
	}
	var names []string
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	// The resolver package carries exactly the three #875 role shards; the
	// staged chart adds the catalog collector profile as an extra entry
	// (GH-1162).
	wantEntries := append([]string{"critic", "deployment-manifest.yaml", "executor", "manifests", "planner"}, extraEntries...)
	sort.Strings(wantEntries)
	if !reflect.DeepEqual(names, wantEntries) {
		return fmt.Errorf("profile package top-level entries = %v, want %v", names, wantEntries)
	}
	return nil
}

func validateRolePackage(
	root string,
	deployment deploymentPackageManifest,
	shard deploymentShard,
) error {
	var manifest rolePackageManifest
	if err := readStrictYAML(filepath.Join(root, filepath.FromSlash(shard.Manifest)), &manifest); err != nil {
		return err
	}
	if manifest.SchemaVersion != 1 || manifest.Application != deployment.Application ||
		manifest.Role != shard.Role || manifest.MountPath != deployment.MountPath ||
		manifest.Profile != shard.Profile || manifest.Checksum == "" ||
		shard.ConfigMaps != len(manifest.ConfigMaps) {
		return fmt.Errorf("role manifest does not match deployment manifest")
	}
	if !sort.StringsAreSorted(manifest.Files) || hasDuplicateStrings(manifest.Files) {
		return fmt.Errorf("role files must be sorted and unique")
	}
	roleRoot := filepath.Join(root, shard.Path)
	actual, err := regularRelativeFiles(roleRoot)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(actual, manifest.Files) {
		return fmt.Errorf("role files = %v, manifest files = %v", actual, manifest.Files)
	}
	assets := make(map[string]string, len(actual))
	for _, path := range actual {
		assets[path] = path
	}
	checksum, err := roleClosureChecksum(roleRoot, assets, actual)
	if err != nil {
		return err
	}
	if checksum != manifest.Checksum {
		return fmt.Errorf("checksum mismatch: manifest %s, content %s", manifest.Checksum, checksum)
	}
	covered := make(map[string]bool, len(actual))
	keys := make(map[string]string, len(actual))
	for index, partition := range manifest.ConfigMaps {
		if partition.Index != index || len(partition.Files) == 0 {
			return fmt.Errorf("ConfigMap partition %d has invalid index or no files", index)
		}
		size := 0
		for _, path := range partition.Files {
			if covered[path] {
				return fmt.Errorf("profile asset %s appears in multiple ConfigMap partitions", path)
			}
			if _, exists := assets[path]; !exists {
				return fmt.Errorf("ConfigMap partition references missing asset %s", path)
			}
			covered[path] = true
			key := strings.ReplaceAll(path, "/", "__")
			if previous, exists := keys[key]; exists {
				return fmt.Errorf("ConfigMap key conflict %s from %s and %s", key, previous, path)
			}
			keys[key] = path
			data, err := os.ReadFile(filepath.Join(roleRoot, filepath.FromSlash(path)))
			if err != nil {
				return err
			}
			size += len(key) + len(data)
		}
		if size != partition.SizeBytes || size > deployment.ConfigMapPayloadLimit {
			return fmt.Errorf("ConfigMap partition %d size %d, manifest %d, limit %d",
				index, size, partition.SizeBytes, deployment.ConfigMapPayloadLimit)
		}
	}
	if len(covered) != len(actual) {
		return fmt.Errorf("ConfigMap partitions cover %d of %d files", len(covered), len(actual))
	}
	return nil
}

func regularRelativeFiles(root string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("package contains symlink %s", path)
		}
		if entry.IsDir() {
			return nil
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("package contains non-regular file %s", path)
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

func readStrictYAML(filename string, target any) error {
	file, err := os.Open(filename)
	if err != nil {
		return fmt.Errorf("read %s: %w", filename, err)
	}
	defer file.Close()
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

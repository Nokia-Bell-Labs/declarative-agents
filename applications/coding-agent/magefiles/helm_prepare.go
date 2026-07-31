// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// HelmPrepare regenerates #875 role packages, stages them into the source
// chart, and stages the catalog collector profile alongside them.
func HelmPrepare() error {
	root, err := os.Getwd()
	if err != nil {
		return err
	}
	if err := Package(); err != nil {
		return err
	}
	source := envOrDefault(profileOutputEnv, filepath.Join(root, filepath.FromSlash(defaultProfileOutput)))
	if err := prepareHelmProfiles(source, filepath.Join(root, "helm")); err != nil {
		return err
	}
	catalogRoot, err := resolveCatalogRoot("helm prepare", root)
	if err != nil {
		return err
	}
	if err := stageCollectorProfile(catalogRoot, filepath.Join(root, "helm")); err != nil {
		return err
	}
	if err := stageApplierProfile(root, filepath.Join(root, "helm")); err != nil {
		return err
	}
	fmt.Printf("prepared Helm profile artifacts from %s\n", source)
	return nil
}

func prepareHelmProfiles(packageRoot, chartRoot string) error {
	if _, err := os.Stat(filepath.Join(packageRoot, "deployment-manifest.yaml")); err != nil {
		return fmt.Errorf("prepared profile package has no deployment manifest: %w", err)
	}
	if err := validatePreparedPackage(packageRoot); err != nil {
		return fmt.Errorf("validate prepared profile package: %w", err)
	}
	parent := chartRoot
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return err
	}
	stage, err := os.MkdirTemp(parent, ".profiles-stage-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	if err := copyTree(packageRoot, stage); err != nil {
		return fmt.Errorf("stage Helm profiles: %w", err)
	}
	destination := filepath.Join(chartRoot, "profiles")
	if err := os.RemoveAll(destination); err != nil {
		return err
	}
	if err := os.Rename(stage, destination); err != nil {
		return fmt.Errorf("publish Helm profiles: %w", err)
	}
	return nil
}

// resolveApplicationRoot walks up from the working directory to the coding-agent
// application root -- the directory carrying agents/application.yaml -- so the
// application-owned applier profile resolves whether the caller runs from the
// application directory (mage) or from the magefiles package (go test), the same
// concern resolveCatalogRoot handles for the catalog.
func resolveApplicationRoot(owner string) (string, error) {
	dir, err := filepath.Abs(".")
	if err != nil {
		return "", fmt.Errorf("%s: resolve working directory: %w", owner, err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "agents", "application.yaml")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf(
				"%s: could not locate the coding-agent application root (agents/application.yaml)", owner)
		}
		dir = parent
	}
}

// stageApplierProfile stages the application-owned applier profile (srd006) into
// the chart the same way stageCollectorProfile stages the collector: a flat family
// of files mounted at /profiles/agents/applier. The applier is not a canonical
// serving role and is not driven through deployment.serving_profiles, so it never
// enters the #875 serving-deployment package; like the collector it is a
// special-cased profile the chart carries alongside the serving shards.
func stageApplierProfile(appRoot, chartRoot string) error {
	source := filepath.Join(appRoot, "agents", "serving", "applier")
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

func stageCollectorProfile(catalogRoot, chartRoot string) error {
	source := filepath.Join(catalogRoot, "agents", "collector")
	destination := filepath.Join(chartRoot, "profiles", "collector", "agents", "collector")
	if err := os.MkdirAll(destination, 0o755); err != nil {
		return fmt.Errorf("stage collector profile: %w", err)
	}
	entries, err := os.ReadDir(source)
	if err != nil {
		return fmt.Errorf("read catalog collector: %w", err)
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
			return fmt.Errorf("read collector %s: %w", entry.Name(), err)
		}
		if err := os.WriteFile(filepath.Join(destination, entry.Name()), data, info.Mode().Perm()&fs.ModePerm); err != nil {
			return fmt.Errorf("write collector %s: %w", entry.Name(), err)
		}
	}
	return nil
}

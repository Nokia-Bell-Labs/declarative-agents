// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// provisionCuratorUIShards packs the documentation-curator's browser UI and the
// catalog docs tree into gzipped sub-1-MiB shards, then creates one ConfigMap per
// shard in namespace and returns the ConfigMap names for the chart's
// curatorUI.shards value. The shards are provisioned OUTSIDE the Helm release
// because the gzipped UI (~1.2 MiB) is incompressible and carrying it in-release
// (chart files plus the rendered binaryData) blows past the 3 MiB apiserver
// limit (GH-1402); the curator init container concatenates the mounted shards
// back into the tar.gz it unpacks (GH-1368).
//
// Each ConfigMap holds a single binaryData key part-NNN, so the chart can
// project them into one volume without per-key item maps and the init container's
// `cat /curator-ui/part-*` reassembles them in order.
func provisionCuratorUIShards(environment smokeEnvironment, catalogRoot, namespace, releasePrefix string) ([]string, error) {
	dir, err := os.MkdirTemp("", "curator-ui-shards-*")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(dir) }()
	if err := prepareCuratorAssets(catalogRoot, dir); err != nil {
		return nil, fmt.Errorf("stage curator UI shards: %w", err)
	}
	shardPaths, err := filepath.Glob(filepath.Join(dir, curatorAssetsDir, curatorAssetShardPrefix+"*"))
	if err != nil {
		return nil, err
	}
	sort.Strings(shardPaths)
	if len(shardPaths) == 0 {
		return nil, fmt.Errorf("no curator UI shards were staged under %s", dir)
	}
	names := make([]string, 0, len(shardPaths))
	for _, path := range shardPaths {
		key := filepath.Base(path) // part-NNN
		name := releasePrefix + "-curator-ui-" + strings.TrimPrefix(key, curatorAssetShardPrefix)
		if err := runSmokeCommand(environment, 30*time.Second, "kubectl", "create", "configmap", name,
			"--from-file="+key+"="+path, "-n", namespace); err != nil {
			return nil, fmt.Errorf("create curator UI shard ConfigMap %s: %w", name, err)
		}
		names = append(names, name)
	}
	return names, nil
}

// curatorUIShardSetArgs renders the shard ConfigMap names as helm --set arguments
// for the chart's curatorUI.shards list, so the curator Deployment mounts and
// unpacks them.
func curatorUIShardSetArgs(names []string) []string {
	args := make([]string, 0, len(names)*2)
	for index, name := range names {
		args = append(args, "--set", fmt.Sprintf("curatorUI.shards[%d]=%s", index, name))
	}
	return args
}

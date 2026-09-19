// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package kindrig

import (
	"fmt"
	"runtime"
	"strings"
	"time"
)

// installStep is one command of a pinned-image install, named for its phase
// line so a slow boot names the step that stalled (GH-2226).
type installStep struct {
	phase   string
	command []string
}

// pinnedImageSteps returns the steps that make a digest-pinned source image
// available in the cluster node under runtimeImage. The pull is skipped when
// the digest is already local: a digest names immutable content, so a cached
// image is the image, and pulling it again only asks the registry, which
// stalled a release boot for 19 minutes (GH-2226).
func pinnedImageSteps(run CommandRunner, cluster, sourceImage, runtimeImage string) []installStep {
	var steps []installStep
	if _, err := run("docker", "image", "inspect", "--format", "{{.Id}}", sourceImage); err != nil {
		steps = append(steps, installStep{"image-pull",
			[]string{"docker", "pull", "--platform", "linux/" + runtime.GOARCH, sourceImage}})
	} else {
		LogPhase(cluster, "image-pull", "skipped", time.Now(), "image="+sourceImage)
	}
	return append(steps,
		installStep{"image-tag", []string{"docker", "tag", sourceImage, runtimeImage}},
		installStep{"image-load", nodeImportCommand(runtimeImage, cluster)},
	)
}

// nodeImportCommand streams a host image into the kind node's containerd for
// the host platform only. kind load imports with --all-platforms, which fails
// when the host holds a multi-platform index with only its own platform's
// layers, as Docker Desktop's containerd store does for a digest-pinned pull
// (GH-2222).
func nodeImportCommand(image, cluster string) []string {
	return []string{"sh", "-c",
		`docker save "$1" | docker exec -i "$2" ctr --namespace=k8s.io images import --platform="$3" --snapshotter=overlayfs -`,
		"node-import", image, cluster + "-control-plane", "linux/" + runtime.GOARCH}
}

// runInstallSteps runs steps in order, logging one phase line per step, and
// stops at the first failure with the command and its output.
func runInstallSteps(run CommandRunner, cluster, component string, steps []installStep) error {
	for _, step := range steps {
		started := time.Now()
		name, args := step.command[0], step.command[1:]
		output, err := run(name, args...)
		detail := "component=" + component
		if err != nil {
			LogPhase(cluster, step.phase, "failed", started, detail)
			return fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "),
				err, strings.TrimSpace(string(output)))
		}
		LogPhase(cluster, step.phase, "passed", started, detail)
	}
	return nil
}

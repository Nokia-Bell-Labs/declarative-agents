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

// KubeContext is the kubeconfig context kind writes for a cluster.
func KubeContext(cluster string) string { return "kind-" + cluster }

// inCluster binds a kubectl command to one cluster's context. The installers
// name the cluster they load images into, but kubectl otherwise follows
// whatever context happens to be current, so a stale context sends the
// manifest to one cluster while the image lands in another: the pod fails
// ErrImageNeverPull and the rollout times out naming neither (GH-2428).
// Commands that are not kubectl are returned unchanged.
func inCluster(cluster string, command []string) []string {
	if len(command) == 0 || command[0] != "kubectl" || strings.TrimSpace(cluster) == "" {
		return command
	}
	bound := make([]string, 0, len(command)+2)
	bound = append(bound, command[0], "--context", KubeContext(cluster))
	return append(bound, command[1:]...)
}

// runInstallSteps runs steps in order, logging one phase line per step, and
// stops at the first failure with the command and its output. Every kubectl
// step is bound to the named cluster's context.
func runInstallSteps(run CommandRunner, cluster, component string, steps []installStep) error {
	for _, step := range steps {
		started := time.Now()
		bound := inCluster(cluster, step.command)
		name, args := bound[0], bound[1:]
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

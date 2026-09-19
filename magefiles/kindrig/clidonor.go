// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package kindrig

import (
	"fmt"
	"strings"
)

const (
	// CLIDonorImage is the digest-pinned stock image the applier's cli-donor
	// init container copies helm and kubectl from (GH-2222): helm 3.16.3 and
	// kubectl 1.31.4. Every chart's applier.cliDonor.image pins the same
	// reference; a test in each application enforces it.
	CLIDonorImage = "docker.io/alpine/k8s:1.31.4@sha256:9c4976d47656d78cf53a92b0203fc54ac45eae18a2b45001ac221c27da4c8036"
	// CLIDonorHelmVersion is the helm release CLIDonorImage carries. The
	// applier's exec words use helm 3 flag spellings, so the two move together.
	CLIDonorHelmVersion = "v3.16.3"
	// CLIDonorRuntimeImage is the rig-local name the donor is kind-loaded under.
	// A kind-loaded image does not keep its registry digest, so the kind values
	// overlays reference this tag with pullPolicy Never, as the Traefik install
	// does with its own retag.
	CLIDonorRuntimeImage = "kindrig/cli-donor:1.31.4"
)

// EnsureCLIDonorImage makes the donor available in the cluster node under
// CLIDonorRuntimeImage: pulled only when its digest is not already local, then
// retagged and kind-loaded. The caller supplies a runner with docker and kind on
// its path, bounded as the platform runner is.
func EnsureCLIDonorImage(run CommandRunner, cluster string) error {
	if strings.TrimSpace(cluster) == "" {
		return fmt.Errorf("ensure CLI donor: kind cluster name is required")
	}
	steps := pinnedImageSteps(run, cluster, CLIDonorImage, CLIDonorRuntimeImage)
	if err := runInstallSteps(run, cluster, "cli-donor", steps); err != nil {
		return fmt.Errorf("ensure CLI donor: %w", err)
	}
	return nil
}

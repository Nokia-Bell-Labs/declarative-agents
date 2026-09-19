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

// VerifyCLIDonor proves, inside a running applier container reached through
// inApplier, what the donor pattern promises: helm resolves to CLIDonorHelmVersion
// with the major the exec declarations are written for, kubectl is present, and
// the container cannot write /opt/tools, so the binaries it execs cannot be
// replaced at runtime. It returns the helm version for the caller's evidence.
func VerifyCLIDonor(declaredHelmMajor string, inApplier func(args ...string) (string, error)) (string, error) {
	version, err := inApplier("helm", "version", "--template", "{{.Version}}")
	if err != nil {
		return "", fmt.Errorf("helm in the applier container: %w: %s", err, version)
	}
	if version != CLIDonorHelmVersion {
		return "", fmt.Errorf("the applier resolves helm %s, but the pinned CLI donor carries %s",
			version, CLIDonorHelmVersion)
	}
	if major := strings.TrimPrefix(strings.SplitN(version, ".", 2)[0], "v"); major != declaredHelmMajor {
		return "", fmt.Errorf("the donor ships helm %s, but the exec declarations are written for helm %s; "+
			"the flag spellings differ between majors and helm rejects an unknown flag",
			version, declaredHelmMajor)
	}
	if output, err := inApplier("kubectl", "version", "--client"); err != nil ||
		!strings.Contains(output, "Client Version") {
		return "", fmt.Errorf("kubectl in the applier container: %v: %s", err, output)
	}
	output, err := inApplier("sh", "-c", "touch /opt/tools/cli-donor-write-probe")
	if err == nil {
		return "", fmt.Errorf("the applier container can write /opt/tools, so the binaries it execs are replaceable")
	}
	if !strings.Contains(strings.ToLower(output), "read-only") {
		return "", fmt.Errorf("writing /opt/tools failed, but not on a read-only mount: %v: %s", err, output)
	}
	fmt.Printf("cli-donor: the applier runs helm %s and kubectl from a read-only /opt/tools\n", version)
	return version, nil
}
